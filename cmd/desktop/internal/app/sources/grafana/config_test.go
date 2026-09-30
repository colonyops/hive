package grafana

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

func TestMetricsConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		config  MetricsConfig
		wantErr bool
	}{
		{
			name:   "complete",
			config: MetricsConfig{Credential: "grafana/host-1", DatasourceUID: "ds", Expr: "up"},
		},
		{
			name:   "title is optional",
			config: MetricsConfig{Credential: "grafana/host-1", DatasourceUID: "ds", Expr: "up", Title: "Uptime"},
		},
		{
			name:    "zero config",
			config:  MetricsConfig{},
			wantErr: true,
		},
		{
			name:    "missing datasource",
			config:  MetricsConfig{Credential: "grafana/host-1", Expr: "up"},
			wantErr: true,
		},
		{
			name:    "missing expr",
			config:  MetricsConfig{Credential: "grafana/host-1", DatasourceUID: "ds"},
			wantErr: true,
		},
		{
			name:    "wrong provider",
			config:  MetricsConfig{Credential: "github/octocat", DatasourceUID: "ds", Expr: "up"},
			wantErr: true,
		},
		{
			name:    "credential without account",
			config:  MetricsConfig{Credential: "grafana", DatasourceUID: "ds", Expr: "up"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.config.Validate()
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestMetricsConfigCredentialRef(t *testing.T) {
	t.Parallel()

	ref, err := (&MetricsConfig{Credential: "grafana/prod-1"}).CredentialRef()
	require.NoError(t, err)
	assert.Equal(t, Provider, ref.Provider)
	assert.Equal(t, "prod-1", ref.Account)

	_, err = (&MetricsConfig{Credential: "posthog/x"}).CredentialRef()
	assert.Error(t, err, "a ref naming another provider is a config mistake, rejected at load")
}

// The cadence floor is optional everywhere it is offered, and a negative one
// is a config error rather than a floor of zero.
func TestMetricsConfigInterval(t *testing.T) {
	t.Parallel()
	base := func() MetricsConfig {
		return MetricsConfig{Credential: "grafana/stack-1", DatasourceUID: "ds", Expr: "up"}
	}

	unset := base()
	require.NoError(t, unset.Validate())
	assert.Zero(t, unset.Interval.Duration())

	hourly := base()
	hourly.Interval = connector.Duration(time.Hour)
	require.NoError(t, hourly.Validate())

	negative := base()
	negative.Interval = connector.Duration(-time.Second)
	assert.ErrorContains(t, negative.Validate(), "interval must not be negative")
}
