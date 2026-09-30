package httpapi

import (
	"net/http"
	"net/http/pprof"
)

// PprofPathPrefix is where PprofHandler mounts on the shared loopback server.
const PprofPathPrefix = "/debug/pprof/"

// PprofHandler serves net/http/pprof on a private mux. cpuProfile replaces the
// standard CPU handler when continuous profiling needs to share the profiler.
func PprofHandler(cpuProfile http.Handler) http.Handler {
	if cpuProfile == nil {
		cpuProfile = http.HandlerFunc(pprof.Profile)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.Handle("/debug/pprof/profile", cpuProfile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}
