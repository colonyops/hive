// Package web holds the HTTP plumbing shared by the JSON APIs in this repo:
// the error wire shape, build identity, and the version handler.
package web

import (
	"net/http"
	"runtime/debug"

	"github.com/hay-kot/httpkit/server"
)

// ErrorBody is the error wire shape: {kind, message}, with per-field detail
// on validation failures.
type ErrorBody struct {
	Kind    string            `json:"kind"             jsonschema:"enum=invalid,enum=not_found,enum=conflict,enum=unauthenticated,enum=unavailable,enum=internal,description=Stable machine-readable error category."`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty" jsonschema:"description=Per-field validation messages keyed by field name; present on validation failures."`
}

// BadRequestError marks a body the transport could not read (400), as
// distinct from a well-formed request that failed validation (422).
type BadRequestError struct {
	Msg string
	Err error
}

func (e *BadRequestError) Error() string {
	if e.Err == nil {
		return e.Msg
	}
	return e.Msg + ": " + e.Err.Error()
}

func (e *BadRequestError) Unwrap() error { return e.Err }

// Build is the VCS identity Go stamps into a binary. Revision is empty under
// -buildvcs=false, which reads as an unknown build.
type Build struct {
	Revision string `json:"revision"`
	Modified bool   `json:"modified"`
	Time     string `json:"time"`
	Go       string `json:"go"`
}

// ReadBuild reads the build identity from the running binary.
func ReadBuild() Build {
	var b Build
	if info, ok := debug.ReadBuildInfo(); ok {
		b.Go = info.GoVersion
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				b.Revision = s.Value
			case "vcs.modified":
				b.Modified = s.Value == "true"
			case "vcs.time":
				b.Time = s.Value
			}
		}
	}
	return b
}

// VersionHandler serves {service, ...Build} so a caller can confirm which build
// is running.
func VersionHandler(service string) http.HandlerFunc {
	type resp struct {
		Service string `json:"service"`
		Build   Build  `json:"build"`
	}

	build := ReadBuild()

	return func(w http.ResponseWriter, _ *http.Request) {
		_ = server.JSON(w, http.StatusOK, resp{Service: service, Build: build})
	}
}
