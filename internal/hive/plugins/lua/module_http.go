package lua

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	glua "github.com/yuin/gopher-lua"
)

const (
	httpDefaultTimeout  = 30 * time.Second
	httpDefaultMaxBytes = int64(10 * 1024 * 1024)
)

// httpClient is the subset of *http.Client the module uses; tests
// inject a fake to avoid hitting the network.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type httpRequest struct {
	Method   string
	URL      string
	Headers  http.Header
	Body     string
	Timeout  time.Duration
	MaxBytes int64
}

// httpResult is the executor's return value. Network/protocol failures
// populate Err; non-2xx status codes are not failures and live alongside
// Body+Headers. Cookies preserves Set-Cookie verbatim because cookie
// values can contain commas (Expires dates), which would corrupt every
// cookie after the first if joined into Headers.
type httpResult struct {
	Status  int
	Body    string
	Headers map[string]string
	Cookies []string
	Err     error
}

// HTTPModule exposes hive.http.{get,post,put,delete,request}. Every
// entry point is async: the call returns a handle immediately, the
// request runs on a goroutine bound to a per-handle context, and the
// callback fires on the dispatcher when the response (or error) is in.
// Close cancels every in-flight call.
type HTTPModule struct {
	Client          httpClient
	DefaultTimeout  time.Duration
	DefaultMaxBytes int64
	Logger          zerolog.Logger

	Runtime *Runtime

	registry  asyncRegistry
	closeOnce sync.Once
}

func (m *HTTPModule) Register(state *glua.LState, hive *glua.LTable) error {
	if m.Client == nil {
		m.Client = &http.Client{}
	}
	if m.DefaultTimeout <= 0 {
		m.DefaultTimeout = httpDefaultTimeout
	}
	if m.DefaultMaxBytes <= 0 {
		m.DefaultMaxBytes = httpDefaultMaxBytes
	}

	if err := m.registry.init(state, m, asyncRegistryConfig{
		KeyPrefix:     "hive.http.",
		MetatableName: "hive.http.handle",
	}); err != nil {
		return fmt.Errorf("http module: %w", err)
	}

	tbl := state.NewTable()
	state.SetField(tbl, "get", state.NewFunction(m.luaVerb("GET")))
	state.SetField(tbl, "post", state.NewFunction(m.luaVerb("POST")))
	state.SetField(tbl, "put", state.NewFunction(m.luaVerb("PUT")))
	state.SetField(tbl, "delete", state.NewFunction(m.luaVerb("DELETE")))
	state.SetField(tbl, "request", state.NewFunction(m.luaRequest))
	state.SetField(hive, "http", tbl)
	return nil
}

func (m *HTTPModule) Close() error {
	m.closeOnce.Do(func() { m.registry.shutdown() })
	return nil
}

// luaVerb handles `hive.http.<verb>(url, fn)` and `(url, opts, fn)`.
func (m *HTTPModule) luaVerb(method string) glua.LGFunction {
	return func(state *glua.LState) int {
		rawURL := state.CheckString(1)

		var optsTable *glua.LTable
		var callback *glua.LFunction
		switch state.GetTop() {
		case 2:
			callback = state.CheckFunction(2)
		case 3:
			if state.Get(2) != glua.LNil {
				optsTable = state.CheckTable(2)
			}
			callback = state.CheckFunction(3)
		default:
			state.RaiseError("hive.http.%s: expected (url, fn) or (url, opts, fn)", strings.ToLower(method))
			return 0
		}

		req := httpRequest{
			Method:   method,
			URL:      rawURL,
			Headers:  http.Header{},
			Timeout:  m.DefaultTimeout,
			MaxBytes: m.DefaultMaxBytes,
		}
		if optsTable != nil {
			if err := applyHTTPOpts(optsTable, &req); err != nil {
				state.RaiseError("hive.http.%s: %s", strings.ToLower(method), err.Error())
				return 0
			}
		}
		if req.URL == "" {
			state.RaiseError("hive.http.%s: url must be a non-empty string", strings.ToLower(method))
			return 0
		}

		state.Push(m.registry.handleUserData(state, m.spawnAsync(state, callback, req)))
		return 1
	}
}

