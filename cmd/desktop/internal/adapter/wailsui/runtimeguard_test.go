package wailsui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGuardWailsRuntime(t *testing.T) {
	const call = `/wails/runtime?object=0&method=0&args={"methodID":2190116655}`

	tests := []struct {
		name    string
		method  string
		target  string
		headers map[string]string
		want    int
	}{
		{name: "an image in a foreign page", method: http.MethodGet, target: call, want: http.StatusForbidden},
		{name: "a GET that claims the client header", method: http.MethodGet, target: call, headers: map[string]string{"x-wails-client-id": "x"}, want: http.StatusForbidden},
		{name: "a no-cors POST, which loses the header", method: http.MethodPost, target: wailsRuntimePath, want: http.StatusForbidden},
		{name: "a sandboxed frame with the header", method: http.MethodPost, target: wailsRuntimePath, headers: map[string]string{"x-wails-client-id": "x", "Origin": "null"}, want: http.StatusForbidden},
		{name: "another site with the header", method: http.MethodPost, target: wailsRuntimePath, headers: map[string]string{"x-wails-client-id": "x", "Origin": "https://evil.example"}, want: http.StatusForbidden},
		{name: "another port with the header", method: http.MethodPost, target: wailsRuntimePath, headers: map[string]string{"x-wails-client-id": "x", "Origin": "http://example.com:9999"}, want: http.StatusForbidden},
		{name: "the app in WKWebView", method: http.MethodPost, target: wailsRuntimePath, headers: map[string]string{"x-wails-client-id": "x"}, want: http.StatusTeapot},
		{name: "the app in a browser", method: http.MethodPost, target: wailsRuntimePath, headers: map[string]string{"x-wails-client-id": "x", "Origin": "http://example.com"}, want: http.StatusTeapot},
		{name: "another path", method: http.MethodGet, target: "/wails/runtime.js", want: http.StatusTeapot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			})
			request := httptest.NewRequest(tt.method, tt.target, strings.NewReader(""))
			for name, value := range tt.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			guardWailsRuntime(next).ServeHTTP(recorder, request)
			assert.Equal(t, tt.want, recorder.Code)
		})
	}
}
