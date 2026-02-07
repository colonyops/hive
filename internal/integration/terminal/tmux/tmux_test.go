package tmux

import (
	"context"
	"testing"
	"time"
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
				if w != nil {
					t.Fatalf("expected nil, got window %q", w.windowName)
				}
				return
			}
			if w == nil {
				t.Fatal("expected window, got nil")
			}
			if w.windowName != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, w.windowName)
			}
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
				if w != nil {
					t.Fatalf("expected nil, got window %q", w.windowIndex)
				}
				return
			}
			if w == nil {
				t.Fatal("expected window, got nil")
			}
			if w.windowIndex != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, w.windowIndex)
			}
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
	if w == nil || w.windowIndex != "0" {
		t.Fatal("single window should always be returned")
	}
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
	if w == nil || w.windowIndex != "1" {
		t.Fatalf("expected window 1 (path match), got %v", w)
	}
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
	if w == nil || w.windowIndex != "1" {
		t.Fatalf("expected window 1 (name match), got %v", w)
	}
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
	if w == nil || w.windowIndex != "1" {
		t.Fatalf("expected window 1 (highest activity), got %v", w)
	}
}

func TestSessionInfoFromWindow(t *testing.T) {
	integ := New(nil)

	t.Run("nil window", func(t *testing.T) {
		sc := &sessionCache{}
		info := integ.sessionInfoFromWindow("mysess", sc, nil)
		if info.Name != "mysess" || info.Pane != "" || info.WindowName != "" {
			t.Fatalf("unexpected info: %+v", info)
		}
	})

	t.Run("single window omits WindowName", func(t *testing.T) {
		w := &agentWindow{windowIndex: "2", windowName: "claude"}
		sc := &sessionCache{agentWindows: []*agentWindow{w}}
		info := integ.sessionInfoFromWindow("mysess", sc, w)
		if info.Name != "mysess" || info.Pane != "2" || info.WindowName != "" {
			t.Fatalf("single window should not set WindowName: %+v", info)
		}
	})

	t.Run("multi window sets WindowName", func(t *testing.T) {
		w1 := &agentWindow{windowIndex: "0", windowName: "claude"}
		w2 := &agentWindow{windowIndex: "1", windowName: "aider"}
		sc := &sessionCache{agentWindows: []*agentWindow{w1, w2}}
		info := integ.sessionInfoFromWindow("mysess", sc, w2)
		if info.Name != "mysess" || info.Pane != "1" || info.WindowName != "aider" {
			t.Fatalf("multi window should set WindowName: %+v", info)
		}
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
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			t.Fatal("expected info")
		}
		if info.Pane != "1" {
			t.Fatalf("expected pane 1 (path match), got %q", info.Pane)
		}
		if info.WindowName != "claude" {
			t.Fatalf("expected window name 'claude', got %q", info.WindowName)
		}
	})

	t.Run("no path match falls to activity", func(t *testing.T) {
		// Set different activities
		integ.cache["my-session"].agentWindows[0].activity = 200
		integ.cache["my-session"].agentWindows[1].activity = 100

		info, err := integ.DiscoverSession(ctx, "my-session", map[string]string{
			sessionPathKey: "/nonexistent",
		})
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			t.Fatal("expected info")
		}
		// No path match, no name match for slug "my-session" in window names — should pick highest activity
		if info.Pane != "0" {
			t.Fatalf("expected pane 0 (highest activity), got %q", info.Pane)
		}
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
			if got != tt.want {
				t.Fatalf("matchesPreferredWindow(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// timeNow returns a time that makes cache fresh for tests.
func timeNow() (t time.Time) {
	return time.Now()
}
