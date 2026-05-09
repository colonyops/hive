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

// httpDefaultTimeout is used when HTTPModule.DefaultTimeout is zero.
const httpDefaultTimeout = 30 * time.Second

// httpDefaultMaxBytes caps response bodies so a runaway download cannot
// OOM the process. Plugins that legitimately need more must opt in
// per-call via opts.max_bytes.
const httpDefaultMaxBytes = int64(10 * 1024 * 1024)

// httpClient is the subset of *http.Client the module uses; tests
// inject a fake to avoid hitting the network.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// httpRequest captures the resolved per-call inputs for an HTTP call.
// Built from the Lua opts table by applyHTTPOpts so the executor stays
// independent of glua types.
type httpRequest struct {
	Method   string
	URL      string
	Headers  http.Header
	Body     string
	Timeout  time.Duration
	MaxBytes int64
}

// httpResult is the executor's complete return value. Network/protocol
// failures populate Err; non-2xx status codes are not failures and live
// alongside Body+Headers.
type httpResult struct {
	Status  int
	Body    string
	Headers map[string]string
	// Cookies preserves Set-Cookie verbatim and in order. Set-Cookie
	// cannot be safely flattened into Headers because cookie values
	// legitimately contain commas (Expires dates), so a comma-joined
	// string corrupts every cookie after the first.
	Cookies []string
	Err     error
}

// HTTPModule exposes hive.http.{get,post,put,delete,request} for outbound
// HTTP requests. Every entry point is async: the call returns a handle
// immediately, the request runs on a goroutine bound to a per-handle
// context, and the caller-supplied callback fires on the dispatcher when
// the response (or error) is in. Close cancels every in-flight call.
type HTTPModule struct {
	Client          httpClient
	DefaultTimeout  time.Duration
	DefaultMaxBytes int64
	Logger          zerolog.Logger

	Runtime *Runtime

	registry asyncRegistry

	closeOnce sync.Once
}

// Register installs the hive.http subtable and initialises the per-module
// async registry. Defaults the client, timeout, and max-bytes if unset.
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

// Close cancels every in-flight request and waits for the workers to
// drain. Idempotent.
func (m *HTTPModule) Close() error {
	m.closeOnce.Do(func() { m.registry.shutdown() })
	return nil
}

// luaVerb returns the Lua handler for a verb where the URL is the first
// positional argument. Accepts (url, fn) or (url, opts, fn).
func (m *HTTPModule) luaVerb(method string) glua.LGFunction {
	return func(state *glua.LState) int {
		req, callback, err := m.parseVerbArgs(state, method)
		if err != nil {
			state.RaiseError("hive.http.%s: %s", strings.ToLower(method), err.Error())
			return 0
		}
		handle := m.spawnAsync(state, callback, req)
		state.Push(m.registry.handleUserData(state, handle))
		return 1
	}
}

// luaRequest is hive.http.request(opts, fn). opts.method defaults to GET
// and opts.url is required.
func (m *HTTPModule) luaRequest(state *glua.LState) int {
	opts := state.CheckTable(1)
	callback := state.CheckFunction(2)

	req, err := m.parseRequestTable(opts)
	if err != nil {
		state.RaiseError("hive.http.request: %s", err.Error())
		return 0
	}
	handle := m.spawnAsync(state, callback, req)
	state.Push(m.registry.handleUserData(state, handle))
	return 1
}

// parseVerbArgs handles the (url, fn) and (url, opts, fn) shapes shared
// by get/post/put/delete.
func (m *HTTPModule) parseVerbArgs(state *glua.LState, method string) (httpRequest, *glua.LFunction, error) {
	rawURL := state.CheckString(1)

	var (
		optsTable *glua.LTable
		callback  *glua.LFunction
	)
	switch state.GetTop() {
	case 2:
		callback = state.CheckFunction(2)
	case 3:
		if state.Get(2) != glua.LNil {
			optsTable = state.CheckTable(2)
		}
		callback = state.CheckFunction(3)
	default:
		return httpRequest{}, nil, fmt.Errorf("expected (url, fn) or (url, opts, fn)")
	}

	req := m.defaultRequest()
	req.Method = method
	req.URL = rawURL
	if optsTable != nil {
		if err := applyHTTPOpts(optsTable, &req); err != nil {
			return httpRequest{}, nil, err
		}
	}
	if err := finaliseRequest(&req); err != nil {
		return httpRequest{}, nil, err
	}
	return req, callback, nil
}

// parseRequestTable handles hive.http.request's single opts table. Like
// applyHTTPOpts but also reads method and url from the table.
func (m *HTTPModule) parseRequestTable(opts *glua.LTable) (httpRequest, error) {
	req := m.defaultRequest()
	req.Method = "GET"
	if method, ok := opts.RawGetString("method").(glua.LString); ok && method != "" {
		req.Method = strings.ToUpper(string(method))
	}
	rawURL, ok := opts.RawGetString("url").(glua.LString)
	if !ok || rawURL == "" {
		return httpRequest{}, fmt.Errorf("opts.url must be a non-empty string")
	}
	req.URL = string(rawURL)
	if err := applyHTTPOpts(opts, &req); err != nil {
		return httpRequest{}, err
	}
	if err := finaliseRequest(&req); err != nil {
		return httpRequest{}, err
	}
	return req, nil
}

