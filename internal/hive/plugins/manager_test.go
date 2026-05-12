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
// up a real backend.
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

// initUpdatingPlugin mutates its command set during Init so InitAll seeds the
// post-init command map rather than the pre-init one.
type initUpdatingPlugin struct {
	name     string
	commands map[string]config.UserCommand
	updated  map[string]config.UserCommand
}

func (p *initUpdatingPlugin) Name() string    { return p.name }
func (p *initUpdatingPlugin) Available() bool { return true }
func (p *initUpdatingPlugin) Init(_ context.Context) error {
	p.commands = p.updated
	return nil
}
func (p *initUpdatingPlugin) Close() error                            { return nil }
func (p *initUpdatingPlugin) Commands() map[string]config.UserCommand { return p.commands }
func (p *initUpdatingPlugin) StatusProvider() StatusProvider          { return nil }

func TestManager_InitAll_SeedsCommandsObservedAfterInit(t *testing.T) {
	set := NewCommandSet(nil, nil)
	mgr := NewManager(NewWorkerPool(0), set)

	mgr.Register(&initUpdatingPlugin{
		name:     "self",
		commands: map[string]config.UserCommand{"Old": {Sh: "echo old"}},
		updated:  map[string]config.UserCommand{"Foo": {Sh: "echo foo"}, "Bar": {Sh: "echo bar"}},
	})

	require.NoError(t, mgr.InitAll(context.Background()))

	got := set.Plugin("self")
	require.NotNil(t, got)
	assert.Equal(t, "echo foo", got["Foo"].Sh)
	assert.Equal(t, "echo bar", got["Bar"].Sh)
	_, hasOld := got["Old"]
	assert.False(t, hasOld)
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
