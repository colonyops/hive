package sourcehttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// The taxonomy the app acts on: unauthorized drives re-auth, rate limited and
// unreachable drive fetch cooldowns. A connector classifies into these three
// and gets both behaviours without the app knowing its name.
var (
	ErrUnauthorized = errors.New("source: unauthorized")
	ErrRateLimited  = errors.New("source: rate limited")
	ErrUnreachable  = errors.New("source: unreachable")
)

// bodyPeek bounds how much of an error response is read for its message.
const bodyPeek = 512

// RateLimitError carries the server's retry time. It unwraps to
// ErrRateLimited, so errors.Is keeps working for callers that cannot act on
// the deadline.
type RateLimitError struct {
	// ResetAt is zero when the server sent neither Retry-After nor
	// X-RateLimit-Reset.
	ResetAt time.Time
}

func (e *RateLimitError) Error() string {
	if e.ResetAt.IsZero() {
		return ErrRateLimited.Error()
	}
	return fmt.Sprintf("%s until %s", ErrRateLimited, e.ResetAt.Format(time.RFC3339))
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// ForbiddenClassifier reports whether a 403 is a rate limit rather than a
// permission failure. Only providers that overload the status need one.
type ForbiddenClassifier func(resp *http.Response, body []byte) bool

// Errors maps one provider's responses onto the taxonomy.
type Errors struct {
	Name      string
	Forbidden ForbiddenClassifier
}

func (e Errors) prefix() string {
	if e.Name == "" {
		return "source"
	}
	return e.Name
}

// Unreachable wraps a transport failure.
func (e Errors) Unreachable(err error) error {
	return fmt.Errorf("%s: %w: %w", e.prefix(), ErrUnreachable, err)
}

// Errorf builds a provider-prefixed error.
func (e Errors) Errorf(format string, args ...any) error {
	return fmt.Errorf("%s: %w", e.prefix(), fmt.Errorf(format, args...))
}

// Status maps a response onto the taxonomy, returning nil for 2xx. It reads
// and closes nothing on success; on failure it consumes up to bodyPeek bytes
// for the message.
func (e Errors) Status(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return RateLimit(resp.Header)
	case resp.StatusCode == http.StatusForbidden:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, bodyPeek))
		if e.Forbidden != nil && e.Forbidden(resp, body) {
			return RateLimit(resp.Header)
		}
		return e.statusErrorf(resp, body)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, bodyPeek))
		return e.statusErrorf(resp, body)
	}
}

func (e Errors) statusErrorf(resp *http.Response, body []byte) error {
	return fmt.Errorf("%s: %s: %s", e.prefix(), requestLabel(resp), summarize(resp.StatusCode, body))
}

// RateLimit extracts the server's wait hint. Retry-After wins when present:
// it describes an active secondary-limit penalty, where X-RateLimit-Reset is
// the primary limit's epoch reset.
func RateLimit(h http.Header) *RateLimitError {
	if seconds, err := strconv.Atoi(h.Get("Retry-After")); err == nil && seconds >= 0 {
		return &RateLimitError{ResetAt: time.Now().Add(time.Duration(seconds) * time.Second)}
	}
	if epoch, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil && epoch >= 0 {
		return &RateLimitError{ResetAt: time.Unix(epoch, 0)}
	}
	return &RateLimitError{}
}

func requestLabel(resp *http.Response) string {
	if resp.Request == nil || resp.Request.URL == nil {
		return "request"
	}
	return resp.Request.Method + " " + resp.Request.URL.Path
}

func summarize(status int, body []byte) string {
	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		for _, msg := range []string{payload.Message, payload.Error, payload.Detail} {
			if strings.TrimSpace(msg) != "" {
				return fmt.Sprintf("HTTP %d: %s", status, msg)
			}
		}
	}
	return fmt.Sprintf("HTTP %d", status)
}
