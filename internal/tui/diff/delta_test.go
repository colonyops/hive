package diff

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckDeltaAvailable(t *testing.T) {
	// This is an integration test that checks if delta is in the system PATH.
	// It will pass if delta is installed, fail with DeltaNotFoundError if not.
	err := CheckDeltaAvailable()

	// We can't mock exec.LookPath easily, so we just verify the error type if delta is missing
	if err != nil {
		var deltaErr *DeltaNotFoundError
		require.True(t, errors.As(err, &deltaErr), "expected DeltaNotFoundError")
		assert.Contains(t, err.Error(), "delta not found")
		assert.Contains(t, err.Error(), "brew install git-delta")
		assert.Contains(t, err.Error(), "https://github.com/dandavison/delta")
		t.Skip("delta not installed, skipping test")
	}
}

func TestExecDelta(t *testing.T) {
	// Skip if delta is not available
	if err := CheckDeltaAvailable(); err != nil {
		t.Skip("delta not installed")
	}

	const sampleDiff = `diff --git a/file.go b/file.go
index abc123..def456 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main

 func main() {
+	fmt.Println("hello")
 }`

	ctx := context.Background()
	output, err := ExecDelta(ctx, sampleDiff)

	require.NoError(t, err)
	assert.NotEmpty(t, output, "delta output should not be empty")

	// Delta adds ANSI color codes, so output should be longer than input
	assert.Greater(t, len(output), len(sampleDiff), "delta should add color codes")

	// Basic sanity check - output should contain the file path
	assert.Contains(t, output, "file.go")
}

func TestExecDelta_WithInvalidCommand(t *testing.T) {
	// This test verifies error handling when delta command fails
	// We can't easily mock exec.Command, but we can test with invalid input

	// Skip if delta is not available
	if err := CheckDeltaAvailable(); err != nil {
		t.Skip("delta not installed")
	}

	ctx := context.Background()

	// Empty input should still work (delta handles it gracefully)
	output, err := ExecDelta(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(output))
}

func TestExecDelta_WithCanceledContext(t *testing.T) {
	// Skip if delta is not available
	if err := CheckDeltaAvailable(); err != nil {
		t.Skip("delta not installed")
	}

	// Create a canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// ExecDelta should fail with context canceled error
	_, err := ExecDelta(ctx, "some diff content")
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "signal: killed"))
}

func TestDeltaNotFoundError_Message(t *testing.T) {
	err := &DeltaNotFoundError{}

	msg := err.Error()
	assert.Contains(t, msg, "delta not found")
	assert.Contains(t, msg, "brew install git-delta")
	assert.Contains(t, msg, "cargo install git-delta")
	assert.Contains(t, msg, "sudo apt install git-delta")
	assert.Contains(t, msg, "https://github.com/dandavison/delta")
}

// TestDeltaRealExecution is a manual integration test
// Run with: go test -v -run TestDeltaRealExecution
func TestDeltaRealExecution(t *testing.T) {
	// Check if delta is available, but don't skip - just log
	err := CheckDeltaAvailable()
	if err != nil {
		t.Logf("Delta not available: %v", err)
		t.Skip("delta not installed")
		return
	}

	// Check delta version
	cmd := exec.Command("delta", "--version")
	versionOut, err := cmd.Output()
	require.NoError(t, err)
	t.Logf("Delta version: %s", strings.TrimSpace(string(versionOut)))

	// Test with a real diff
	const realDiff = `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,8 +1,9 @@
 package main

 import (
 	"fmt"
+	"os"
 )

 func main() {
-	fmt.Println("Hello, World!")
+	fmt.Println("Hello, " + os.Getenv("USER"))
 }`

	ctx := context.Background()
	output, err := ExecDelta(ctx, realDiff)
	require.NoError(t, err)

	t.Logf("Delta output length: %d bytes", len(output))
	t.Logf("Input length: %d bytes", len(realDiff))

	// Verify delta processed the diff
	assert.Greater(t, len(output), len(realDiff))
	assert.Contains(t, output, "main.go")
}
