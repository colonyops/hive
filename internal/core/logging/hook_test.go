package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
)

func TestContextHook_Run(t *testing.T) {
	tests := []struct {
		name      string
		setupCtx  func() context.Context
		wantKeys  []string
		wantEmpty []string
	}{
		{
			name: "both session_id and agent_id",
			setupCtx: func() context.Context {
				ctx := context.Background()
				ctx = WithSessionID(ctx, "sess-123")
				ctx = WithAgentID(ctx, "agent-456")
				return ctx
			},
			wantKeys: []string{"session_id", "agent_id"},
		},
		{
			name: "only session_id",
			setupCtx: func() context.Context {
				return WithSessionID(context.Background(), "sess-123")
			},
			wantKeys:  []string{"session_id"},
			wantEmpty: []string{"agent_id"},
		},
		{
			name: "only agent_id",
			setupCtx: func() context.Context {
				return WithAgentID(context.Background(), "agent-456")
			},
			wantKeys:  []string{"agent_id"},
			wantEmpty: []string{"session_id"},
		},
		{
			name:      "no context values",
			setupCtx:  context.Background,
			wantEmpty: []string{"session_id", "agent_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := tt.setupCtx()

			logger := zerolog.New(&buf).Hook(ContextHook{})
			logger.Info().Ctx(ctx).Msg("test")

			var logEntry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("failed to parse log: %v", err)
			}

			for _, key := range tt.wantKeys {
				if _, ok := logEntry[key]; !ok {
					t.Errorf("expected %s to be present in log", key)
				}
			}

			for _, key := range tt.wantEmpty {
				if _, ok := logEntry[key]; ok {
					t.Errorf("expected %s to be absent from log", key)
				}
			}
		})
	}
}
