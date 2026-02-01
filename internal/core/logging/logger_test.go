package logging

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestComponent(t *testing.T) {
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)

	logger := Component("test-component")
	logger.Info().Msg("test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log: %v", err)
	}

	cmp, ok := logEntry["cmp"]
	if !ok {
		t.Fatal("expected 'cmp' key in log output")
	}

	if cmp != "test-component" {
		t.Errorf("Component() cmp = %q, want %q", cmp, "test-component")
	}

	msg, ok := logEntry["message"]
	if !ok {
		t.Fatal("expected 'message' key in log output")
	}

	if msg != "test message" {
		t.Errorf("Component() message = %q, want %q", msg, "test message")
	}
}
