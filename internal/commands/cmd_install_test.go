package commands

import (
	"testing"
)

func TestAddHookEntry(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() map[string][]hookEntry // initial hooks state
		event   string
		matcher string
		want    bool // expected return value
		wantLen int  // expected len of hooks[event] after call
	}{
		{
			name:    "new event adds entry",
			setup:   func() map[string][]hookEntry { return make(map[string][]hookEntry) },
			event:   "SessionStart",
			matcher: "",
			want:    true,
			wantLen: 1,
		},
		{
			name: "identical call is no-op",
			setup: func() map[string][]hookEntry {
				h := make(map[string][]hookEntry)
				addHookEntry(h, "SessionStart", "")
				return h
			},
			event:   "SessionStart",
			matcher: "",
			want:    false,
			wantLen: 1,
		},
		{
			name: "different matcher adds new entry",
			setup: func() map[string][]hookEntry {
				h := make(map[string][]hookEntry)
				addHookEntry(h, "Notification", "")
				return h
			},
			event:   "Notification",
			matcher: "permission_prompt|elicitation_dialog",
			want:    true,
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hooks := tt.setup()
			got := addHookEntry(hooks, tt.event, tt.matcher)
			if got != tt.want {
				t.Errorf("addHookEntry() = %v, want %v", got, tt.want)
			}
			if len(hooks[tt.event]) != tt.wantLen {
				t.Errorf("len(hooks[%q]) = %d, want %d", tt.event, len(hooks[tt.event]), tt.wantLen)
			}
		})
	}
}
