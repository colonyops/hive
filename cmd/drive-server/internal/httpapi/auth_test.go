package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/cmd/drive-server/internal/mail"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

type memSender struct {
	mu   sync.Mutex
	sent []core.Mail
}

func (m *memSender) Send(_ context.Context, mm core.Mail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, mm)
	return nil
}

type authServer struct {
	*server
	mailer *mail.Mailer
	mails  *memSender
}

func newAuthServer(t *testing.T, baseURL string) *authServer {
	t.Helper()
	s := newServerAt(t, baseURL)
	mails := &memSender{}
	return &authServer{server: s, mailer: mail.NewMailer(zerolog.Nop(), s.svc.Outbox, mails, nil), mails: mails}
}

var codeInMail = regexp.MustCompile(`\b\d{8}\b`)

func (s *authServer) latestCode(t *testing.T, email string) string {
	t.Helper()
	require.NoError(t, s.mailer.Drain(t.Context()))
	s.mails.mu.Lock()
	defer s.mails.mu.Unlock()
	for i := len(s.mails.sent) - 1; i >= 0; i-- {
		if m := s.mails.sent[i]; m.To == email {
			return codeInMail.FindString(m.Body)
		}
	}
	require.FailNow(t, "no mail sent to "+email)
	return ""
}

func (s *authServer) sent() int {
	s.mails.mu.Lock()
	defer s.mails.mu.Unlock()
	return len(s.mails.sent)
}

func (s *authServer) start(email string) *httptest.ResponseRecorder {
	return s.send(call{method: http.MethodPost, target: "/api/v1/auth/code", body: `{"email":"` + email + `"}`})
}

func (s *authServer) signIn(t *testing.T, email string) *http.Cookie {
	t.Helper()
	require.Equal(t, http.StatusAccepted, s.start(email).Code)
	rec := s.send(call{method: http.MethodPost, target: "/api/v1/auth/verify", body: `{"email":"` + email + `","code":"` + s.latestCode(t, email) + `"}`})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	return cookies[0]
}

func TestSignInRoundTripOverHTTPLoopback(t *testing.T) {
	s := newAuthServer(t, "http://127.0.0.1:8080")
	c := s.signIn(t, "dev@example.com")

	assert.Equal(t, "hive_drive_session", c.Name)
	assert.False(t, c.Secure)
	assert.True(t, c.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)

	rec := s.send(call{method: http.MethodGet, target: "/api/v1/me", cookie: c})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	me := decode[driveapi.Me](t, rec)
	assert.Equal(t, "dev@example.com", me.Email)

	rec = s.send(call{method: http.MethodPost, target: "/api/v1/auth/sign-out", cookie: c})
	require.Equal(t, http.StatusNoContent, rec.Code)
	cleared := rec.Result().Cookies()
	require.Len(t, cleared, 1)
	assert.Equal(t, -1, cleared[0].MaxAge)

	assert.Equal(t, http.StatusUnauthorized, s.send(call{method: http.MethodGet, target: "/api/v1/me", cookie: c}).Code)
}

func TestSessionCookieAttributesOnHTTPS(t *testing.T) {
	s := newAuthServer(t, "https://drive.example.com")
	c := s.signIn(t, "dev@example.com")

	assert.Equal(t, "__Host-hive_drive_session", c.Name)
	assert.True(t, c.Secure)
	assert.True(t, c.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
	assert.Empty(t, c.Domain, "a __Host- cookie must not set Domain")
	assert.Equal(t, 30*24*3600, c.MaxAge)
}

func TestStartIsIndistinguishableAndSendsNothingWhenUnmatched(t *testing.T) {
	s := newAuthServer(t, testBaseURL)

	matched := s.start("dev@example.com")
	unmatched := s.start("dev@other.example")
	assert.Equal(t, http.StatusAccepted, matched.Code)
	assert.Equal(t, http.StatusAccepted, unmatched.Code)
	assert.Equal(t, matched.Body.String(), unmatched.Body.String())

	require.NoError(t, s.mailer.Drain(t.Context()))
	assert.Equal(t, 1, s.sent())
}

func TestWrongCodeIsUnauthenticated(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	require.Equal(t, http.StatusAccepted, s.start("dev@example.com").Code)

	rec := s.send(call{method: http.MethodPost, target: "/api/v1/auth/verify", body: `{"email":"dev@example.com","code":"00000000"}`})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Result().Cookies())

	rec = s.send(call{method: http.MethodPost, target: "/api/v1/auth/verify", body: `{"email":"dev@example.com","code":"abc"}`})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestCrossOriginPostIsRejected(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	body := `{"email":"dev@example.com"}`

	rec := s.send(call{method: http.MethodPost, target: "/api/v1/auth/code", body: body, header: map[string]string{"Origin": "https://evil.example"}})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Zero(t, s.sent())

	rec = s.send(call{method: http.MethodPost, target: "/api/v1/auth/code", body: body, header: map[string]string{"Origin": "http://drive.test"}})
	assert.Equal(t, http.StatusAccepted, rec.Code, "same origin passes")
}

func TestFourthCodeRequestInAnHourIsRateLimited(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	for range core.StartPerEmail {
		require.Equal(t, http.StatusAccepted, s.start("dev@example.com").Code)
	}

	rec := s.start("Dev@Example.com")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "rate_limited", errorKind(t, rec))
	assert.Equal(t, "1200", rec.Header().Get("Retry-After"))

	assert.Equal(t, http.StatusAccepted, s.start("other@example.com").Code, "the email bucket is per address")
}

