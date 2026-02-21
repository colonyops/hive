package commands

import (
	"encoding/json"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   BatchInput
		wantErr string
	}{
		{
			name:    "empty sessions",
			input:   BatchInput{Sessions: []BatchSession{}},
			wantErr: "sessions",
		},
		{
			name: "missing name",
			input: BatchInput{Sessions: []BatchSession{
				{Remote: "https://github.com/org/repo"},
			}},
			wantErr: "name",
		},
		{
			name: "whitespace name",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "   "},
			}},
			wantErr: "name",
		},
		{
			name: "duplicate names",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "test"},
				{Name: "test"},
			}},
			wantErr: "duplicate",
		},
		{
			name: "invalid session_id uppercase",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "test", SessionID: "ABC123"},
			}},
			wantErr: "session_id",
		},
		{
			name: "invalid session_id with hyphen",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "test", SessionID: "abc-123"},
			}},
			wantErr: "session_id",
		},
		{
			name: "invalid session_id with space",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "test", SessionID: "abc 123"},
			}},
			wantErr: "session_id",
		},
		{
			name: "duplicate session_id",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "test1", SessionID: "abc123"},
				{Name: "test2", SessionID: "abc123"},
			}},
			wantErr: "duplicate session_id",
		},
		{
			name: "valid input",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "session1"},
				{Name: "session2", Remote: "https://github.com/org/repo"},
			}},
			wantErr: "",
		},
		{
			name: "valid input with session_id",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "session1", SessionID: "abc123"},
				{Name: "session2", SessionID: "def456"},
			}},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err, "expected error containing %q, got nil", tt.wantErr)
			assert.Contains(t, err.Error(), tt.wantErr, "expected error containing %q, got %q", tt.wantErr, err.Error())
		})
	}
}

func TestBatchInput_ValidateWithProfiles(t *testing.T) {
	profiles := map[string]config.AgentProfile{
		"claude": {},
		"aider":  {},
	}

	tests := []struct {
		name    string
		input   BatchInput
		wantErr string
	}{
		{
			name: "valid session agent",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "task1", Agent: "claude"},
			}},
		},
		{
			name: "invalid session agent",
			input: BatchInput{Sessions: []BatchSession{
				{Name: "task1", Agent: "unknown"},
			}},
			wantErr: "agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.ValidateWithProfiles(profiles)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestBatchInput_JSON(t *testing.T) {
	jsonInput := `{
		"sessions": [
			{"name": "task1", "session_id": "abc123"},
			{"name": "task2", "remote": "https://github.com/org/repo"}
		]
	}`

	var input BatchInput
	require.NoError(t, json.Unmarshal([]byte(jsonInput), &input))

	assert.Len(t, input.Sessions, 2, "expected 2 sessions, got %d", len(input.Sessions))
	assert.Equal(t, "task1", input.Sessions[0].Name)
	assert.Equal(t, "abc123", input.Sessions[0].SessionID)
	assert.Equal(t, "https://github.com/org/repo", input.Sessions[1].Remote)
}

func TestBatchCmd_resolveAgent(t *testing.T) {
	cmd := &BatchCmd{agent: "aider"}

	assert.Equal(t, "claude", cmd.resolveAgent(BatchSession{Name: "task", Agent: "claude"}))
	assert.Equal(t, "aider", cmd.resolveAgent(BatchSession{Name: "task"}))
}

func TestBatchOutput_JSON(t *testing.T) {
	output := BatchOutput{
		BatchID: "abc123",
		LogFile: "/tmp/logs/batch-abc123.log",
		Results: []BatchResult{
			{Name: "task1", SessionID: "def456", Path: "/tmp/session", Status: StatusCreated},
			{Name: "task2", Status: StatusFailed, Error: "clone failed"},
			{Name: "task3", Status: StatusSkipped},
		},
	}

	data, err := json.Marshal(output)
	require.NoError(t, err)

	var decoded BatchOutput
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "abc123", decoded.BatchID)
	assert.Equal(t, "/tmp/logs/batch-abc123.log", decoded.LogFile)
	assert.Len(t, decoded.Results, 3, "expected 3 results, got %d", len(decoded.Results))
	assert.Equal(t, StatusCreated, decoded.Results[0].Status)
	assert.Equal(t, "clone failed", decoded.Results[1].Error)
	assert.Equal(t, StatusSkipped, decoded.Results[2].Status)
}

func TestBatchErrorOutput_JSON(t *testing.T) {
	output := BatchErrorOutput{Error: "something went wrong"}

	data, err := json.Marshal(output)
	require.NoError(t, err)

	var decoded BatchErrorOutput
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "something went wrong", decoded.Error)
}

func TestCountByStatus(t *testing.T) {
	results := []BatchResult{
		{Status: StatusCreated},
		{Status: StatusCreated},
		{Status: StatusFailed},
		{Status: StatusSkipped},
		{Status: StatusSkipped},
		{Status: StatusSkipped},
	}

	assert.Equal(t, 2, countByStatus(results, StatusCreated), "countByStatus(created) = %d, want 2", countByStatus(results, StatusCreated))
	assert.Equal(t, 1, countByStatus(results, StatusFailed), "countByStatus(failed) = %d, want 1", countByStatus(results, StatusFailed))
	assert.Equal(t, 3, countByStatus(results, StatusSkipped), "countByStatus(skipped) = %d, want 3", countByStatus(results, StatusSkipped))
}