// defaultRequest seeds a request with the module's default timeout and
// max-bytes plus an empty headers map ready for opt copying.
func (m *HTTPModule) defaultRequest() httpRequest {
	return httpRequest{
		Headers:  http.Header{},
		Timeout:  m.DefaultTimeout,
		MaxBytes: m.DefaultMaxBytes,
	}
}

// applyHTTPOpts copies headers/query/body/timeout/max_bytes from t into
// req. Unknown keys are ignored so plugins can add forward-compatible
// extensions to their own opts tables without surprise errors.
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

// finaliseRequest validates the assembled request shape so dispatcher
// goroutines never fire a malformed call.
func finaliseRequest(req *httpRequest) error {
	if req.URL == "" {
		return fmt.Errorf("url must be a non-empty string")
	}
	if req.Method == "" {
		return fmt.Errorf("method must be a non-empty string")
	}
	return nil
}

// copyHeaders copies string-valued entries from t into dst. Numeric
// values are coerced via tostring so plugins can pass bare numbers
// (e.g. Content-Length) without manual conversion. Other types are
// rejected so silent string("") substitutions don't strip data.
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

// mergeQuery splices key=value pairs from t into rawURL's query string.
// Existing query parameters are preserved.
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
		case glua.LNumber:
			q.Add(string(name), glua.LVAsString(v))
		case glua.LBool:
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

// spawnAsync mirrors ShModule.spawnAsync — pin the callback in the Lua
// registry, allocate a per-handle context, run the request on a tracked
// goroutine, and dispatch the result back through the runtime so
// shutdown can drain in-flight work. Each request emits paired
// start/finish log lines tagged with the handle id so operators can
// correlate events for a specific in-flight call.
func (m *HTTPModule) spawnAsync(
	state *glua.LState,
	fn *glua.LFunction,
	req httpRequest,
) *asyncHandle {
	id, ctx := m.registry.allocate(state, fn)
	h := m.registry.newHandle(id, m.Runtime)

	m.registry.Go(func() {
		start := time.Now()
		m.Logger.Debug().
			Uint64("handle", id).
			Str("method", req.Method).
			Str("url", req.URL).
			Dur("timeout", req.Timeout).
			Int64("max_bytes", req.MaxBytes).
			Msg("hive.http: starting request")

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

// do builds and sends the HTTP request, capping the response body at
// req.MaxBytes. Cancellation comes from ctx (per-handle) and the
// per-call timeout layered on top.
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

	limit := req.MaxBytes
	if limit <= 0 {
		limit = httpDefaultMaxBytes
	}
	// Read up to limit+1 so we can tell "exactly limit" apart from
	// "exceeded the cap"; on overrun, surface a single deterministic
	// error rather than a partially-truncated body.
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return httpResult{Err: err}
	}
	if int64(len(data)) > limit {
		return httpResult{
			Err: fmt.Errorf("response body exceeded max_bytes (%d)", limit),
		}
	}

	headers, cookies := splitHeaders(resp.Header)
	return httpResult{
		Status:  resp.StatusCode,
		Body:    string(data),
		Headers: headers,
		Cookies: cookies,
	}
}

// splitHeaders flattens http.Header into a single-value map plus a
// cookies slice. Set-Cookie is split off because comma-joining cookie
// header lines corrupts dates (Expires=Tue, 06 May ...). Other
// multi-value headers join with ", " — plugins can split if they need
// the original values.
//
// Header names follow net/http's canonical MIME case (e.g. "X-Echo")
// because the response is keyed by the values http.Header chose; this
// is documented in the Lua-facing docs so plugin authors don't have to
// guess the casing.
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

// dispatch invokes the callback on the dispatcher with (response, err).
// On network/protocol failure, response is LNil and err is a string.
// Non-2xx HTTP responses are NOT failures here — the callback gets a
// populated response and can branch on response.status.
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
		m.Logger.Warn().
			Err(err).
			Msg("hive.http: callback returned error")
	}
}

// buildResponseTable shapes the success response for Lua: status, body,
// a string-valued headers map, and a cookies array. Cookies are always
// present (empty array when none were sent) so plugins can iterate
// without nil-checking.
func buildResponseTable(state *glua.LState, r httpResult) *glua.LTable {
	out := state.NewTable()
	state.SetField(out, "status", glua.LNumber(r.Status))
	state.SetField(out, "body", glua.LString(r.Body))

	headers := state.NewTable()
	for name, value := range r.Headers {
		state.SetField(headers, name, glua.LString(value))
	}
	state.SetField(out, "headers", headers)

	cookies := state.NewTable()
	for i, c := range r.Cookies {
		cookies.RawSetInt(i+1, glua.LString(c))
	}
	state.SetField(out, "cookies", cookies)
	return out
}
