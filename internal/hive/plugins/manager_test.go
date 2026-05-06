package plugins

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
)

func TestSessionsChanged(t *testing.T) {
	mkSession := func(id string) *session.Session {
		return &session.Session{ID: id}
	}

	tests := []struct {
		name string
		old  []*session.Session
		new  []*session.Session
		want bool
	}{
		{
			name: "both nil",
			old:  nil,
			new:  nil,
			want: false,
		},
		{
			name: "same single session",
			old:  []*session.Session{mkSession("a")},
			new:  []*session.Session{mkSession("a")},
			want: false,
		},
		{
			name: "same sessions different order",
			old:  []*session.Session{mkSession("a"), mkSession("b")},
			new:  []*session.Session{mkSession("b"), mkSession("a")},
			want: false,
		},
		{
			name: "session added",
			old:  []*session.Session{mkSession("a")},
			new:  []*session.Session{mkSession("a"), mkSession("b")},
			want: true,
		},
		{
			name: "session removed",
			old:  []*session.Session{mkSession("a"), mkSession("b")},
			new:  []*session.Session{mkSession("a")},
			want: true,
		},
		{
			name: "session replaced",
			old:  []*session.Session{mkSession("a")},
			new:  []*session.Session{mkSession("b")},
			want: true,
		},
		{
			name: "nil to non-empty",
			old:  nil,
			new:  []*session.Session{mkSession("a")},
			want: true,
		},
		{
			name: "non-empty to nil",
			old:  []*session.Session{mkSession("a")},
			new:  nil,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionsChanged(tt.old, tt.new)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMergedCommandsPriority(t *testing.T) {
	mgr := NewManager(config.PluginsConfig{})
	mgr.Register(&mockPlugin{
		name: "plugin",
		commands: map[string]config.UserCommand{
			"Shared":     {Sh: "plugin"},
			"PluginOnly": {Sh: "plugin-only"},
		},
	})

	system := map[string]config.UserCommand{
		"Shared":     {Sh: "system"},
		"SystemOnly": {Sh: "system-only"},
	}
	user := map[string]config.UserCommand{
		"Shared":   {Sh: "user"},
		"UserOnly": {Sh: "user-only"},
	}

	merged := mgr.MergedCommands(system, user)

	assert.Equal(t, "user", merged["Shared"].Sh)
	assert.Equal(t, "plugin-only", merged["PluginOnly"].Sh)
	assert.Equal(t, "system-only", merged["SystemOnly"].Sh)
	assert.Equal(t, "user-only", merged["UserOnly"].Sh)
}

type mockPlugin struct {
	name     string
	commands map[string]config.UserCommand
}

func (p *mockPlugin) Name() string                            { return p.name }
func (p *mockPlugin) Available() bool                         { return true }
func (p *mockPlugin) Init(_ context.Context) error            { return nil }
func (p *mockPlugin) Close() error                            { return nil }
func (p *mockPlugin) Commands() map[string]config.UserCommand { return p.commands }
func (p *mockPlugin) StatusProvider() StatusProvider          { return nil }
