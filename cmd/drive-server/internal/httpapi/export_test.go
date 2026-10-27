package httpapi

import (
	"net/http"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/web/mid"
	"github.com/colonyops/hive/pkg/buildinfo"
)

type patternRecorder []string

func (p *patternRecorder) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	*p = append(*p, pattern)
}

// RoutePatterns lists every pattern New registers.
func RoutePatterns(svc Services) []string {
	var rec patternRecorder
	registerRoutes(&rec, errchain.New(mid.Errors(zerolog.Nop(), mapError)), buildinfo.Info{}, svc)
	return rec
}
