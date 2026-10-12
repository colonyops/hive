package relay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/pkg/duration"
)

func validConfig() *Config {
	return &Config{Credential: "relay/relay.example.com-me@example.com", Source: "src_1"}
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{name: "minimal is valid", mutate: func(*Config) {}},
		{name: "generic with a map is valid", mutate: func(c *Config) {
			c.Dialect = "generic"
			c.Map = &Map{Key: "/raw/body/monitor/id", Title: "", States: map[string]string{"0": "down"}}
			c.Checks = map[string]Check{"backup": {Period: duration.Duration(time.Hour)}}
		}},
		{name: "missing credential", mutate: func(c *Config) { c.Credential = "" }, wantErr: "credential is required"},
		{name: "credential for another provider", mutate: func(c *Config) { c.Credential = "gitea/host-me" }, wantErr: "is not a relay credential"},
		{name: "missing source", mutate: func(c *Config) { c.Source = " " }, wantErr: "source is required"},
		{name: "unknown dialect", mutate: func(c *Config) { c.Dialect = "xml" }, wantErr: `format "xml" is not one of`},
		{name: "later dialect", mutate: func(c *Config) { c.Dialect = "healthchecks" }, wantErr: "arrives in a later release"},
		{name: "pointer without slash", mutate: func(c *Config) { c.Map = &Map{Title: "raw/body"} }, wantErr: "map.title"},
		{name: "pointer with a bad escape", mutate: func(c *Config) { c.Map = &Map{Key: "/a~2b"} }, wantErr: "map.key"},
		{name: "bad icon", mutate: func(c *Config) { c.Icon = "not-an-icon" }, wantErr: "not a supported feed icon"},
		{name: "bad image", mutate: func(c *Config) { c.Image = "nope" }, wantErr: "not a valid mark reference"},
		{name: "negative interval", mutate: func(c *Config) { c.Interval = duration.Duration(-time.Minute) }, wantErr: "interval"},
		{name: "check with no period", mutate: func(c *Config) { c.Checks = map[string]Check{"backup": {}} }, wantErr: "period greater than zero"},
		{name: "check with negative grace", mutate: func(c *Config) {
			c.Checks = map[string]Check{"backup": {Period: duration.Duration(time.Hour), Grace: duration.Duration(-time.Second)}}
		}, wantErr: "negative grace"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := validConfig()
			tc.mutate(c)
			err := c.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
