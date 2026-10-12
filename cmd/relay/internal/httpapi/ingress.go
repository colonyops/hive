package httpapi

import (
	"errors"
	"io"
	"net/http"
	"net/netip"
	"time"

	"github.com/hay-kot/httpkit/server"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/relay/internal/guard"
	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

const (
	limiterKeys = 10_000
	limiterIdle = 10 * time.Minute
)

// ingressHandler takes any request on /in/{token}[/...]. It sits outside the
// error chain because senders are not API clients: every rejection is plain
// text, and every guard runs before the body is read.
type ingressHandler struct {
	ingress   *relay.Ingress
	limits    relay.Limits
	trusted   []netip.Prefix
	sem       *guard.Semaphore
	perIP     *guard.Limiter
	perSource *guard.Limiter
	log       zerolog.Logger
}

func newIngressHandler(core Core, trusted []netip.Prefix, log zerolog.Logger) *ingressHandler {
	return &ingressHandler{
		ingress:   core.Ingress,
		limits:    core.Limits,
		trusted:   trusted,
		sem:       guard.NewSemaphore(core.Limits.Concurrency),
		perIP:     guard.NewLimiter(core.Limits.IngressPerIP, limiterKeys, limiterIdle, nil),
		perSource: guard.NewLimiter(core.Limits.IngressPerSource, limiterKeys, limiterIdle, nil),
		log:       log,
	}
}

type ingressResponse struct {
	ID string `json:"id"`
}

func (h *ingressHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip := guard.ClientIP(r, h.trusted)

	release, ok := h.sem.TryAcquire()
	if !ok {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "relay is busy", http.StatusServiceUnavailable)
		return
	}
	defer release()

	if ok, wait := h.perIP.Allow(ip.String()); !ok {
		tooMany(w, wait)
		return
	}
	if r.ContentLength > h.limits.BodyBytes {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.limits.BodyBytes)

	token := r.PathValue("token")
	src, err := h.ingress.ResolveSource(r.Context(), token, presentedSecret(r))
	if err != nil {
		h.reject(w, r, err)
		return
	}
	if ok, wait := h.perSource.Allow(src.ID); !ok {
		tooMany(w, wait)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "could not read body", http.StatusBadRequest)
		return
	}

	path := ""
	if rest := r.PathValue("rest"); rest != "" {
		path = "/" + rest
	}
	rec := relay.Record{
		Method:      r.Method,
		Path:        path,
		Query:       r.URL.Query(),
		Headers:     relay.RedactHeaders(r.Header, h.limits.HeaderBytes),
		Body:        body,
		ContentType: r.Header.Get("Content-Type"),
		Remote:      ip.String(),
	}
	id, err := h.ingress.AcceptSource(r.Context(), src, rec)
	if err != nil {
		h.reject(w, r, err)
		return
	}
	_ = server.JSON(w, http.StatusOK, ingressResponse{ID: id})
}

func (h *ingressHandler) reject(w http.ResponseWriter, r *http.Request, err error) {
	var e *relay.Error
	if !errors.As(err, &e) {
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("ingress failed")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	status, ok := statusForKind(e.Kind)
	if !ok {
		h.log.Error().Err(err).Str("path", r.URL.Path).Msg("ingress failed with an unmapped kind")
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if e.Kind == relay.KindRateLimited && e.RetryAfter > 0 {
		w.Header().Set("Retry-After", retrySeconds(e.RetryAfter))
	}
	msg := e.Message
	if msg == "" {
		msg = string(e.Kind)
	}
	http.Error(w, msg, status)
}

func tooMany(w http.ResponseWriter, wait time.Duration) {
	w.Header().Set("Retry-After", retrySeconds(wait))
	http.Error(w, "rate limited", http.StatusTooManyRequests)
}

func presentedSecret(r *http.Request) string {
	if v := r.Header.Get("X-Hive-Secret"); v != "" {
		return v
	}
	if v := r.Header.Get("X-Gotify-Key"); v != "" {
		return v
	}
	return bearer(r)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func logIngress(log zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)

		path := "/in/<token>"
		if rest := r.PathValue("rest"); rest != "" {
			path += "/" + rest
		}
		log.Info().
			Str("method", r.Method).
			Str("path", path).
			Int("status", rec.status).
			Dur("duration", time.Since(start)).
			Msg("request complete")
	})
}
