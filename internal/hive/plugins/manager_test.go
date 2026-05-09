package plugins

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPlugin is a minimal Plugin implementation used by manager-level tests
// to exercise registration, init, and command-slot seeding without spinning
// up a real Lua/GitHub backend.
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

func TestManager_InitAll_SeedsStaticPluginSlots(t *testing.T) {
	set := NewCommandSet(nil, nil)
	mgr := NewManager(NewWorkerPool(0), set)

	stub := &mockPlugin{
		name: "stub",
		commands: map[string]config.UserCommand{
			"Foo": {Sh: "echo foo"},
		},
	}
	mgr.Register(stub)

	require.NoError(t, mgr.InitAll(context.Background()))

	got := set.Plugin("stub")
	require.NotNil(t, got)
	assert.Equal(t, "echo foo", got["Foo"].Sh)
}

func TestManager_InitAll_SkipsPluginsWithNilCommands(t *testing.T) {
	set := NewCommandSet(nil, nil)
	mgr := NewManager(NewWorkerPool(0), set)

	mgr.Register(&mockPlugin{name: "empty", commands: nil})

	require.NoError(t, mgr.InitAll(context.Background()))

	assert.Nil(t, set.Plugin("empty"), "no slot should be created for plugins that return nil commands")
}

// selfRegisteringPlugin populates its slot directly from Init, mirroring
// the Lua plugin's MergePlugin-from-entrypoint behavior. Commands() reads
// back from the slot.
type selfRegisteringPlugin struct {
	name string
	set  *CommandSet
	init map[string]config.UserCommand
	post map[string]config.UserCommand
}

func (p *selfRegisteringPlugin) Name() string    { return p.name }
func (p *selfRegisteringPlugin) Available() bool { return true }
func (p *selfRegisteringPlugin) Init(_ context.Context) error {
	p.set.MergePlugin(p.name, p.init)
	if p.post != nil {
		p.set.MergePlugin(p.name, p.post)
	}
	return nil
}
func (p *selfRegisteringPlugin) Close() error { return nil }
func (p *selfRegisteringPlugin) Commands() map[string]config.UserCommand {
	return p.set.Plugin(p.name)
}
func (p *selfRegisteringPlugin) StatusProvider() StatusProvider { return nil }

func TestManager_InitAll_PreservesSelfRegisteredCommands(t *testing.T) {
	set := NewCommandSet(nil, nil)
	mgr := NewManager(NewWorkerPool(0), set)

	// Simulates a Lua-style plugin whose entrypoint registers Foo and a
	// ticker callback registers Bar between Init returning and the
	// manager's seeding step.
	mgr.Register(&selfRegisteringPlugin{
		name: "self",
		set:  set,
		init: map[string]config.UserCommand{"Foo": {Sh: "echo foo"}},
		post: map[string]config.UserCommand{"Bar": {Sh: "echo bar"}},
	})

	require.NoError(t, mgr.InitAll(context.Background()))

	got := set.Plugin("self")
	require.NotNil(t, got)
	assert.Equal(t, "echo foo", got["Foo"].Sh, "entrypoint registration must survive InitAll")
	assert.Equal(t, "echo bar", got["Bar"].Sh, "post-Init registration must survive InitAll")
}

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
