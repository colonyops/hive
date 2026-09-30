package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

type Ingester interface {
	IngestObservation(ctx context.Context, classifier models.Classifier, p stores.IngestObservationParams) (stores.IngestResult, error)
}

// SnapshotAppender persists authoritative source state after a delivery
// changes an item, so replay can restore feed claims.
type SnapshotAppender interface {
	AppendSnapshot(ctx context.Context, topic, sourceKind, sourceScope string, items []models.SnapshotItem) (offset int64, err error)
}

type CaptureStore interface {
	Upsert(ctx context.Context, topic string, receivedAt int64, body []byte) error
}

type InboxItemLister interface {
	ListUnarchivedBySource(ctx context.Context, profileID, sourceKind, sourceScope string) ([]stores.InboxItem, error)
}

// maxBodyBytes caps a webhook request body. Payloads are stored verbatim as
// the inbox item payload and in webhook_capture, so the cap bounds both.
const maxBodyBytes = 1 << 20

// Instances resolves every live webhook source instance. It is called per
// request rather than fixed at construction, so adding, editing, or removing
// a webhook source node takes effect without a restart — the same
// late-binding posture as the producer's per-tick source resolution.
//
// It is a function of connector instances rather than of flows so this
// package never imports the flow package: the flow set is walked by whoever
// owns the registry, and the connector only sees its own instances.
type Instances func(context.Context) []connector.Instance

// Listener is the desktop's local webhook ingress: a localhost-only HTTP
// server whose /hooks/<path> routes are resolved per request. It bypasses the
// poll producer entirely — a delivery calls IngestObservation directly, the
// production source boundary, then appends the topic's authoritative snapshot
// so membership replay keeps webhook-fed feeds intact across deploys and
// restarts.
type Listener struct {
	ingester  Ingester
	snapshots SnapshotAppender
	captures  CaptureStore
	items     InboxItemLister
	instances Instances
	notifier  LogAppendNotifier
	logger    zerolog.Logger
	recorder  activity.Recorder

	host     string
	port     int
	server   *http.Server
	listener net.Listener
	startErr error

	mounts []mount

	stopOnce sync.Once
}

type mount struct {
	prefix  string
	handler http.Handler
}

// LogAppendNotifier must wake the flow engine synchronously. A bus event is
// insufficient because subscribers coalesce bursts and could delay routing.
type LogAppendNotifier interface {
	PublishLogAppended(nextOffset int64)
}

// NewListener assumes host has passed loopback-only configuration validation.
func NewListener(ingester Ingester, snapshots SnapshotAppender, captures CaptureStore, items InboxItemLister, instances Instances, host string, port int, notifier LogAppendNotifier, logger zerolog.Logger) *Listener {
	return &Listener{
		ingester: ingester, snapshots: snapshots, captures: captures, items: items,
		instances: instances, host: host, port: port, notifier: notifier, logger: logger,
	}
}

// SetRecorder attaches an activity recorder so ingest failures surface in the
// Activity view. Set once at wiring time, before Start.
func (l *Listener) SetRecorder(r activity.Recorder) { l.recorder = r }

// Start binds the configured loopback host and serves in a goroutine. A bind failure (port in
// use) is returned to the caller, which logs and continues — a busy webhook
// port must never take the desktop app down with it — and is retained for
// StartError so settings can surface it instead of leaving it in the log.
func (l *Listener) Start(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort(l.host, strconv.Itoa(l.port)))
	if err != nil {
		l.startErr = fmt.Errorf("webhook listener: %w", err)
		return l.startErr
	}
	l.listener = ln
	l.server = &http.Server{Handler: l.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := l.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.logger.Error().Err(err).Msg("webhook listener stopped unexpectedly")
		}
	}()
	l.logger.Info().Str("host", l.host).Int("port", l.Port()).Msg("webhook listener started")
	return nil
}

// Stop gracefully shuts the server down, letting in-flight ingests finish
// until ctx is done. Idempotent: a second call, or a call when Start was
// never invoked or never bound, is a no-op that returns nil.
//
// It takes ctx rather than owning a timeout itself so the caller supplies the
// shutdown budget — app.go's plugs.Plugin wrapper derives one with
// context.WithoutCancel, since by the time a plugin's cleanup runs its own
// ctx is already Done. Whether a shutdown error is fatal is that caller's
// policy to decide, the same way Start's bind error is: this method only
// reports, it does not judge.
func (l *Listener) Stop(ctx context.Context) error {
	var err error
	l.stopOnce.Do(func() {
		if l.server == nil {
			return
		}
		err = l.server.Shutdown(ctx)
	})
	return err
}

// Running reports whether Start succeeded and the listener is bound.
func (l *Listener) Running() bool { return l.listener != nil }

// StartError returns why Start failed to bind, or nil if it never failed.
func (l *Listener) StartError() error { return l.startErr }

// Port returns the bound TCP port once Running, else the configured port.
// They differ only when the listener was constructed with port 0 (tests).
func (l *Listener) Port() int {
	if l.listener != nil {
		if addr, ok := l.listener.Addr().(*net.TCPAddr); ok {
			return addr.Port
		}
	}
	return l.port
}

// Host returns the actual bound address once running, else the configured host.
func (l *Listener) Host() string {
	if l.listener != nil {
		if addr, ok := l.listener.Addr().(*net.TCPAddr); ok {
			return addr.IP.String()
		}
	}
	return l.host
}

