package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsInitCommand(t *testing.T) {
	// Note: space-separated --flag value pairs are ambiguous without full flag-schema
	// parsing; the value is treated as a potential subcommand. Use --flag=value syntax
	// when passing global flags before init (e.g. --config=path/to/config.yaml init).
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "init subcommand", args: []string{"hive", "init"}, want: true},
		{name: "init with trailing flag", args: []string{"hive", "init", "--help"}, want: true},
		{name: "init after short flag", args: []string{"hive", "-v", "init"}, want: true},
		{name: "init after flag=value", args: []string{"hive", "--config=foo.yaml", "init"}, want: true},
		{name: "other subcommand", args: []string{"hive", "new"}, want: false},
		{name: "no subcommand", args: []string{"hive"}, want: false},
		{name: "empty args", args: []string{}, want: false},
		{name: "flags only, no subcommand", args: []string{"hive", "--flag"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isInitCommand(tt.args))
		})
	}
}

// Before skips opening the hive runtime when this reports true.
func TestIsShellCompletion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "static script", args: []string{"hive", "completion", "zsh"}, want: true},
		{name: "dynamic completion", args: []string{"hive", "hc", "--generate-shell-completion"}, want: true},
		{name: "flag after --", args: []string{"hive", "msg", "--", "--generate-shell-completion"}, want: false},
		{name: "flag not last", args: []string{"hive", "--generate-shell-completion", "ls"}, want: false},
		{name: "plain command", args: []string{"hive", "ls"}, want: false},
		{name: "no subcommand", args: []string{"hive"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isShellCompletion(tt.args))
		})
	}
}
