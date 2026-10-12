package config

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/relay/internal/relay"
)

func load(t *testing.T, args ...string) (Config, error) {
	t.Helper()
	var cfg Config
	var loadErr error
	cmd := &cli.Command{
		Name:  "serve",
		Flags: append(Flags(), &cli.StringFlag{Name: FlagMint}),
		Action: func(_ context.Context, cmd *cli.Command) error {
			cfg, loadErr = Load(cmd)
			return nil
		},
	}
	require.NoError(t, cmd.Run(context.Background(), append([]string{"serve"}, args...)))
	return cfg, loadErr
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(t)
	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.Listen)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Empty(t, cfg.PublicURL)
	assert.Empty(t, cfg.TrustedProxies)
	assert.Equal(t, relay.DefaultLimits(), cfg.Limits)
	assert.Error(t, cfg.RequirePublicURL())
}

func TestLoadEnv(t *testing.T) {
	t.Setenv("RELAY_TRUSTED_PROXIES", "10.0.0.0/8, 192.0.2.7,2001:db8::/32")
	t.Setenv("RELAY_RETENTION", "1h")
	t.Setenv("RELAY_BODY_BYTES", "1024")
	t.Setenv("RELAY_PUBLIC_URL", "https://relay.example.com/")

	cfg, err := load(t)
	require.NoError(t, err)
	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.0.2.7/32"),
		netip.MustParsePrefix("2001:db8::/32"),
	}, cfg.TrustedProxies)
	assert.Equal(t, time.Hour, cfg.Limits.Retention)
	assert.Equal(t, int64(1024), cfg.Limits.BodyBytes)
	assert.Equal(t, "https://relay.example.com", cfg.PublicURL)
	assert.NoError(t, cfg.RequirePublicURL())
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		args []string
	}{
		{"bad cidr", map[string]string{"RELAY_TRUSTED_PROXIES": "10.0.0.0/33"}, nil},
		{"not an ip", map[string]string{"RELAY_TRUSTED_PROXIES": "proxy.local"}, nil},
		{"relative public url", nil, []string{"--public-url", "relay.example.com"}},
		{"non-http public url", nil, []string{"--public-url", "ftp://relay.example.com"}},
		{"bad log level", nil, []string{"--log-level", "loud"}},
		{"zero limit", nil, []string{"--concurrency", "0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			_, err := load(t, tt.args...)
			assert.Error(t, err)
		})
	}
}

func TestLoadMintTrims(t *testing.T) {
	cfg, err := load(t, "--mint", " dev@example.com ")
	require.NoError(t, err)
	assert.Equal(t, "dev@example.com", cfg.MintFor)
}
