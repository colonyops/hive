package plugins

import (
	"sync"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/stretchr/testify/assert"
)

func TestCommandSet_PrecedenceOrdering(t *testing.T) {
	system := map[string]config.UserCommand{"Foo": {Sh: "S"}}
	user := map[string]config.UserCommand{"Foo": {Sh: "U"}}

	// All three sources contribute Foo: user wins.
	s := NewCommandSet(system, user)
	s.SetPlugin("p", map[string]config.UserCommand{"Foo": {Sh: "P"}})

	got, ok := s.Lookup("Foo")
	assert.True(t, ok)
	assert.Equal(t, "U", got.Sh)
	assert.Equal(t, "U", s.All()["Foo"].Sh)

	// Drop user: plugin wins.
	s = NewCommandSet(system, nil)
	s.SetPlugin("p", map[string]config.UserCommand{"Foo": {Sh: "P"}})

	got, ok = s.Lookup("Foo")
	assert.True(t, ok)
	assert.Equal(t, "P", got.Sh)
	assert.Equal(t, "P", s.All()["Foo"].Sh)

	// Drop user and plugin: system wins.
	s = NewCommandSet(system, nil)

	got, ok = s.Lookup("Foo")
	assert.True(t, ok)
	assert.Equal(t, "S", got.Sh)
	assert.Equal(t, "S", s.All()["Foo"].Sh)
}

func TestCommandSet_SetPlugin_ReplacesSlot(t *testing.T) {
	s := NewCommandSet(nil, nil)

	s.SetPlugin("test", map[string]config.UserCommand{
		"A": {Sh: "a"},
		"B": {Sh: "b"},
	})
	s.SetPlugin("test", map[string]config.UserCommand{
		"C": {Sh: "c"},
	})

	got := s.Plugin("test")
	assert.Len(t, got, 1)
	_, hasA := got["A"]
	_, hasB := got["B"]
	assert.False(t, hasA)
	assert.False(t, hasB)
	assert.Equal(t, "c", got["C"].Sh)
}

func TestCommandSet_SetPluginNil_ClearsSlot(t *testing.T) {
	s := NewCommandSet(nil, nil)

	s.SetPlugin("test", map[string]config.UserCommand{"A": {Sh: "a"}})
	s.SetPlugin("test", nil)

	assert.Nil(t, s.Plugin("test"))

	all := s.All()
	_, hasA := all["A"]
	assert.False(t, hasA)
}

func TestCommandSet_DefensiveCopy_All(t *testing.T) {
	s := NewCommandSet(
		map[string]config.UserCommand{"Sys": {Sh: "s"}},
		map[string]config.UserCommand{"Usr": {Sh: "u"}},
	)
	s.SetPlugin("p", map[string]config.UserCommand{"Plug": {Sh: "p"}})

	first := s.All()
	first["Injected"] = config.UserCommand{Sh: "x"}
	mutated := first["Sys"]
	mutated.Sh = "MUTATED"
	first["Sys"] = mutated

	second := s.All()
	_, hasInjected := second["Injected"]
	assert.False(t, hasInjected)
	assert.Equal(t, "s", second["Sys"].Sh)
	assert.Equal(t, "u", second["Usr"].Sh)
	assert.Equal(t, "p", second["Plug"].Sh)
}

func TestCommandSet_DefensiveCopy_Plugin(t *testing.T) {
	s := NewCommandSet(nil, nil)
	s.SetPlugin("test", map[string]config.UserCommand{"A": {Sh: "a"}})

	first := s.Plugin("test")
	first["Injected"] = config.UserCommand{Sh: "x"}
	mutated := first["A"]
	mutated.Sh = "MUTATED"
	first["A"] = mutated

	second := s.Plugin("test")
	_, hasInjected := second["Injected"]
	assert.False(t, hasInjected)
	assert.Equal(t, "a", second["A"].Sh)

	// And Lookup is unaffected.
	got, ok := s.Lookup("A")
	assert.True(t, ok)
	assert.Equal(t, "a", got.Sh)
}

func TestCommandSet_NewCommandSet_DefensiveCopy(t *testing.T) {
	system := map[string]config.UserCommand{"Sys": {Sh: "s"}}
	user := map[string]config.UserCommand{"Usr": {Sh: "u"}}

	s := NewCommandSet(system, user)

	// Mutate input maps after construction.
	system["Sys"] = config.UserCommand{Sh: "MUTATED"}
	system["Injected"] = config.UserCommand{Sh: "x"}
	user["Usr"] = config.UserCommand{Sh: "MUTATED"}
	user["Injected"] = config.UserCommand{Sh: "x"}

	all := s.All()
	assert.Equal(t, "s", all["Sys"].Sh)
	assert.Equal(t, "u", all["Usr"].Sh)
	_, hasInjected := all["Injected"]
	assert.False(t, hasInjected)
}

func TestCommandSet_Lookup_NoAllocations(t *testing.T) {
	s := NewCommandSet(
		map[string]config.UserCommand{"OnlySys": {Sh: "s"}},
		map[string]config.UserCommand{"OnlyUsr": {Sh: "u"}},
	)
	s.SetPlugin("p", map[string]config.UserCommand{"OnlyPlug": {Sh: "p"}})

	cases := []struct {
		name string
		key  string
	}{
		{"user", "OnlyUsr"},
		{"plugin", "OnlyPlug"},
		{"system", "OnlySys"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := testing.AllocsPerRun(100, func() {
				_, ok := s.Lookup(tc.key)
				if !ok {
					t.Fatalf("Lookup(%q) returned !ok", tc.key)
				}
			})
			assert.InDelta(t, 0.0, got, 0, "Lookup must not allocate (%s slot)", tc.name)
		})
	}
}

func TestCommandSet_ConcurrentReadersWriters(t *testing.T) {
	const sharedKey = "Shared"

	s := NewCommandSet(
		map[string]config.UserCommand{"Sys": {Sh: "s"}},
		map[string]config.UserCommand{"Usr": {Sh: "u"}},
	)
	// Seed a few plugin slots so readers have something to find.
	for i := range 4 {
		s.SetPlugin(pluginName(i), map[string]config.UserCommand{
			sharedKey: {Sh: "p"},
		})
	}

	const writers = 10
	const writerIters = 100
	const readers = 10
	const readerIters = 1000

	var wg sync.WaitGroup
	start := make(chan struct{})

	for w := range writers {
		wg.Go(func() {
			<-start
			pname := pluginName(w)
			for range writerIters {
				s.SetPlugin(pname, map[string]config.UserCommand{
					sharedKey: {Sh: "set"},
				})
			}
		})
	}

	for range readers {
		wg.Go(func() {
			<-start
			for range readerIters {
				_, _ = s.Lookup(sharedKey)
				_, _ = s.Lookup("Sys")
				_, _ = s.Lookup("Usr")
				_ = s.All()
			}
		})
	}

	close(start)
	wg.Wait()
}

func pluginName(i int) string {
	return "p-" + string(rune('a'+i))
}
