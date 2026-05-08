package lua

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

// fakeHTTPClient implements httpClient with caller-provided behaviour.
// Every Do call records the request and dispatches to respond.
type fakeHTTPClient struct {
	mu       sync.Mutex
	respond  func(req *http.Request) (*http.Response, error)
	requests []*http.Request
}

func (f *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	respond := f.respond
	f.mu.Unlock()
	if respond == nil {
		return jsonResponse(200, "", nil), nil
	}
	return respond(req)
}

func (f *fakeHTTPClient) lastRequest() *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return nil
	}
	return f.requests[len(f.requests)-1]
}

func (f *fakeHTTPClient) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// jsonResponse builds a 200-class *http.Response with the given body and
// optional header overrides. Used by the fake client's respond funcs.
func jsonResponse(status int, body string, headers http.Header) *http.Response {
	h := http.Header{}
	maps.Copy(h, headers)
	if h.Get("Content-Type") == "" {
		h.Set("Content-Type", "text/plain")
	}
	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// httpHarness wraps luaHarness with the HTTPModule reference so tests can
// reach the module for explicit shutdown ordering.
type httpHarness struct {
	*luaHarness
	module *HTTPModule
	client *fakeHTTPClient
}

func newHTTPHarness(t *testing.T, script string, client *fakeHTTPClient) *httpHarness {
	t.Helper()
	module := &HTTPModule{
		Client:          client,
		DefaultTimeout:  5 * time.Second,
		DefaultMaxBytes: 1024,
		Logger:          zerolog.Nop(),
	}
	return &httpHarness{
		luaHarness: newLuaHarness(t, script, module),
		module:     module,
		client:     client,
	}
}

func TestHTTPModule_GetReturnsResponse(t *testing.T) {
	t.Parallel()

	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "GET", req.Method)
			assert.Equal(t, "https://example.com/api", req.URL.String())
			return jsonResponse(200, `{"ok":true}`, http.Header{"X-Echo": []string{"yes"}}), nil
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.get("https://example.com/api", function(res, err)
    if err ~= nil then
      hive.test_capture("err", err)
      return
    end
    hive.test_capture("status", res.status)
    hive.test_capture("body", res.body)
    hive.test_capture("echo", res.headers["X-Echo"])
  end)
end
`, client)

	waitForKey(t, h.capture, "status")
	assert.Equal(t, 200, int(h.capture.Number("status")))
	assert.Equal(t, `{"ok":true}`, h.capture.String("body"))
	assert.Equal(t, "yes", h.capture.String("echo"))
	assert.False(t, h.capture.Has("err"))
}

func TestHTTPModule_PostSendsBodyAndHeaders(t *testing.T) {
	t.Parallel()

	var (
		gotBody    string
		gotMethod  string
		gotHeader  string
		bodyReadOK bool
	)

	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			gotMethod = req.Method
			gotHeader = req.Header.Get("Content-Type")
			if req.Body != nil {
				data, err := io.ReadAll(req.Body)
				bodyReadOK = err == nil
				gotBody = string(data)
			}
			return jsonResponse(201, "ok", nil), nil
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.post("https://example.com/items", {
    headers = { ["Content-Type"] = "application/json" },
    body = '{"a":1}',
  }, function(res, err)
    if err ~= nil then
      hive.test_capture("err", err)
      return
    end
    hive.test_capture("status", res.status)
  end)
end
`, client)

	waitForKey(t, h.capture, "status")
	assert.Equal(t, 201, int(h.capture.Number("status")))
	assert.Equal(t, "POST", gotMethod)
	assert.Equal(t, "application/json", gotHeader)
	assert.True(t, bodyReadOK)
	assert.Equal(t, `{"a":1}`, gotBody)
}

