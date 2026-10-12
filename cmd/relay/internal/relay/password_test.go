package relay_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/argon2"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := relay.HashPassword("correct horse")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$"), hash)
	require.NoError(t, relay.ParsePasswordHash(hash))

	ok, err := relay.VerifyPassword(hash, "correct horse")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = relay.VerifyPassword(hash, "wrong horse")
	require.NoError(t, err)
	assert.False(t, ok)

	other, err := relay.HashPassword("correct horse")
	require.NoError(t, err)
	assert.NotEqual(t, hash, other, "every hash gets its own salt")
}

func TestVerifyPasswordReadsParameters(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte("pw"), salt, 1, 8, 1, 16)
	enc := "$argon2id$v=19$m=8,t=1,p=1$" + b64(salt) + "$" + b64(key)

	ok, err := relay.VerifyPassword(enc, "pw")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestParsePasswordHashRejects(t *testing.T) {
	for _, bad := range []string{
		"",
		"secret",
		"$argon2i$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=18$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=0,t=3,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=4$!!!$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$",
		"$argon2id$v=19$m=65536,t=3$c2FsdA$aGFzaA",
	} {
		require.Error(t, relay.ParsePasswordHash(bad), bad)
		_, err := relay.VerifyPassword(bad, "pw")
		assert.Error(t, err, bad)
	}
}
