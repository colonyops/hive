package tmux

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionCache_FindWindow(t *testing.T) {
	sc := &sessionCache{
		agentWindows: []*agentWindow{
			{windowIndex: "0", windowName: "bash"},
			{windowIndex: "1", windowName: "claude"},
			{windowIndex: "2", windowName: "vim"},
		},
	}

	tests := []struct {
		name    string
		index   string
		wantNil bool
		want    string
	}{
		{"found", "1", false, "claude"},
		{"not found", "5", true, ""},
		{"first", "0", false, "bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := sc.findWindow(tt.index)
			if tt.wantNil {
				assert.Nil(t, w, "expected nil, got window")
				return
			}
			require.NotNil(t, w, "expected window, got nil")
			assert.Equal(t, tt.want, w.windowName)
		})
	}
}

func TestSessionCache_BestWindow(t *testing.T) {
	tests := []struct {
		name    string
		windows []*agentWindow
		want    string
	}{
		{
			"single",
			[]*agentWindow{{windowIndex: "0", activity: 100}},
			"0",
		},
		{
			"highest activity",
			[]*agentWindow{
				{windowIndex: "0", activity: 100},
				{windowIndex: "1", activity: 200},
				{windowIndex: "2", activity: 150},
			},
			"1",
		},
		{
			"empty",
			nil,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := &sessionCache{agentWindows: tt.windows}
			w := sc.bestWindow()
			if tt.want == "" {
				assert.Nil(t, w, "expected nil, got window")
				return
			}
			require.NotNil(t, w, "expected window, got nil")
			assert.Equal(t, tt.want, w.windowIndex)
		})
	}
}

func TestDisambiguateWindow_SingleWindow(t *testing.T) {
	integ := New(nil)
	sc := &sessionCache{
		agentWindows: []*agentWindow{
			{windowIndex: "0", windowName: "bash", workDir: "/home/user"},
		},
	}

	w := integ.disambiguateWindow(sc, "/other/path", "myslug")
	require.NotNil(t, w, "single window should always be returned")
	assert.Equal(t, "0", w.windowIndex)
}

func TestDisambiguateWindow_PathMatch(t *testing.T) {
	integ := New(nil)
	sc := &sessionCache{
		agentWindows: []*agentWindow{
			{windowIndex: "0", windowName: "claude", workDir: "/home/user/project-a"},
			{windowIndex: "1", windowName: "claude", workDir: "/home/user/project-b"},
		},
	}

	w := integ.disambiguateWindow(sc, "/home/user/project-b", "myslug")
	require.NotNil(t, w, "expected window 1 (path match), got nil")
	assert.Equal(t, "1", w.windowIndex)
}

func TestDisambiguateWindow_NameMatch(t *testing.T) {
	integ := New(nil)
	sc := &sessionCache{
		agentWindows: []*agentWindow{
			{windowIndex: "0", windowName: "claude", workDir: "/a"},
			{windowIndex: "1", windowName: "myslug-work", workDir: "/b"},
		},
	}

	// No path match, should fall back to name match
	w := integ.disambiguateWindow(sc, "/nonexistent", "myslug")
	require.NotNil(t, w, "expected window 1 (name match), got nil")
	assert.Equal(t, "1", w.windowIndex)
}

func TestDisambiguateWindow_FallbackToActivity(t *testing.T) {
	integ := New(nil)
	sc := &sessionCache{
		agentWindows: []*agentWindow{
			{windowIndex: "0", windowName: "bash", workDir: "/a", activity: 100},
			{windowIndex: "1", windowName: "zsh", workDir: "/b", activity: 200},
		},
	}

	// No path match, no name match — should pick highest activity
	w := integ.disambiguateWindow(sc, "/nonexistent", "notfound")
	require.NotNil(t, w, "expected window 1 (highest activity), got nil")
	assert.Equal(t, "1", w.windowIndex)
}

func TestSessionInfoFromWindow(t *testing.T) {
	integ := New(nil)

	t.Run("nil window", func(t *testing.T) {
		sc := &sessionCache{}
		info := integ.sessionInfoFromWindow("mysess", sc, nil)
		assert.Equal(t, "mysess", info.Name)
		assert.Empty(t, info.WindowIndex)
		assert.Empty(t, info.WindowName)
	})

	t.Run("single window sets WindowName", func(t *testing.T) {
		w := &agentWindow{windowIndex: "2", windowName: "claude"}
		sc := &sessionCache{agentWindows: []*agentWindow{w}}
		info := integ.sessionInfoFromWindow("mysess", sc, w)
		assert.Equal(t, "mysess", info.Name)
		assert.Equal(t, "2", info.WindowIndex)
		assert.Equal(t, "claude", info.WindowName)
	})

	t.Run("multi window sets WindowName", func(t *testing.T) {
		w1 := &agentWindow{windowIndex: "0", windowName: "claude"}
		w2 := &agentWindow{windowIndex: "1", windowName: "aider"}
		sc := &sessionCache{agentWindows: []*agentWindow{w1, w2}}
		info := integ.sessionInfoFromWindow("mysess", sc, w2)
		assert.Equal(t, "mysess", info.Name)
		assert.Equal(t, "1", info.WindowIndex)
		assert.Equal(t, "aider", info.WindowName)
	})
}

