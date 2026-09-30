package sourcehttp

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"no query is untouched", "https://api.test/user", "https://api.test/user"},
		{"benign params survive", "https://api.test/n?all=true", "https://api.test/n?all=true"},
		{"secret is redacted", "https://api.test/n?access_token=sec", "https://api.test/n?access_token=REDACTED"},
		{"only the secret is redacted", "https://api.test/n?all=true&token=sec", "https://api.test/n?all=true&token=REDACTED"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.want, redactQuery(u))
		})
	}
}