func TestHTTPModule_PutAndDelete(t *testing.T) {
	t.Parallel()

	methods := make(chan string, 2)
	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			methods <- req.Method
			return jsonResponse(204, "", nil), nil
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.put("https://example.com/x", { body = "u" }, function(res, _)
    hive.test_capture(res.status)
  end)
  hive.http.delete("https://example.com/x", function(res, _)
    hive.test_capture(res.status)
  end)
end
`, client)

	values := waitForCaptures(t, h.capture, 2)
	for _, v := range values {
		assert.Equal(t, 204, asLuaInt(t, v))
	}

	seen := map[string]bool{}
	for range 2 {
		select {
		case m := <-methods:
			seen[m] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("expected two methods")
		}
	}
	assert.True(t, seen["PUT"])
	assert.True(t, seen["DELETE"])
}

func TestHTTPModule_RequestUsesMethodAndURL(t *testing.T) {
	t.Parallel()

	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "PATCH", req.Method)
			assert.Equal(t, "https://example.com/r", req.URL.String())
			return jsonResponse(200, "ok", nil), nil
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.request({ method = "PATCH", url = "https://example.com/r", body = "x" }, function(res, _)
    hive.test_capture("status", res.status)
  end)
end
`, client)

	waitForKey(t, h.capture, "status")
	assert.Equal(t, 200, int(h.capture.Number("status")))
}

func TestHTTPModule_QueryParamsMerged(t *testing.T) {
	t.Parallel()

	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			return jsonResponse(200, "", nil), nil
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.get("https://example.com/api?keep=1", { query = { extra = "two", n = 3 } }, function(res, _)
    hive.test_capture("status", res.status)
  end)
end
`, client)

	waitForKey(t, h.capture, "status")

	req := h.client.lastRequest()
	require.NotNil(t, req)
	q := req.URL.Query()
	assert.Equal(t, "1", q.Get("keep"))
	assert.Equal(t, "two", q.Get("extra"))
	assert.Equal(t, "3", q.Get("n"))
}

func TestHTTPModule_NetworkErrorPassedToCallback(t *testing.T) {
	t.Parallel()

	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.get("https://example.com", function(res, err)
    hive.test_capture("res", res == nil and "nil" or "set")
    hive.test_capture("err", err == nil and "nil" or err)
  end)
end
`, client)

	waitForKey(t, h.capture, "err")
	assert.Equal(t, "nil", h.capture.String("res"))
	assert.Contains(t, h.capture.String("err"), "connection refused")
}

func TestHTTPModule_NonZeroStatusIsSuccess(t *testing.T) {
	t.Parallel()

	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			return jsonResponse(503, "down for maintenance", nil), nil
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.get("https://example.com", function(res, err)
    hive.test_capture("err_is_nil", err == nil)
    if res ~= nil then
      hive.test_capture("status", res.status)
      hive.test_capture("body", res.body)
    end
  end)
end
`, client)

	waitForKey(t, h.capture, "status")
	assert.True(t, h.capture.Bool("err_is_nil"))
	assert.Equal(t, 503, int(h.capture.Number("status")))
	assert.Equal(t, "down for maintenance", h.capture.String("body"))
}

func TestHTTPModule_MaxBytesEnforced(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("a", 2000)
	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			return jsonResponse(200, big, nil), nil
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.get("https://example.com", { max_bytes = 100 }, function(res, err)
    hive.test_capture("res", res == nil and "nil" or "set")
    hive.test_capture("err", err == nil and "" or err)
  end)
end
`, client)

	waitForKey(t, h.capture, "err")
	assert.Equal(t, "nil", h.capture.String("res"), "response should be nil when cap exceeded")
	assert.Contains(t, h.capture.String("err"), "max_bytes")
}

