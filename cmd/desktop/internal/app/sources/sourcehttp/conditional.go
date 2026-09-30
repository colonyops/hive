package sourcehttp

import (
	"net/http"

	"github.com/hay-kot/appkit/httpclient"
)

// Validators are the cache validators carried between a response and the next
// conditional request for the same resource. An authenticated 304 is free of
// rate-limit cost on most APIs, so polling an unchanged resource costs
// nothing.
type Validators struct {
	ETag         string
	LastModified string
}

func ReadValidators(h http.Header) Validators {
	return Validators{
		ETag:         h.Get("ETag"),
		LastModified: h.Get("Last-Modified"),
	}
}

func (v Validators) Empty() bool { return v.ETag == "" && v.LastModified == "" }

// Apply makes a request conditional. Both headers are sent when both
// validators are held; RFC 9110 has the server prefer If-None-Match.
func (v Validators) Apply(h http.Header) {
	if v.ETag != "" {
		h.Set("If-None-Match", v.ETag)
	}
	if v.LastModified != "" {
		h.Set("If-Modified-Since", v.LastModified)
	}
}

// Conditional makes one request conditional on v.
func Conditional(v Validators) httpclient.Middleware {
	return func(next httpclient.Doer) httpclient.Doer {
		return httpclient.DoerFunc(func(req *http.Request) (*http.Response, error) {
			v.Apply(req.Header)
			return next.Do(req)
		})
	}
}

// NotModified must be checked before [Errors.Status], which classifies a 304
// as a failure like any other non-2xx.
func NotModified(resp *http.Response) bool {
	return resp.StatusCode == http.StatusNotModified
}
