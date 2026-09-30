package exec

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

func validConfig() *Config {
	return &Config{Command: "gcx alerts list -o json", Timeout: connector.Duration(30 * time.Second)}
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		mutate func(*Config)
		wants  string
	}{
		"valid":               {func(*Config) {}, ""},
		"no command":          {func(c *Config) { c.Command = "  " }, "command is required"},
		"no timeout":          {func(c *Config) { c.Timeout = 0 }, "timeout is required"},
		"timeout too long":    {func(c *Config) { c.Timeout = connector.Duration(10 * time.Minute) }, "exceeds the maximum"},
		"negative interval":   {func(c *Config) { c.Interval = connector.Duration(-time.Second) }, "must not be negative"},
		"relative cwd":        {func(c *Config) { c.Cwd = "src/repo" }, "must be an absolute path"},
		"absolute cwd":        {func(c *Config) { c.Cwd = "/Users/x/src" }, ""},
		"home-relative cwd":   {func(c *Config) { c.Cwd = "~/src" }, ""},
		"unknown icon":        {func(c *Config) { c.Icon = "not-an-icon" }, "not a supported feed icon"},
		"known icon":          {func(c *Config) { c.Icon = "bell" }, ""},
		"bad env name":        {func(c *Config) { c.Env = map[string]string{"A=B": "c"} }, "not a usable variable name"},
		"good env":            {func(c *Config) { c.Env = map[string]string{"TOKEN": "x"} }, ""},
		"interval is a floor": {func(c *Config) { c.Interval = connector.Duration(time.Hour) }, ""},
		"image":               {func(c *Config) { c.Image = "0123456789abcdef0123456789abcdef" }, ""},
		"uppercase image":     {func(c *Config) { c.Image = "0123456789ABCDEF0123456789abcdef" }, "not a valid mark reference"},
		"short image":         {func(c *Config) { c.Image = "0123" }, "not a valid mark reference"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := validConfig()
			tc.mutate(cfg)

			err := cfg.Validate()
			if tc.wants == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wants)
		})
	}
}

func TestConfigTooManyEnvVars(t *testing.T) {
	cfg := validConfig()
	cfg.Env = map[string]string{}
	for i := range maxEnvVars + 1 {
		cfg.Env[strings.Repeat("A", i+1)] = "x"
	}

	require.ErrorContains(t, cfg.Validate(), "at most 32")
}

// The timeout and interval round-trip as duration strings in both directions —
// a flow file is hand-authored YAML and the node editor is JSON over the same
// struct, so a nanosecond count in either would be a broken node.
func TestConfigDurationsRoundTripAsStrings(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("command: echo '[]'\ntimeout: 30s\ninterval: 1h\n"), &cfg))

	assert.Equal(t, 30*time.Second, cfg.Timeout.Duration())
	assert.Equal(t, time.Hour, cfg.Interval.Duration())

	out, err := yaml.Marshal(&cfg)
	require.NoError(t, err)
	assert.Contains(t, string(out), "timeout: 30s")
	assert.Contains(t, string(out), "interval: 1h0m0s")
}

// A bare number is virtually always an author assuming seconds. Reading it as
// nanoseconds would make the command time out instantly, forever.
func TestConfigRejectsABareNumberTimeout(t *testing.T) {
	var cfg Config
	err := yaml.Unmarshal([]byte("command: echo '[]'\ntimeout: 30\n"), &cfg)

	require.ErrorContains(t, err, "not a bare number")
}

// The reflected schema is what an MCP tool's input schema and a future
// generated form are built from. Without Duration's own JSONSchema it would
// report the underlying int64 and ask a caller for nanoseconds.
func TestConfigSchemaTypesDurationsAsStrings(t *testing.T) {
	raw, err := connector.Schema(Descriptor)
	require.NoError(t, err)

	var doc struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	assert.Equal(t, "string", doc.Properties["timeout"].Type)
	assert.Equal(t, "string", doc.Properties["interval"].Type)
	assert.Equal(t, "object", doc.Properties["env"].Type)
}
