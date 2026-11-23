package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/ops"
)

// driveRoot registers hive drive and the groups the engine commands join.
func driveRoot(a *app.App) func(*cli.Command) *cli.Command {
	flags := &Flags{}
	return func(root *cli.Command) *cli.Command {
		for _, register := range []func(*cli.Command) *cli.Command{
			NewDriveCmd(flags, a).Register,
			NewHoneycombCmd(flags, a).Register,
			NewMsgCmd(flags, a).Register,
			NewTodoCmd(flags, a).Register,
			NewEngineCmd(flags, a).Register,
		} {
			root = register(root)
		}
		return root
	}
}

// No desktop runs in these tests: the data directory has no endpoint file, so
// a call that needed the desktop would fail with desktopNotRunningError.
func TestDrivePutWritesWithTheDesktopClosed(t *testing.T) {
	a := testApp(t)

	out, err := runRoot(t, driveRoot(a), "drive", "put", "--caller", "global", "reports/probe.md", "--body", "# probe\n")
	require.NoError(t, err)
	var entry map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &entry))
	assert.Equal(t, "reports/probe.md", entry["path"])
	assert.Equal(t, "markdown", entry["kind"])

	body, err := os.ReadFile(filepath.Join(a.Config.ContextDir(), "shared", "reports", "probe.md"))
	require.NoError(t, err)
	assert.Equal(t, "# probe\n", string(body))

	out, err = runRoot(t, driveRoot(a), "drive", "get", "--caller", "global", "reports/probe.md")
	require.NoError(t, err)
	assert.Contains(t, out, `"body":"# probe\n"`)

	out, err = runRoot(t, driveRoot(a), "drive", "list", "--caller", "global", "--detail", "tree")
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 1, "one line per child of the root")
	assert.Contains(t, lines[0], `"path":"reports"`)
}

// The desktop's TestWriteDocumentRefusals checks the same text over MCP.
func TestDrivePutRefusesTheSameOversizedBody(t *testing.T) {
	input := filepath.Join(t.TempDir(), "big.json")
	raw, err := json.Marshal(map[string]any{"path": "big.md", "body": strings.Repeat("x", drivesvc.MaxDocumentBytes+1)})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(input, raw, 0o600))

	_, err = runRoot(t, driveRoot(testApp(t)), "drive", "put", "--caller", "global", "--input", "@"+input)

	require.Error(t, err)
	assert.Equal(t, ops.KindInvalid, ErrorKind(err))
	assert.Equal(t, "invalid: document is too large (2097152 bytes max)", err.Error())
}

func TestDriveDocumentCommandsRefuseSpaceFlags(t *testing.T) {
	_, err := runRoot(t, driveRoot(testApp(t)), "drive", "--shared", "put", "--caller", "global", "a.md", "--body", "x")
	require.Error(t, err)
	assert.Equal(t, ops.KindInvalid, ErrorKind(err))
	assert.Contains(t, err.Error(), "--shared")
}

func TestDriveCallsOfAChatGoToTheDesktop(t *testing.T) {
	_, err := runRoot(t, driveRoot(testApp(t)), "drive", "put", "--caller", "0b5f3c2e-7a41-4d8e-9c3a-2f6d1e8b4a90", "a.md", "--body", "x")
	require.Error(t, err)
	assert.ErrorAs(t, err, new(desktopNotRunningError), "a chat's documents need the desktop's chat records")
}