// luaRequest handles `hive.http.request(opts, fn)`. opts.method defaults
// to GET; opts.url is required.
func (m *HTTPModule) luaRequest(state *glua.LState) int {
	opts := state.CheckTable(1)
	callback := state.CheckFunction(2)

	req := httpRequest{
		Method:   "GET",
		Headers:  http.Header{},
		Timeout:  m.DefaultTimeout,
		MaxBytes: m.DefaultMaxBytes,
	}
	if method, ok := opts.RawGetString("method").(glua.LString); ok && method != "" {
		req.Method = strings.ToUpper(string(method))
	}
	if rawURL, ok := opts.RawGetString("url").(glua.LString); ok {
		req.URL = string(rawURL)
	}
	if err := applyHTTPOpts(opts, &req); err != nil {
		state.RaiseError("hive.http.request: %s", err.Error())
		return 0
	}
	if req.URL == "" {
		state.RaiseError("hive.http.request: opts.url must be a non-empty string")
		return 0
	}

	state.Push(m.registry.handleUserData(state, m.spawnAsync(state, callback, req)))
	return 1
}

// applyHTTPOpts copies headers/query/body/timeout/max_bytes from t into
// req. Unknown keys are ignored so plugins can extend their own opts
// tables forward-compatibly.
func applyHTTPOpts(t *glua.LTable, req *httpRequest) error {
	if headers, ok := t.RawGetString("headers").(*glua.LTable); ok {
		if err := copyHeaders(headers, req.Headers); err != nil {
			return err
		}
	}
	if query, ok := t.RawGetString("query").(*glua.LTable); ok {
		merged, err := mergeQuery(req.URL, query)
		if err != nil {
			return err
		}
		req.URL = merged
	}
	if body, ok := t.RawGetString("body").(glua.LString); ok {
		req.Body = string(body)
	}
	if timeoutSecs, ok := t.RawGetString("timeout").(glua.LNumber); ok {
		req.Timeout = time.Duration(float64(timeoutSecs) * float64(time.Second))
	}
	if maxBytes, ok := t.RawGetString("max_bytes").(glua.LNumber); ok {
		req.MaxBytes = int64(maxBytes)
	}
	return nil
}

// copyHeaders rejects non-string/number values explicitly — silently
// stringifying a table or function would strip data without warning.
func copyHeaders(t *glua.LTable, dst http.Header) error {
	var walkErr error
	t.ForEach(func(key, value glua.LValue) {
		if walkErr != nil {
			return
		}
		name, ok := key.(glua.LString)
		if !ok {
			walkErr = fmt.Errorf("header names must be strings")
			return
		}
		switch v := value.(type) {
		case glua.LString:
			dst.Add(string(name), string(v))
		case glua.LNumber:
			dst.Add(string(name), glua.LVAsString(v))
		default:
			walkErr = fmt.Errorf("header %q must be a string", string(name))
		}
	})
	return walkErr
}

