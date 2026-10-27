package core_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/config"
	"github.com/colonyops/hive/cmd/drive-server/internal/core"
)

var eightDigits = regexp.MustCompile(`\b\d{8}\b`)

func closedPolicy() config.Policy {
	p := policy("closed", "*@example.com")
	no := false
	p.SignUp = &no
	return p
}

func (e *env) mails(t *testing.T) []core.Mail {
	t.Helper()
	mails, err := e.store.Outbox().Due(t.Context(), e.clock.Now().Add(24*time.Hour), 100)
	require.NoError(t, err)
	return mails
}

func (e *env) start(t *testing.T, email string) string {
	t.Helper()
	before := len(e.mails(t))
	require.NoError(t, e.signIn.Start(t.Context(), email))
	mails := e.mails(t)
	require.Len(t, mails, before+1)
	return eightDigits.FindString(mails[len(mails)-1].Body)
}

func TestSignInMailsCodeAndLink(t *testing.T) {
	e := newEnv(t, devPolicy())
	require.NoError(t, e.signIn.Start(t.Context(), " Dev@Example.com "))

	mails := e.mails(t)
	require.Len(t, mails, 1)
	assert.Equal(t, "dev@example.com", mails[0].To)
	assert.Regexp(t, eightDigits, mails[0].Body)
	assert.Contains(t, mails[0].Body, testBaseURL+"/sign-in?email=dev%40example.com")
}

func TestSignInVerifyCreatesAccountAndSession(t *testing.T) {
	e := newEnv(t, devPolicy())
	code := e.start(t, "dev@example.com")

	value, err := e.signIn.Verify(t.Context(), "DEV@example.com", code)
	require.NoError(t, err)

	p, err := e.sessions.Resolve(t.Context(), value)
	require.NoError(t, err)
	assert.Equal(t, "dev@example.com", p.Email)
	assert.Equal(t, core.ScopePush, p.Scope)
	assert.True(t, p.ViaSession)

	acct, err := e.store.Accounts().GetByEmail(t.Context(), "dev@example.com")
	require.NoError(t, err)
	require.NotNil(t, acct.LastSignInAt)
	assert.True(t, e.clock.Now().Equal(*acct.LastSignInAt))

	_, err = e.signIn.Verify(t.Context(), "dev@example.com", code)
	requireKind(t, err, core.KindUnauthenticated)
}

func TestSignInCodeExpires(t *testing.T) {
	e := newEnv(t, devPolicy())
	code := e.start(t, "dev@example.com")

	e.clock.Advance(10 * time.Minute)
	_, err := e.signIn.Verify(t.Context(), "dev@example.com", code)
	requireKind(t, err, core.KindUnauthenticated)
}

func TestSignInFiveWrongAttemptsBurnTheCode(t *testing.T) {
	e := newEnv(t, devPolicy())
	code := e.start(t, "dev@example.com")
	wrong := "00000000"
	if code == wrong {
		wrong = "11111111"
	}

	for i := range 5 {
		_, err := e.signIn.Verify(t.Context(), "dev@example.com", wrong)
		requireKind(t, err, core.KindUnauthenticated)
		if i < 4 {
			assert.Contains(t, err.Error(), "attempts left")
		}
	}
	_, err := e.signIn.Verify(t.Context(), "dev@example.com", code)
	requireKind(t, err, core.KindUnauthenticated)
	assert.NotContains(t, err.Error(), "attempts left")
}

func TestSignInNewCodeReplacesOld(t *testing.T) {
	e := newEnv(t, devPolicy())
	first := e.start(t, "dev@example.com")
	second := e.start(t, "dev@example.com")
	require.NotEqual(t, first, second, "8 random digits; a clash is a one in 10^8 flake")

	_, err := e.signIn.Verify(t.Context(), "dev@example.com", first)
	requireKind(t, err, core.KindUnauthenticated)
	_, err = e.signIn.Verify(t.Context(), "dev@example.com", second)
	require.NoError(t, err)
}

func TestSignInUnmatchedEmailSendsNothing(t *testing.T) {
	e := newEnv(t, devPolicy())

	require.NoError(t, e.signIn.Start(t.Context(), "dev@other.example"))
	assert.Empty(t, e.mails(t))

	_, err := e.signIn.Verify(t.Context(), "dev@other.example", "12345678")
	requireKind(t, err, core.KindUnauthenticated)
}

