package main

import (
	"net/http"
	"sync"

	"github.com/hay-kot/httpkit/errchain"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/devproxy"
	"github.com/colonyops/hive/internal/web"
	"github.com/colonyops/hive/internal/web/mid"
)

// Control is the devserver's own API: everything that drives the simulation,
// as opposed to the proxy's GitHub-shaped surface. It is mounted under /_ctl/
// so it cannot collide with a GitHub path.
type Control struct {
	cfg    Config
	store  *Store
	cache  *Cache
	proxy  *Proxy
	pusher *Pusher
	logger zerolog.Logger

	mu sync.Mutex
	// scenarios holds the step count of each in-flight scenario, keyed by its
	// generated name and cleared when the run ends. Presence means running, so
	// a poller sees a scenario finish when it disappears.
	scenarios map[string]int
	seq       int
}

func NewControl(cfg Config, store *Store, cache *Cache, proxy *Proxy, pusher *Pusher, logger zerolog.Logger) *Control {
	return &Control{
		cfg: cfg, store: store, cache: cache, proxy: proxy, pusher: pusher,
		logger: logger, scenarios: make(map[string]int),
	}
}

// Handler returns the control routes.
func (c *Control) Handler() http.Handler {
	chain := errchain.New(mid.Errors(c.logger, mapControlError))

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+devproxy.HealthPath, chain.ToHandlerFunc(c.Health))
	mux.Handle("GET /_ctl/version", web.VersionHandler("hive devserver"))
	mux.HandleFunc("GET /_ctl/state", chain.ToHandlerFunc(c.State))
	mux.HandleFunc("POST /_ctl/overlay", chain.ToHandlerFunc(c.SetOverlay))
	mux.HandleFunc("POST /_ctl/overlay/clear", chain.ToHandlerFunc(c.ClearOverlay))
	mux.HandleFunc("POST /_ctl/overlays/clear", chain.ToHandlerFunc(c.ClearAllOverlays))
	mux.HandleFunc("POST /_ctl/action", chain.ToHandlerFunc(c.Action))
	mux.HandleFunc("POST /_ctl/scenario", chain.ToHandlerFunc(c.RunScenario))
	mux.HandleFunc("POST /_ctl/webhooks/push", chain.ToHandlerFunc(c.Push))
	mux.HandleFunc("POST /_ctl/cache/purge", chain.ToHandlerFunc(c.PurgeCache))
	return mid.Logger(c.logger, devproxy.HealthPath, "/_ctl/state", "/_ctl/version")(mux)
}

// mapControlError keeps the cause on the wire — devserver is local dev
// tooling, so an opaque 500 would just send the author to the logs.
func mapControlError(err error) (int, any, bool) {
	return http.StatusInternalServerError, web.ErrorBody{Kind: "internal", Message: err.Error()}, true
}
