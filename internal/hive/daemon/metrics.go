package daemon

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/session"
)

// metrics uses a private registry, not observe.Meter: depguard keeps the OTel
// SDK, and so every exporter, inside Desktop telemetry (ADR on the daemon).
type metrics struct {
	registry     *prometheus.Registry
	rpcRequests  *prometheus.CounterVec
	rpcDuration  *prometheus.HistogramVec
	loopDuration prometheus.Histogram
	loopErrors   prometheus.Counter
}

func newMetrics(logger zerolog.Logger, startedAt time.Time, list func(ctx context.Context) ([]session.Session, error)) *metrics {
	m := &metrics{
		registry: prometheus.NewRegistry(),
		rpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "hive_daemon_rpc_requests_total",
			Help: "RPCs the daemon finished, by procedure and Connect code.",
		}, []string{"procedure", "code"}),
		rpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "hive_daemon_rpc_duration_seconds",
			Help:    "Time the daemon took to answer an RPC.",
			Buckets: prometheus.DefBuckets,
		}, []string{"procedure"}),
		loopDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "hive_daemon_status_loop_duration_seconds",
			Help:    "Time one status loop tick took.",
			Buckets: prometheus.DefBuckets,
		}),
		loopErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "hive_daemon_status_loop_errors_total",
			Help: "Status loop ticks that failed.",
		}),
	}
	start := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "hive_daemon_start_time_seconds",
		Help: "Unix time the daemon started.",
	})
	start.Set(float64(startedAt.Unix()))
	m.registry.MustRegister(
		m.rpcRequests, m.rpcDuration, m.loopDuration, m.loopErrors, start,
		&sessionsCollector{logger: logger, list: list},
	)
	return m
}

func (m *metrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *metrics) observeRPC(procedure string, began time.Time, err error) {
	code := "ok"
	if err != nil {
		code = connect.CodeOf(err).String()
	}
	m.rpcRequests.WithLabelValues(procedure, code).Inc()
	m.rpcDuration.WithLabelValues(procedure).Observe(time.Since(began).Seconds())
}

func (m *metrics) interceptor() connect.Interceptor { return rpcInterceptor{m} }

type rpcInterceptor struct{ m *metrics }

func (i rpcInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		began := time.Now()
		resp, err := next(ctx, req)
		i.m.observeRPC(req.Spec().Procedure, began, err)
		return resp, err
	}
}

func (i rpcInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i rpcInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		began := time.Now()
		err := next(ctx, conn)
		i.m.observeRPC(conn.Spec().Procedure, began, err)
		return err
	}
}

// sessionsCollector counts sessions at scrape time, so the gauge cannot drift
// from the store when another process writes hive.db.
type sessionsCollector struct {
	logger zerolog.Logger
	list   func(ctx context.Context) ([]session.Session, error)
}

var sessionsDesc = prometheus.NewDesc("hive_daemon_sessions", "Sessions in the store, by state.", []string{"state"}, nil)

func (c *sessionsCollector) Describe(ch chan<- *prometheus.Desc) { ch <- sessionsDesc }

func (c *sessionsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Second)
	defer cancel()
	sessions, err := c.list(ctx)
	if err != nil {
		c.logger.Warn().Err(err).Msg("metrics: list sessions")
		return
	}
	counts := map[session.State]int{}
	for _, state := range []session.State{session.StateActive, session.StateRecycled, session.StateCorrupted} {
		counts[state] = 0
	}
	for _, sess := range sessions {
		counts[sess.State]++
	}
	for state, n := range counts {
		ch <- prometheus.MustNewConstMetric(sessionsDesc, prometheus.GaugeValue, float64(n), string(state))
	}
}
