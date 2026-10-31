package wailsui

import (
	"net/http"
	"net/url"
)

const wailsRuntimePath = "/wails/runtime"

// guardWailsRuntime refuses a bound-method call that did not come from the app's own page.
// Wails runs one on any method, and reads it from the query string when the body is empty,
// so an image in a page an applet's frame navigated to would otherwise run hive.exec with
// no bridge token (ADR applets-are-web-pages-with-full-access-to-the-machine).
func guardWailsRuntime(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == wailsRuntimePath && !isAppRuntimeCall(r) {
			http.Error(w, "this call did not come from Hive", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isAppRuntimeCall needs all three checks. WKWebView sends no preflight to a custom scheme,
// so a foreign page's custom header does reach here, and only its Origin gives it away: a
// sandboxed frame's is "null". A no-cors request has no Origin, but loses the custom header.
// The app's own POST has no Origin in WKWebView, and its own host in a browser.
func isAppRuntimeCall(r *http.Request) bool {
	if r.Method != http.MethodPost || r.Header.Get("x-wails-client-id") == "" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && parsed.Host == r.Host
}