// MountAPI mounts an additional handler at prefix so a driving adapter can
// share the loopback port. Call it before Start: Handler() is built once there,
// so a later mount is silently dropped.
func (l *Listener) MountAPI(prefix string, h http.Handler) {
	if l.server != nil {
		l.logger.Warn().Str("prefix", prefix).Msg("MountAPI called after Start; handler will not be served")
		return
	}
	l.mounts = append(l.mounts, mount{prefix: prefix, handler: h})
}

// Handler returns the listener's route handler. Exposed (rather than only
// being installed by Start) so tests can drive deliveries through httptest
// without binding a real port.
func (l *Listener) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(PathPrefix, l.handleHook)
	for _, m := range l.mounts {
		mux.Handle(m.prefix, m.handler)
	}
	return mux
}

// target is one enabled webhook source node matched by path.
type target struct {
	instance connector.Instance
	secret   string
}

// resolveTargets returns every enabled webhook source instance declaring
// path. Several may share a path — each receives the delivery, so one sender
// can fan into multiple flows.
func (l *Listener) resolveTargets(ctx context.Context, path string) []target {
	var out []target
	for _, inst := range l.instances(ctx) {
		cfg, ok := inst.Config.(*Config)
		if !ok || cfg.Path != path {
			continue
		}
		out = append(out, target{instance: inst, secret: cfg.Secret})
	}
	return out
}

func (l *Listener) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, PathPrefix)
	targets := l.resolveTargets(r.Context(), path)
	if len(targets) == 0 {
		http.Error(w, "no webhook endpoint at this path", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			http.Error(w, "request body exceeds 1 MiB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "reading request body", http.StatusBadRequest)
		return
	}
	if !json.Valid(body) {
		http.Error(w, "request body must be valid JSON", http.StatusBadRequest)
		return
	}

	secret := r.Header.Get(SecretHeader)
	authorized := targets[:0:0]
	for _, t := range targets {
		if t.secret == "" || subtle.ConstantTimeCompare([]byte(t.secret), []byte(secret)) == 1 {
			authorized = append(authorized, t)
		}
	}
	if len(authorized) == 0 {
		http.Error(w, "invalid or missing "+SecretHeader+" header", http.StatusUnauthorized)
		return
	}

	now := time.Now().UnixMilli()
	key, title, url := identity(path, body)

	delivered := 0
	var lastOffset int64
	for _, t := range authorized {
		offset, err := l.ingest(r.Context(), t.instance, key, title, url, body, now)
		if err != nil {
			l.logger.Error().Err(err).Str("topic", t.instance.Node.Topic()).Msg("webhook ingest failed")
			l.record(r.Context(), activity.RefreshFailed(t.instance.Node.ID(), err.Error()))
			continue
		}
		delivered++
		if offset > lastOffset {
			lastOffset = offset
		}
	}
	if delivered == 0 {
		http.Error(w, "ingest failed", http.StatusInternalServerError)
		return
	}
	if lastOffset > 0 && l.notifier != nil {
		l.notifier.PublishLogAppended(lastOffset)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]int{"delivered": delivered})
}

// ingest persists one delivery for one instance: the observation (which
// appends the routed event-log row when the payload changed), the capture row
// (always — it is "the last request", independent of deduplication), and,
// after a write, the topic's complete authoritative snapshot so
// startup/deploy membership replay resolves this source's feed claims. It
// returns the offset of the last event-log row it appended (0 when the
// payload was unchanged).
func (l *Listener) ingest(ctx context.Context, inst connector.Instance, key, title, url string, body []byte, now int64) (int64, error) {
	topic := inst.Node.Topic()
	meta := inst.Metadata

	result, err := l.ingester.IngestObservation(ctx, inst.Classifier, stores.IngestObservationParams{
		ProfileID: meta.ProfileID,
		Topic:     topic,
		Policy:    meta.Policy,
		Current: models.Observation{
			ExternalID:  key,
			Title:       title,
			URL:         url,
			SourceKind:  meta.SourceKind,
			SourceScope: meta.SourceScope,
			ObservedAt:  now,
			Payload:     body,
		},
	})
	if err != nil {
		return 0, fmt.Errorf("ingesting webhook observation %q: %w", key, err)
	}

	if err := l.captures.Upsert(ctx, topic, now, body); err != nil {
		// The capture only powers editor affordances; losing it must not
		// fail a delivery that already ingested.
		l.logger.Warn().Err(err).Str("topic", topic).Msg("webhook capture write failed")
	}

	if !result.Wrote {
		return 0, nil
	}

	rows, err := l.items.ListUnarchivedBySource(ctx, meta.ProfileID, meta.SourceKind, meta.SourceScope)
	if err != nil {
		return result.Offset, fmt.Errorf("listing webhook snapshot items for %q: %w", topic, err)
	}
	items := make([]models.SnapshotItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, models.SnapshotItem{Key: row.ExternalID, Payload: row.Payload})
	}
	offset, err := l.snapshots.AppendSnapshot(ctx, topic, meta.SourceKind, meta.SourceScope, items)
	if err != nil {
		return result.Offset, fmt.Errorf("appending webhook snapshot for %q: %w", topic, err)
	}
	return offset, nil
}

func (l *Listener) record(ctx context.Context, event activity.Event) {
	if l.recorder == nil {
		return
	}
	l.recorder.Record(ctx, event)
}
