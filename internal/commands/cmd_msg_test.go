package commands

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/hay-kot/hive/internal/core/config"
	"github.com/urfave/cli/v3"
)

func TestRunTopic_DefaultPrefix(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{
		Config: &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "agent",
			},
		},
	}

	cmd := NewMsgCmd(flags)

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "msg", "topic"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Remove trailing newline
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be "agent.XXXX" format
	pattern := regexp.MustCompile(`^agent\.[a-z0-9]{4}$`)
	if !pattern.MatchString(output) {
		t.Errorf("output %q does not match expected pattern agent.[a-z0-9]{4}", output)
	}
}

func TestRunTopic_CustomPrefixFlag(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{
		Config: &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "agent",
			},
		},
	}

	cmd := NewMsgCmd(flags)

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "msg", "topic", "--prefix", "task"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be "task.XXXX" format (flag overrides config)
	pattern := regexp.MustCompile(`^task\.[a-z0-9]{4}$`)
	if !pattern.MatchString(output) {
		t.Errorf("output %q does not match expected pattern task.[a-z0-9]{4}", output)
	}
}

func TestRunTopic_EmptyPrefixFlag(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{
		Config: &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "agent",
			},
		},
	}

	cmd := NewMsgCmd(flags)

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "msg", "topic", "--prefix", ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be just "XXXX" format (no prefix, no dot)
	pattern := regexp.MustCompile(`^[a-z0-9]{4}$`)
	if !pattern.MatchString(output) {
		t.Errorf("output %q does not match expected pattern [a-z0-9]{4}", output)
	}

	// Verify no dot present
	if bytes.Contains([]byte(output), []byte(".")) {
		t.Errorf("output %q should not contain a dot when prefix is empty", output)
	}
}

func TestRunTopic_EmptyConfigPrefix(t *testing.T) {
	var buf bytes.Buffer

	flags := &Flags{
		Config: &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "", // Empty config prefix
			},
		},
	}

	cmd := NewMsgCmd(flags)

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	err := app.Run(context.Background(), []string{"hive", "msg", "topic"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be just "XXXX" format (no prefix from config)
	pattern := regexp.MustCompile(`^[a-z0-9]{4}$`)
	if !pattern.MatchString(output) {
		t.Errorf("output %q does not match expected pattern [a-z0-9]{4}", output)
	}
}

func TestRunTopic_Uniqueness(t *testing.T) {
	// Generate multiple topic IDs and verify they're unique
	seen := make(map[string]bool)

	for range 10 {
		var buf bytes.Buffer

		flags := &Flags{
			Config: &config.Config{
				Messaging: config.MessagingConfig{
					TopicPrefix: "agent",
				},
			},
		}

		cmd := NewMsgCmd(flags)

		app := &cli.Command{
			Name:   "hive",
			Writer: &buf,
		}
		cmd.Register(app)

		err := app.Run(context.Background(), []string{"hive", "msg", "topic"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if len(output) > 0 && output[len(output)-1] == '\n' {
			output = output[:len(output)-1]
		}

		seen[output] = true
	}

	// Should have generated unique IDs (with 36^4 = 1.6M combinations, duplicates in 10 tries would be very rare)
	if len(seen) < 9 {
		t.Errorf("generated only %d unique topic IDs in 10 attempts, expected near 10", len(seen))
	}
}
