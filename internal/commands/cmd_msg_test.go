package commands

import (
	"bytes"
	"context"
	"regexp"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestRunTopic_DefaultPrefix(t *testing.T) {
	var buf bytes.Buffer

	cfg := &config.Config{
		Messaging: config.MessagingConfig{
			TopicPrefix: "agent",
		},
	}
	flags := &Flags{}

	cmd := NewMsgCmd(flags, &hive.App{Config: cfg})

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	require.NoError(t, app.Run(context.Background(), []string{"hive", "msg", "topic"}))

	output := buf.String()
	// Remove trailing newline
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be "agent.XXXX" format
	pattern := regexp.MustCompile(`^agent\.[a-z0-9]{4}$`)
	assert.True(t, pattern.MatchString(output), "output %q does not match expected pattern agent.[a-z0-9]{4}", output)
}

func TestRunTopic_CustomPrefixFlag(t *testing.T) {
	var buf bytes.Buffer

	cfg := &config.Config{
		Messaging: config.MessagingConfig{
			TopicPrefix: "agent",
		},
	}
	flags := &Flags{}

	cmd := NewMsgCmd(flags, &hive.App{Config: cfg})

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	require.NoError(t, app.Run(context.Background(), []string{"hive", "msg", "topic", "--prefix", "task"}))

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be "task.XXXX" format (flag overrides config)
	pattern := regexp.MustCompile(`^task\.[a-z0-9]{4}$`)
	assert.True(t, pattern.MatchString(output), "output %q does not match expected pattern task.[a-z0-9]{4}", output)
}

func TestRunTopic_EmptyPrefixFlag(t *testing.T) {
	var buf bytes.Buffer

	cfg := &config.Config{
		Messaging: config.MessagingConfig{
			TopicPrefix: "agent",
		},
	}
	flags := &Flags{}

	cmd := NewMsgCmd(flags, &hive.App{Config: cfg})

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	require.NoError(t, app.Run(context.Background(), []string{"hive", "msg", "topic", "--prefix", ""}))

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be just "XXXX" format (no prefix, no dot)
	pattern := regexp.MustCompile(`^[a-z0-9]{4}$`)
	assert.True(t, pattern.MatchString(output), "output %q does not match expected pattern [a-z0-9]{4}", output)

	// Verify no dot present
	assert.False(t, bytes.Contains([]byte(output), []byte(".")), "output %q should not contain a dot when prefix is empty", output)
}

func TestRunTopic_EmptyConfigPrefix(t *testing.T) {
	var buf bytes.Buffer

	cfg := &config.Config{
		Messaging: config.MessagingConfig{
			TopicPrefix: "", // Empty config prefix
		},
	}
	flags := &Flags{}

	cmd := NewMsgCmd(flags, &hive.App{Config: cfg})

	app := &cli.Command{
		Name:   "hive",
		Writer: &buf,
	}
	cmd.Register(app)

	require.NoError(t, app.Run(context.Background(), []string{"hive", "msg", "topic"}))

	output := buf.String()
	if len(output) > 0 && output[len(output)-1] == '\n' {
		output = output[:len(output)-1]
	}

	// Should be just "XXXX" format (no prefix from config)
	pattern := regexp.MustCompile(`^[a-z0-9]{4}$`)
	assert.True(t, pattern.MatchString(output), "output %q does not match expected pattern [a-z0-9]{4}", output)
}

func TestRunTopic_Uniqueness(t *testing.T) {
	// Generate multiple topic IDs and verify they're unique
	seen := make(map[string]bool)

	for range 10 {
		var buf bytes.Buffer

		cfg := &config.Config{
			Messaging: config.MessagingConfig{
				TopicPrefix: "agent",
			},
		}
		flags := &Flags{}

		cmd := NewMsgCmd(flags, &hive.App{Config: cfg})

		app := &cli.Command{
			Name:   "hive",
			Writer: &buf,
		}
		cmd.Register(app)

		require.NoError(t, app.Run(context.Background(), []string{"hive", "msg", "topic"}))

		output := buf.String()
		if len(output) > 0 && output[len(output)-1] == '\n' {
			output = output[:len(output)-1]
		}

		seen[output] = true
	}

	// Should have generated unique IDs (with 36^4 = 1.6M combinations, duplicates in 10 tries would be very rare)
	assert.GreaterOrEqual(t, len(seen), 9, "generated only %d unique topic IDs in 10 attempts, expected near 10", len(seen))
}