func mergeQuery(rawURL string, t *glua.LTable) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url %q: %w", rawURL, err)
	}
	q := u.Query()
	var walkErr error
	t.ForEach(func(key, value glua.LValue) {
		if walkErr != nil {
			return
		}
		name, ok := key.(glua.LString)
		if !ok {
			walkErr = fmt.Errorf("query keys must be strings")
			return
		}
		switch v := value.(type) {
		case glua.LString:
			q.Add(string(name), string(v))
		case glua.LNumber, glua.LBool:
			q.Add(string(name), glua.LVAsString(v))
		default:
			walkErr = fmt.Errorf("query %q must be a string, number, or boolean", string(name))
		}
	})
	if walkErr != nil {
		return "", walkErr
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// spawnAsync mirrors ShModule.spawnAsync: pin the callback in the Lua
// registry, allocate a per-handle context, run the request on a tracked
// goroutine, and dispatch the result back through the runtime so
// shutdown can drain in-flight work.
func (m *HTTPModule) spawnAsync(state *glua.LState, fn *glua.LFunction, req httpRequest) *asyncHandle {
	id, ctx := m.registry.allocate(state, fn)
	h := m.registry.newHandle(id, m.Runtime)

	m.registry.Go(func() {
		start := time.Now()
		result := m.do(ctx, req)

		level := m.Logger.Debug
		if result.Err != nil {
			level = m.Logger.Warn
		}
		event := level().
			Uint64("handle", id).
			Str("method", req.Method).
			Str("url", req.URL).
			Dur("duration", time.Since(start))
		if result.Err != nil {
			event = event.Err(result.Err)
		} else {
			event = event.Int("status", result.Status).Int("bytes", len(result.Body))
		}
		event.Msg("hive.http: request finished")

		err := m.Runtime.submitSync(func(state *glua.LState) error {
			fn := m.registry.loadFunction(state, id)
			if fn == nil {
				return nil
			}
			m.dispatch(state, fn, result)
			m.registry.release(state, id)
			h.poison()
			return nil
		})
		if err != nil {
			m.Logger.Debug().
				Uint64("handle", id).
				Err(err).
				Msg("hive.http: dispatch dropped (runtime closed)")
		}
	})
	return h
}

func (m *HTTPModule) do(ctx context.Context, req httpRequest) httpResult {
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return httpResult{Err: err}
	}
	for name, values := range req.Headers {
		for _, v := range values {
			httpReq.Header.Add(name, v)
		}
	}

	resp, err := m.Client.Do(httpReq)
	if err != nil {
		return httpResult{Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	// Read up to MaxBytes+1 so an exact-cap body is distinguishable from
	// an over-cap body. Over-cap surfaces a single deterministic error
	// rather than a partially-truncated body.
	data, err := io.ReadAll(io.LimitReader(resp.Body, req.MaxBytes+1))
	if err != nil {
		return httpResult{Err: err}
	}
	if int64(len(data)) > req.MaxBytes {
		return httpResult{Err: fmt.Errorf("response body exceeded max_bytes (%d)", req.MaxBytes)}
	}

	headers, cookies := splitHeaders(resp.Header)
	return httpResult{
		Status:  resp.StatusCode,
		Body:    string(data),
		Headers: headers,
		Cookies: cookies,
	}
}

// splitHeaders peels Set-Cookie off into its own slice because comma-joining
// cookie header lines corrupts dates (Expires=Tue, 06 May ...). All other
// multi-value headers join with ", ".
func splitHeaders(h http.Header) (map[string]string, []string) {
	headers := make(map[string]string, len(h))
	var cookies []string
	for name, values := range h {
		if http.CanonicalHeaderKey(name) == "Set-Cookie" {
			cookies = append(cookies, values...)
			continue
		}
		headers[name] = strings.Join(values, ", ")
	}
	return headers, cookies
}

// dispatch invokes the callback with (response, err). On failure,
// response is LNil and err is a string. Non-2xx HTTP responses get a
// populated response — the plugin branches on response.status.
func (m *HTTPModule) dispatch(state *glua.LState, fn *glua.LFunction, result httpResult) {
	var responseArg, errArg glua.LValue
	if result.Err != nil {
		responseArg = glua.LNil
		errArg = glua.LString(result.Err.Error())
	} else {
		responseArg = buildResponseTable(state, result)
		errArg = glua.LNil
	}
	if err := state.CallByParam(glua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, responseArg, errArg); err != nil {
		m.Logger.Warn().Err(err).Msg("hive.http: callback returned error")
	}
}

func buildResponseTable(state *glua.LState, r httpResult) *glua.LTable {
	out := state.NewTable()
	state.SetField(out, "status", glua.LNumber(r.Status))
	state.SetField(out, "body", glua.LString(r.Body))

	headers := state.NewTable()
	for name, value := range r.Headers {
		state.SetField(headers, name, glua.LString(value))
	}
	state.SetField(out, "headers", headers)

	// Empty array (not nil) so plugins can iterate without nil-checking.
	cookies := state.NewTable()
	for i, c := range r.Cookies {
		cookies.RawSetInt(i+1, glua.LString(c))
	}
	state.SetField(out, "cookies", cookies)
	return out
}