func TestSignInClosedPolicyAdmitsExistingAccountsOnly(t *testing.T) {
	e := newEnv(t, closedPolicy())

	require.NoError(t, e.signIn.Start(t.Context(), "new@example.com"))
	assert.Empty(t, e.mails(t), "no account and sign_up is false")

	_, _, err := e.accounts.Ensure(t.Context(), "old@example.com", true)
	require.NoError(t, err)
	code := e.start(t, "old@example.com")
	_, err = e.signIn.Verify(t.Context(), "old@example.com", code)
	require.NoError(t, err)
}

func TestSignInClosedPolicyBlocksVerifyForNewAccount(t *testing.T) {
	e := newEnv(t, devPolicy())
	code := e.start(t, "new@example.com")

	e.policies.Swap([]config.Policy{closedPolicy()})
	_, err := e.signIn.Verify(t.Context(), "new@example.com", code)
	requireKind(t, err, core.KindForbidden)
	_, err = e.store.Accounts().GetByEmail(t.Context(), "new@example.com")
	assert.True(t, core.IsNotFound(err))
}

func TestSessionSlidesAtMostHourlyAndExpires(t *testing.T) {
	e := newEnv(t, devPolicy())
	acct, _, err := e.accounts.Ensure(t.Context(), "dev@example.com", false)
	require.NoError(t, err)
	value, sess, err := e.sessions.Mint(t.Context(), acct.ID)
	require.NoError(t, err)
	assert.True(t, e.clock.Now().Add(core.SessionTTL).Equal(sess.ExpiresAt))

	expiry := func() time.Time {
		var ns int64
		require.NoError(t, e.rawDB(t).QueryRowContext(t.Context(), `SELECT expires_at FROM sessions WHERE id = ?`, sess.ID).Scan(&ns))
		return time.Unix(0, ns).UTC()
	}

	e.clock.Advance(30 * time.Minute)
	_, err = e.sessions.Resolve(t.Context(), value)
	require.NoError(t, err)
	assert.True(t, sess.ExpiresAt.Equal(expiry()), "within the hour, no write")

	e.clock.Advance(30 * time.Minute)
	_, err = e.sessions.Resolve(t.Context(), value)
	require.NoError(t, err)
	assert.True(t, e.clock.Now().Add(core.SessionTTL).Equal(expiry()), "an hour on, expiry slides")

	e.clock.Advance(core.SessionTTL)
	_, err = e.sessions.Resolve(t.Context(), value)
	requireKind(t, err, core.KindUnauthenticated)
}

func TestSessionRevokeAndPolicyRecheck(t *testing.T) {
	e := newEnv(t, devPolicy())
	acct, _, err := e.accounts.Ensure(t.Context(), "dev@example.com", false)
	require.NoError(t, err)
	value, _, err := e.sessions.Mint(t.Context(), acct.ID)
	require.NoError(t, err)

	e.policies.Swap(nil)
	_, err = e.sessions.Resolve(t.Context(), value)
	requireKind(t, err, core.KindUnauthenticated)

	e.policies.Swap([]config.Policy{devPolicy()})
	_, err = e.sessions.Resolve(t.Context(), value)
	require.NoError(t, err)

	require.NoError(t, e.sessions.Revoke(t.Context(), value))
	require.NoError(t, e.sessions.Revoke(t.Context(), value), "revoking twice is fine")
	_, err = e.sessions.Resolve(t.Context(), value)
	requireKind(t, err, core.KindUnauthenticated)

	_, err = e.sessions.Resolve(t.Context(), "never-minted")
	requireKind(t, err, core.KindUnauthenticated)
}

func TestSweeperDeletesExpiredCodesAndSessions(t *testing.T) {
	e := newEnv(t, devPolicy())
	acct, _, err := e.accounts.Ensure(t.Context(), "dev@example.com", false)
	require.NoError(t, err)
	_, _, err = e.sessions.Mint(t.Context(), acct.ID)
	require.NoError(t, err)
	e.start(t, "dev@example.com")

	count := func(table string) int {
		var n int
		require.NoError(t, e.rawDB(t).QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+table).Scan(&n))
		return n
	}
	require.NoError(t, e.sweeper.Tick(t.Context()))
	assert.Equal(t, 1, count("sessions"))
	assert.Equal(t, 1, count("sign_in_codes"))

	e.clock.Advance(11 * time.Minute)
	require.NoError(t, e.sweeper.Tick(t.Context()))
	assert.Equal(t, 1, count("sessions"))
	assert.Equal(t, 0, count("sign_in_codes"))

	e.clock.Advance(core.SessionTTL)
	require.NoError(t, e.sweeper.Tick(t.Context()))
	assert.Equal(t, 0, count("sessions"))
}
