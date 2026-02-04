package config

import "testing"

func TestDefaultUserCommands_IncludesDocReview(t *testing.T) {
	// Verify DocReview is in default commands
	cmd, exists := defaultUserCommands["DocReview"]
	if !exists {
		t.Fatal("DocReview command not found in defaultUserCommands")
	}

	// Verify it has the correct action
	if cmd.Action != ActionDocReview {
		t.Errorf("Expected action %q, got %q", ActionDocReview, cmd.Action)
	}

	// Verify it has help text
	if cmd.Help == "" {
		t.Error("Expected non-empty help text")
	}

	// Verify it's silent (no loading popup)
	if !cmd.Silent {
		t.Error("Expected DocReview command to be silent")
	}
}

func TestMergedUserCommands_IncludesDocReview(t *testing.T) {
	cfg := DefaultConfig()
	merged := cfg.MergedUserCommands()

	// Verify DocReview is included in merged commands
	cmd, exists := merged["DocReview"]
	if !exists {
		t.Fatal("DocReview command not found in merged user commands")
	}

	if cmd.Action != ActionDocReview {
		t.Errorf("Expected action %q, got %q", ActionDocReview, cmd.Action)
	}
}

func TestUserCanOverrideDocReview(t *testing.T) {
	cfg := DefaultConfig()
	// User overrides with custom help text
	cfg.UserCommands = map[string]UserCommand{
		"DocReview": {
			Action: ActionDocReview,
			Help:   "custom review help",
		},
	}

	merged := cfg.MergedUserCommands()
	cmd := merged["DocReview"]

	if cmd.Help != "custom review help" {
		t.Errorf("Expected custom help text, got %q", cmd.Help)
	}
}
