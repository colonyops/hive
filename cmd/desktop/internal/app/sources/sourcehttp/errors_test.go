package sourcehttp

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func response(t *testing.T, status int, body string, header http.Header) *http.Response {
	t.Helper()
	if header == nil {
		header = http.Header{}
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.test/user", nil)
	require.NoError(t, err)
	resp := &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     header,
		Request:    req,
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestStatusMapsTaxonomy(t *testing.T) {
	t.Parallel()

	always := Errors{Forbidden: func(*http.Response, []byte) bool { return true }}
	never := Errors{Forbidden: func(*http.Response, []byte) bool { return false }}

	tests := []struct {
		name    string
		errs    Errors
		status  int
		want    error
		wantErr bool
	}{
		{name: "2xx is success", status: http.StatusOK},
		{name: "401 is unauthorized", status: http.StatusUnauthorized, want: ErrUnauthorized},
		{name: "429 is rate limited", status: http.StatusTooManyRequests, want: ErrRateLimited},
		{name: "403 without a classifier is generic", status: http.StatusForbidden, wantErr: true},
		{name: "403 the classifier accepts is rate limited", errs: always, status: http.StatusForbidden, want: ErrRateLimited},
		{name: "403 the classifier rejects is generic", errs: never, status: http.StatusForbidden, wantErr: true},
		{name: "304 is not a success", status: http.StatusNotModified, wantErr: true},
		{name: "500 is generic", status: http.StatusInternalServerError, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.errs.Status(response(t, tc.status, "", nil)) //nolint:bodyclose // synthesized NopCloser body
			switch {
			case tc.want != nil:
				require.ErrorIs(t, err, tc.want)
			case tc.wantErr:
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrRateLimited)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestStatusPassesBodyToClassifier(t *testing.T) {
	t.Parallel()

	var got string
	errs := Errors{Forbidden: func(_ *http.Response, body []byte) bool {
		got = string(body)
		return false
	}}
	_ = errs.Status(response(t, http.StatusForbidden, `{"message":"nope"}`, nil)) //nolint:bodyclose // synthesized NopCloser body
	assert.JSONEq(t, `{"message":"nope"}`, got)
}

func TestStatusMessageNamesProviderAndReason(t *testing.T) {
	t.Parallel()

	err := Errors{Name: "github"}.Status(response(t, http.StatusInternalServerError, `{"message":"boom"}`, nil)) //nolint:bodyclose // synthesized NopCloser body
	require.ErrorContains(t, err, "github:")
	require.ErrorContains(t, err, "HTTP 500: boom")
}

func TestSummarizeReadsCommonMessageKeys(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "HTTP 400: a", summarize(400, []byte(`{"message":"a"}`)))
	assert.Equal(t, "HTTP 400: b", summarize(400, []byte(`{"error":"b"}`)))
	assert.Equal(t, "HTTP 400: c", summarize(400, []byte(`{"detail":"c"}`)))
	assert.Equal(t, "HTTP 400", summarize(400, []byte(`not json`)))
	assert.Equal(t, "HTTP 400", summarize(400, []byte(`{"message":"  "}`)))
}

func TestRateLimitPrefersRetryAfter(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	h.Set("Retry-After", "60")
	h.Set("X-RateLimit-Reset", "1780000000")

	before := time.Now()
	assert.WithinDuration(t, before.Add(time.Minute), RateLimit(h).ResetAt, time.Second)
}

func TestRateLimitFallsBackToResetEpoch(t *testing.T) {
	t.Parallel()

	const epoch = 1_780_000_000
	h := http.Header{}
	h.Set("X-RateLimit-Reset", strconv.Itoa(epoch))

	assert.Equal(t, time.Unix(epoch, 0), RateLimit(h).ResetAt)
}

func TestRateLimitWithoutHintsHasZeroResetAt(t *testing.T) {
	t.Parallel()

	err := RateLimit(http.Header{})
	assert.True(t, err.ResetAt.IsZero())
	assert.Equal(t, ErrRateLimited.Error(), err.Error())
	require.ErrorIs(t, err, ErrRateLimited)
}

func TestUnreachableWrapsBothSentinelAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial tcp: refused")
	err := Errors{Name: "github"}.Unreachable(cause)
	require.ErrorIs(t, err, ErrUnreachable)
	require.ErrorIs(t, err, cause)
	require.ErrorContains(t, err, "github:")
}

func TestErrorfPreservesWrappedCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("eof")
	err := Errors{Name: "github"}.Errorf("decode /user: %w", cause)
	require.ErrorIs(t, err, cause)
	require.ErrorContains(t, err, "github: decode /user")
}

func TestUnnamedErrorsFallBackToSourcePrefix(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, Errors{}.Unreachable(errors.New("x")), "source:")
}
