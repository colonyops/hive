package logutils_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colonyops/hive/pkg/logutils"
)

// TestNewAppendsAcrossReopens verifies that opening the same log file twice
// preserves earlier log entries. This guards against regressions where the
// file is opened with O_TRUNC and a second logger would wipe the parent's log
// (relevant for detached child processes like `hive timer-fire`).
func TestNewAppendsAcrossReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive.log")

	logger1, closer1, err := logutils.New("info", path)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	logger1.Info().Msg("first")
	closer1()

	logger2, closer2, err := logutils.New("info", path)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	logger2.Info().Msg("second")
	closer2()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	contents := string(data)

	if !strings.Contains(contents, "first") {
		t.Errorf("expected log to contain %q after reopen; got:\n%s", "first", contents)
	}
	if !strings.Contains(contents, "second") {
		t.Errorf("expected log to contain %q after reopen; got:\n%s", "second", contents)
	}
}