func TestDiscoverSession_MultiWindow(t *testing.T) {
	integ := New([]string{"claude"})

	// Manually populate cache with multi-window session
	integ.cache = map[string]*sessionCache{
		"my-session": {
			agentWindows: []*agentWindow{
				{windowIndex: "0", windowName: "claude", workDir: "/project-a"},
				{windowIndex: "1", windowName: "claude", workDir: "/project-b"},
			},
		},
	}
	integ.cacheTime = timeNow() // fresh cache

	ctx := context.Background()

	t.Run("path match selects correct window", func(t *testing.T) {
		info, err := integ.DiscoverSession(ctx, "my-session", map[string]string{
			sessionPathKey: "/project-b",
		})
		require.NoError(t, err)
		require.NotNil(t, info, "expected info")
		assert.Equal(t, "1", info.WindowIndex)
		assert.Equal(t, "claude", info.WindowName)
	})

	t.Run("no path match falls to activity", func(t *testing.T) {
		// Set different activities
		integ.cache["my-session"].agentWindows[0].activity = 200
		integ.cache["my-session"].agentWindows[1].activity = 100

		info, err := integ.DiscoverSession(ctx, "my-session", map[string]string{
			sessionPathKey: "/nonexistent",
		})
		require.NoError(t, err)
		require.NotNil(t, info, "expected info")
		// No path match, no name match for slug "my-session" in window names — should pick highest activity
		assert.Equal(t, "0", info.WindowIndex)
	})
}

func TestMatchesPreferredWindow(t *testing.T) {
	integ := New([]string{"claude", "aider", "codex"})

	tests := []struct {
		name string
		want bool
	}{
		{"claude", true},
		{"Claude-Work", true},
		{"aider", true},
		{"codex", true},
		{"Codex-Agent", true},
		{"bash", false},
		{"vim", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := integ.matchesPreferredWindow(tt.name)
			assert.Equal(t, tt.want, got, "matchesPreferredWindow(%q) = %v, want %v", tt.name, got, tt.want)
		})
	}
}

func TestDiscoverAllWindows(t *testing.T) {
	integ := New([]string{"claude", "aider"})

	// Populate cache with a multi-window session and a single-window session
	integ.cache = map[string]*sessionCache{
		"multi-sess": {
			agentWindows: []*agentWindow{
				{windowIndex: "0", windowName: "claude", workDir: "/project-a"},
				{windowIndex: "1", windowName: "aider", workDir: "/project-b"},
			},
		},
		"single-sess": {
			agentWindows: []*agentWindow{
				{windowIndex: "0", windowName: "claude", workDir: "/project-c"},
			},
		},
	}
	integ.cacheTime = time.Now()
	ctx := context.Background()

	t.Run("returns all windows for multi-window session", func(t *testing.T) {
		infos, err := integ.DiscoverAllWindows(ctx, "multi-sess", nil)
		require.NoError(t, err)
		require.Len(t, infos, 2, "expected 2 windows, got %d", len(infos))
		assert.Equal(t, "0", infos[0].WindowIndex)
		assert.Equal(t, "claude", infos[0].WindowName)
		assert.Equal(t, "1", infos[1].WindowIndex)
		assert.Equal(t, "aider", infos[1].WindowName)
	})

	t.Run("returns single window for single-window session", func(t *testing.T) {
		infos, err := integ.DiscoverAllWindows(ctx, "single-sess", nil)
		require.NoError(t, err)
		require.Len(t, infos, 1, "expected 1 window, got %d", len(infos))
		assert.Equal(t, "0", infos[0].WindowIndex)
	})

	t.Run("returns nil for unknown session", func(t *testing.T) {
		infos, err := integ.DiscoverAllWindows(ctx, "nonexistent", nil)
		require.NoError(t, err)
		assert.Nil(t, infos, "expected nil, got %v", infos)
	})

	t.Run("returns nil with stale cache", func(t *testing.T) {
		integ.cacheTime = time.Now().Add(-5 * time.Second)
		infos, err := integ.DiscoverAllWindows(ctx, "multi-sess", nil)
		require.NoError(t, err)
		assert.Nil(t, infos, "expected nil with stale cache, got %v", infos)
	})

	t.Run("prefix match finds session", func(t *testing.T) {
		// Add a session keyed with a slug prefix
		integ.cache["myslug-extra"] = &sessionCache{
			agentWindows: []*agentWindow{
				{windowIndex: "0", windowName: "claude"},
				{windowIndex: "1", windowName: "aider"},
			},
		}
		integ.cacheTime = time.Now()

		infos, err := integ.DiscoverAllWindows(ctx, "myslug", nil)
		require.NoError(t, err)
		require.Len(t, infos, 2, "expected 2 windows via prefix match, got %d", len(infos))
		assert.Equal(t, "myslug-extra", infos[0].Name)
	})

	t.Run("metadata match takes precedence over prefix", func(t *testing.T) {
		integ.cacheTime = time.Now()
		infos, err := integ.DiscoverAllWindows(ctx, "myslug", map[string]string{
			"tmux_session": "multi-sess",
		})
		require.NoError(t, err)
		require.Len(t, infos, 2, "expected 2 windows, got %d", len(infos))
		assert.Equal(t, "multi-sess", infos[0].Name)
	})
}

// timeNow returns a time that makes cache fresh for tests.
func timeNow() (t time.Time) {
	return time.Now()
}
