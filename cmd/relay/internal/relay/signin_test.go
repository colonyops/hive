package relay_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func b64(b []byte) string { return base64.RawStdEncoding.EncodeToString(b) }

var codeRE = regexp.MustCompile(`\b\d{8}\b`)

type signInFixture struct {
	store relay.Store
	clock *clock
	si    *relay.SignIn
	allow *relay.Allowlist
}

func newSignIn(t *testing.T, users map[string]string) signInFixture {
	t.Helper()
	store := newStore(t)
	c := newClock()
	userEmails := make([]string, 0, len(users))
	for e := range users {
		userEmails = append(userEmails, e)
	}
	allow := relay.NewAllowlist([]string{"*@example.com"}, userEmails)
	si := relay.NewSignIn(store, allow, users, store.Outbox(), "https://relay.test", true)
	si.SetNow(c.Now)
	return signInFixture{store: store, clock: c, si: si, allow: allow}
}

// code reads the code out of the newest queued mail for email.
func (f signInFixture) code(t *testing.T, email string) string {
	t.Helper()
	due, err := f.store.Outbox().Due(context.Background(), f.clock.Now(), 100)
	require.NoError(t, err)
	for i := len(due) - 1; i >= 0; i-- {
		if due[i].To == email {
			assert.Contains(t, due[i].Body, "https://relay.test")
			code := codeRE.FindString(due[i].Body)
			require.NotEmpty(t, code, due[i].Body)
			return code
		}
	}
	t.Fatalf("no mail for %s", email)
	return ""
}

func TestSignInStartAndVerify(t *testing.T) {
	f := newSignIn(t, nil)
	ctx := t.Context()
	require.NoError(t, f.si.Start(ctx, " Alice@Example.com "))
	code := f.code(t, alice)

	plaintext, tok, err := f.si.Verify(ctx, alice, code, "laptop")
	require.NoError(t, err)
	assert.Equal(t, relay.TokenDevice, tok.Kind)
	assert.Equal(t, "laptop", tok.Device)
	assert.Equal(t, alice, tok.Email)

	_, got, err := relay.NewAuth(f.store, f.allow.Allow).Authenticate(ctx, plaintext)
	require.NoError(t, err)
	assert.Equal(t, tok.ID, got.ID)

	_, _, err = f.si.Verify(ctx, alice, code, "laptop")
	requireKind(t, relay.KindUnauthorized, err)
}

func TestSignInStartRejectsDisallowed(t *testing.T) {
	f := newSignIn(t, nil)
	requireKind(t, relay.KindForbidden, f.si.Start(t.Context(), "eve@evil.test"))
	_, _, err := f.si.Verify(t.Context(), "eve@evil.test", "12345678", "")
	requireKind(t, relay.KindForbidden, err)
}

func TestSignInCodeExpires(t *testing.T) {
	f := newSignIn(t, nil)
	ctx := t.Context()
	require.NoError(t, f.si.Start(ctx, alice))
	code := f.code(t, alice)
	f.clock.Advance(10 * time.Minute)
	_, _, err := f.si.Verify(ctx, alice, code, "")
	requireKind(t, relay.KindUnauthorized, err)
}

func TestSignInAttemptCap(t *testing.T) {
	f := newSignIn(t, nil)
	ctx := t.Context()
	require.NoError(t, f.si.Start(ctx, alice))
	code := f.code(t, alice)
	wrong := "00000000"
	if code == wrong {
		wrong = "11111111"
	}
	for left := 4; left > 0; left-- {
		_, _, err := f.si.Verify(ctx, alice, wrong, "")
		requireKind(t, relay.KindUnauthorized, err)
		assert.Contains(t, err.Error(), fmt.Sprintf("%d attempts left", left))
	}
	_, _, err := f.si.Verify(ctx, alice, wrong, "")
	requireKind(t, relay.KindUnauthorized, err)
	_, _, err = f.si.Verify(ctx, alice, code, "")
	requireKind(t, relay.KindUnauthorized, err)
}

func TestSignInLogin(t *testing.T) {
	hash, err := relay.HashPassword("hunter2")
	require.NoError(t, err)
	f := newSignIn(t, map[string]string{"pw@elsewhere.org": hash})
	ctx := t.Context()

	_, tok, err := f.si.Login(ctx, "PW@elsewhere.org", "hunter2", "desk")
	require.NoError(t, err)
	assert.Equal(t, relay.TokenDevice, tok.Kind)
	assert.Equal(t, "pw@elsewhere.org", tok.Email)

	_, _, err = f.si.Login(ctx, "pw@elsewhere.org", "wrong", "desk")
	requireKind(t, relay.KindUnauthorized, err)
	_, _, err = f.si.Login(ctx, alice, "hunter2", "desk")
	requireKind(t, relay.KindForbidden, err)

	f.si.SetUsers(nil)
	_, _, err = f.si.Login(ctx, "pw@elsewhere.org", "hunter2", "desk")
	requireKind(t, relay.KindForbidden, err)
}

func TestSignInMethods(t *testing.T) {
	store := newStore(t)
	tests := []struct {
		name     string
		patterns []string
		users    map[string]string
		email    bool
		want     relay.Methods
	}{
		{"api key only", nil, nil, true, relay.Methods{APIKey: true}},
		{"email without sender", []string{"*@x.test"}, nil, false, relay.Methods{APIKey: true}},
		{"email", []string{"*@x.test"}, nil, true, relay.Methods{EmailCode: true, APIKey: true}},
		{"password", nil, map[string]string{"a@x.test": "h"}, true, relay.Methods{Password: true, APIKey: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			si := relay.NewSignIn(store, relay.NewAllowlist(tt.patterns, nil), tt.users, store.Outbox(), "", tt.email)
			assert.Equal(t, tt.want, si.Methods())
		})
	}
}