func TestVerifyIsRateLimitedPerEmail(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	verify := func() *httptest.ResponseRecorder {
		return s.send(call{method: http.MethodPost, target: "/api/v1/auth/verify", body: `{"email":"dev@example.com","code":"00000000"}`})
	}
	for range core.VerifyPerEmail {
		assert.Equal(t, http.StatusUnauthorized, verify().Code)
	}
	rec := verify()
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestPolicySwapLocksSessionAndTokenOut(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	c := s.signIn(t, "dev@example.com")
	tok := s.token(t, "dev@example.com", core.ScopeRead)
	require.Equal(t, http.StatusOK, s.send(call{method: http.MethodGet, target: "/api/v1/me", cookie: c}).Code)
	require.Equal(t, http.StatusOK, s.send(call{method: http.MethodGet, target: "/api/v1/me", token: tok}).Code)

	s.svc.Policies.Swap(nil)
	assert.Equal(t, http.StatusUnauthorized, s.send(call{method: http.MethodGet, target: "/api/v1/me", cookie: c}).Code)
	assert.Equal(t, http.StatusUnauthorized, s.send(call{method: http.MethodGet, target: "/api/v1/me", token: tok}).Code)
}

func TestMeRejectsBadCredentials(t *testing.T) {
	s := newAuthServer(t, testBaseURL)

	for name, header := range map[string]string{
		"no header":    "",
		"wrong scheme": "Basic abc",
		"bad token":    "Bearer hds_nope",
		"not a token":  "Bearer nope",
	} {
		rec := s.send(call{method: http.MethodGet, target: "/api/v1/me", header: map[string]string{"Authorization": header}})
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
		assert.Equal(t, "unauthenticated", errorKind(t, rec), name)
	}
}

func TestMeUnlimitedQuotaIsNull(t *testing.T) {
	s := newServerWithPolicy(t, testBaseURL, devPolicy(config.Size{Unlimited: true}))

	rec := s.do(http.MethodGet, "/api/v1/me", s.token(t, "dev@example.com", core.ScopePush), "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"email":"dev@example.com","policy":"dev","quota":{"used":0,"limit":null}}`, rec.Body.String())
}

func TestSessionMintsListsAndRevokesTokens(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	c := s.signIn(t, "dev@example.com")

	rec := s.send(call{method: http.MethodPost, target: "/api/v1/tokens", cookie: c, body: `{"name":"laptop","scope":"read","expires_in":"7d"}`})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	created := decode[driveapi.CreatedToken](t, rec)
	require.NotEmpty(t, created.Secret)
	assert.Equal(t, "laptop", created.Name)
	assert.Equal(t, "read", created.Scope)
	require.NotNil(t, created.ExpiresAt)

	assert.Equal(t, http.StatusOK, s.send(call{method: http.MethodGet, target: "/api/v1/me", token: created.Secret}).Code)

	rec = s.send(call{method: http.MethodGet, target: "/api/v1/tokens", cookie: c})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), created.Secret, "the secret is shown once")
	listed := decode[[]driveapi.Token](t, rec)
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)

	rec = s.send(call{method: http.MethodDelete, target: "/api/v1/tokens/" + created.ID, cookie: c})
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, http.StatusUnauthorized, s.send(call{method: http.MethodGet, target: "/api/v1/me", token: created.Secret}).Code)

	for name, body := range map[string]string{
		"bad scope":  `{"name":"x","scope":"admin"}`,
		"bad expiry": `{"name":"x","scope":"read","expires_in":"2d"}`,
		"no name":    `{"name":" ","scope":"read"}`,
	} {
		rec := s.send(call{method: http.MethodPost, target: "/api/v1/tokens", cookie: c, body: body})
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, name)
	}
}

func TestTokenCannotManageTokens(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	tok := s.token(t, "dev@example.com", core.ScopePush)

	for _, c := range []call{
		{method: http.MethodPost, target: "/api/v1/tokens", body: `{"name":"x","scope":"read"}`},
		{method: http.MethodGet, target: "/api/v1/tokens"},
		{method: http.MethodDelete, target: "/api/v1/tokens/some-id"},
		{method: http.MethodPost, target: "/api/v1/auth/sign-out"},
	} {
		c.token = tok
		rec := s.send(c)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s", c.method, c.target)
		assert.Equal(t, "forbidden", errorKind(t, rec))
	}
}

func TestBearerHeaderWinsOverCookie(t *testing.T) {
	s := newAuthServer(t, testBaseURL)
	c := s.signIn(t, "dev@example.com")

	rec := s.send(call{method: http.MethodGet, target: "/api/v1/me", cookie: c, token: "hds_bogus"})
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestOversizedJSONBodyIsTooLarge(t *testing.T) {
	s := newAuthServer(t, testBaseURL)

	rec := s.send(call{method: http.MethodPost, target: "/api/v1/auth/code", body: `{"email":"` + strings.Repeat("x", 1<<20) + `"}`})
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, "too_large", errorKind(t, rec))
}