func TestHTTPModule_PerCallTimeoutCancelsCtx(t *testing.T) {
	t.Parallel()

	captureCtx := make(chan context.Context, 1)
	released := make(chan struct{})

	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			captureCtx <- req.Context()
			<-req.Context().Done()
			close(released)
			return nil, req.Context().Err()
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  hive.http.get("https://example.com", { timeout = 0.05 }, function(res, err)
    hive.test_capture("res", res == nil and "nil" or "set")
    hive.test_capture("err", err == nil and "" or err)
  end)
end
`, client)

	select {
	case ctx := <-captureCtx:
		select {
		case <-released:
			require.Error(t, ctx.Err())
		case <-time.After(2 * time.Second):
			t.Fatalf("request never released after timeout")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("request never received a context")
	}

	waitForKey(t, h.capture, "err")
	assert.Equal(t, "nil", h.capture.String("res"))
	assert.NotEmpty(t, h.capture.String("err"))
}

func TestHTTPModule_HandleCancelStopsRequest(t *testing.T) {
	t.Parallel()

	captureCtx := make(chan context.Context, 1)
	released := make(chan struct{})

	client := &fakeHTTPClient{
		respond: func(req *http.Request) (*http.Response, error) {
			captureCtx <- req.Context()
			<-req.Context().Done()
			close(released)
			return nil, req.Context().Err()
		},
	}

	h := newHTTPHarness(t, `
return function(hive)
  HANDLE = hive.http.get("https://example.com", function(res, err) hive.test_capture(res, err) end)
end
`, client)

	var ctx context.Context
	select {
	case ctx = <-captureCtx:
	case <-time.After(2 * time.Second):
		t.Fatalf("request never received a context")
	}

	cancelDone := make(chan struct{})
	h.runtime.Submit(func(state *glua.LState) {
		defer close(cancelDone)
		ud, ok := state.GetGlobal("HANDLE").(*glua.LUserData)
		require.True(t, ok)
		handle, ok := ud.Value.(*asyncHandle)
		require.True(t, ok)
		handle.Cancel()
	})
	<-cancelDone

	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatalf("request never released after cancel")
	}
	require.Error(t, ctx.Err())

	// Cancellation prevents the dispatch path from loading the registry
	// pin, so the callback should never fire.
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, h.capture.Snapshot())
}

func TestHTTPModule_RequestRequiresURL(t *testing.T) {
	t.Parallel()

	client := &fakeHTTPClient{}
	h := newHTTPHarness(t, `
return function(hive)
  local ok, err = pcall(hive.http.request, { method = "GET" }, function() end)
  hive.test_capture("ok", tostring(ok))
  hive.test_capture("err", tostring(err))
end
`, client)

	assert.Equal(t, "false", h.capture.String("ok"))
	assert.Contains(t, h.capture.String("err"), "url")
	assert.Equal(t, 0, client.requestCount())
}

func TestHTTPModule_VerbsRequireCallback(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"get", "post", "put", "delete"} {
		t.Run(verb, func(t *testing.T) {
			client := &fakeHTTPClient{}
			h := newHTTPHarness(t, fmt.Sprintf(`
return function(hive)
  local ok = pcall(hive.http.%s, "https://example.com")
  hive.test_capture("ok", tostring(ok))
end
`, verb), client)

			assert.Equal(t, "false", h.capture.String("ok"))
			assert.Equal(t, 0, client.requestCount())
		})
	}
}

func TestHTTPModule_HeaderTypeRejected(t *testing.T) {
	t.Parallel()

	client := &fakeHTTPClient{}
	h := newHTTPHarness(t, `
return function(hive)
  local ok, err = pcall(hive.http.get, "https://example.com",
    { headers = { Bad = {1, 2} } },
    function() end)
  hive.test_capture("ok", tostring(ok))
  hive.test_capture("err", tostring(err))
end
`, client)

	assert.Equal(t, "false", h.capture.String("ok"))
	assert.Contains(t, h.capture.String("err"), "header")
	assert.Equal(t, 0, client.requestCount())
}

// waitForKey polls for a keyed capture entry with a generous CI-friendly
// timeout. Use when tests need to await a single named callback rather
// than an N-element list (waitForCaptures handles the latter).
func waitForKey(t *testing.T, c *captureModule, key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if c.Has(key) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected capture key %q within 2s", key)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
