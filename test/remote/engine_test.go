//go:build remote

package remote

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func TestClientCLI_TakesTheNextTaskOfAnEpicOnAServer(t *testing.T) {
	title := fmt.Sprintf("remote next %d", time.Now().UnixNano())
	input := fmt.Sprintf(`{"title":"remote epic","type":"epic","repo":"acme/remote","children":[{"title":%q}]}`, title)
	created := DecodeJSONLines[taskItem](t, serverA.Hive(t, "hc", "create", "--caller", "global", "--input", input).Ok(t))
	require.Len(t, created, 2)
	epic := created[0].ID
	client := NewClient(t)

	next := DecodeJSON[taskItem](t, client.Hive("hc", "next", "--server", serverA.Name, epic).Ok(t))

	assert.Equal(t, created[1], next)
	onB := client.Hive("hc", "next", "--server", serverB.Name, epic).Fails(t)
	assert.Contains(t, ErrorMessage(t, onB), "no actionable task", "the epic is on server-a only")
	client.Hive("hc", "next", epic).Fails(t)
}

type documentNode struct {
	Path string `json:"path"`
}

func TestClientCLI_ListsTheDocumentsOfAServer(t *testing.T) {
	dir := fmt.Sprintf("remote-%d", time.Now().UnixNano())
	path := dir + "/notes.md"
	serverA.Hive(t, "drive", "put", "--caller", "global", path, "--body", "# from server-a\n").Ok(t)
	t.Cleanup(func() { serverA.Hive(t, "drive", "delete", "--caller", "global", path) })

	client := NewClient(t)

	// The server's daemon learns of a document its CLI wrote from its file
	// watcher, which debounces.
	var listed Result
	require.Eventually(t, func() bool {
		listed = client.Hive("drive", "list", "--server", serverA.Name, dir)
		return listed.Err == nil
	}, 10*time.Second, 250*time.Millisecond, "server-a never listed %s: %s", dir, listed.Stderr)
	assert.Equal(t, []documentNode{{Path: path}}, DecodeJSONLines[documentNode](t, listed))
	onB := client.Hive("drive", "list", "--server", serverB.Name, dir).Fails(t)
	assert.Contains(t, onB.Stderr, `"kind":"not_found"`)
}
