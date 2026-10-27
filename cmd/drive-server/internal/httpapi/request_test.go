package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type call struct {
	method, target, body string
	token                string
	cookie               *http.Cookie
	header               map[string]string
}

func serve(h http.Handler, c call) *httptest.ResponseRecorder {
	req := httptest.NewRequest(c.method, c.target, strings.NewReader(c.body))
	req.Host = "drive.test"
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	for k, v := range c.header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func (s *server) send(c call) *httptest.ResponseRecorder { return serve(s.h, c) }

func (s *server) do(method, target, token, body string) *httptest.ResponseRecorder {
	return s.send(call{method: method, target: target, token: token, body: body})
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}
