//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLuaPluginHTTPModule_GetPostStatus verifies that hive.http.{get,post}
// reach a real HTTP server through the Lua runtime end-to-end.
//
// The plugin entrypoint runs during plugin init (any hive command triggers
// it). The first call hits /ok and writes the status code; the second hits
// /echo with a JSON body and writes both the status and the echoed body.
// Plugin shutdown drains in-flight async work, so the callbacks all run
// before `hive config` returns.
func TestLuaPluginHTTPModule_GetPostStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Header().Set("X-Echo", "hello")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("plain-ok"))
		case "/echo":
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(body)
		case "/error":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	h := NewHarness(t)

	outputFile := filepath.Join(h.DataDir(), "http-output.txt")

	entry := filepath.Join(h.DataDir(), "lua", "plugins", "init.lua")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
	require.NoError(t, os.WriteFile(entry, []byte(fmt.Sprintf(`
local OUT = %q
local BASE = %q

return function(hive)
  hive.http.get(BASE .. "/ok", function(res, err)
    if err ~= nil then error("get /ok failed: " .. err) end
    hive.http.post(BASE .. "/echo", { body = "ping" }, function(post, postErr)
      if postErr ~= nil then error("post /echo failed: " .. postErr) end
      hive.http.get(BASE .. "/error", function(errRes, errErr)
        if errErr ~= nil then error("get /error failed: " .. errErr) end
        local summary = string.format(
          "ok_status=%%d|ok_echo=%%s|ok_body=%%s|post_status=%%d|post_body=%%s|err_status=%%d|err_body=%%s",
          res.status, res.headers["X-Echo"], res.body,
          post.status, post.body,
          errRes.status, errRes.body
        )
        hive.sh.run(string.format("printf %%%%s '%%s' > %%s", summary, OUT), function(_) end)
      end)
    end)
  end)
end
`, outputFile, server.URL)), 0o644))

	h.WithConfig(fmt.Sprintf(`
plugins:
  lua:
    entry: %q
`, entry))

	_, err := h.RunStdout("config")
	require.NoError(t, err)

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err, "summary file should be created by the lua plugin entrypoint")

	got := string(content)
	assert.Contains(t, got, "ok_status=200")
	assert.Contains(t, got, "ok_echo=hello")
	assert.Contains(t, got, "ok_body=plain-ok")
	assert.Contains(t, got, "post_status=201")
	assert.Contains(t, got, "post_body=ping")
	// Non-2xx responses are still successes — the plugin sees the status.
	assert.Contains(t, got, "err_status=500")
	assert.Contains(t, got, "err_body=boom")
}
