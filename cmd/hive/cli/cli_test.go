package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

func TestCommandPath(t *testing.T) {
	root := &cli.Command{Flags: []cli.Flag{
		&cli.StringFlag{Name: "log-level"},
		&cli.StringFlag{Name: "config", Aliases: []string{"c"}},
		&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}},
	}}
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "space-separated value", args: []string{"hive", "--config", "x", "daemon", "run"}, want: []string{"daemon", "run"}},
		{name: "level then servers", args: []string{"hive", "--log-level", "debug", "servers", "ls"}, want: []string{"servers", "ls"}},
		{name: "short alias value", args: []string{"hive", "-c", "x.yaml", "init"}, want: []string{"init"}},
		{name: "flag=value", args: []string{"hive", "--config=foo.yaml", "init"}, want: []string{"init"}},
		{name: "bool flag takes no value", args: []string{"hive", "-v", "init"}, want: []string{"init"}},
		{name: "stops at subcommand flags", args: []string{"hive", "daemon", "run", "--wait-lock", "extra"}, want: []string{"daemon", "run"}},
		{name: "init", args: []string{"hive", "init"}, want: []string{"init"}},
		{name: "no subcommand", args: []string{"hive"}, want: nil},
		{name: "empty args", args: []string{}, want: nil},
		{name: "after --", args: []string{"hive", "--", "daemon"}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, commandPath(root, tt.args))
		})
	}
}

func TestSkipsEngine(t *testing.T) {
	assert.True(t, skipsEngine([]string{"daemon", "run"}))
	assert.True(t, skipsEngine([]string{"servers"}))
	assert.False(t, skipsEngine([]string{"session", "list"}))
	assert.False(t, skipsEngine(nil))
}

func TestIsShellCompletion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "dynamic completion", args: []string{"hive", "new", "--generate-shell-completion"}, want: true},
		{name: "static script", args: []string{"hive", "completion", "zsh"}, want: true},
		{name: "flag after --", args: []string{"hive", "new", "--", "--generate-shell-completion"}, want: false},
		{name: "flag not last", args: []string{"hive", "--generate-shell-completion", "new"}, want: false},
		{name: "plain command", args: []string{"hive", "new"}, want: false},
		{name: "no subcommand", args: []string{"hive"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isShellCompletion(tt.args))
		})
	}
}
