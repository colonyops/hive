package hive

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/todo"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTodo() todo.Todo {
	return todo.Todo{
		ID:        "test-1",
		SessionID: "sess-1",
		Source:    todo.SourceAgent,
		Category:  todo.CategoryReview,
		Title:     "Review API research",
		Ref:       ".hive/research/api.md",
		Status:    todo.StatusPending,
		CreatedAt: time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC),
	}
}

func TestTodoExporter_AppendMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todos.md")

	exp, err := NewTodoExporter(config.TodosExportConfig{
		Enabled: true,
		Path:    path,
	}, zerolog.Nop())
	require.NoError(t, err)

	// First export creates the file
	err = exp.Export([]todo.Todo{testTodo()})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Review API research")
	assert.Contains(t, string(data), ".hive/research/api.md")

	// Second export appends
	t2 := testTodo()
	t2.ID = "test-2"
	t2.Title = "Second item"
	err = exp.Export([]todo.Todo{t2})
	require.NoError(t, err)

	data, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "Review API research")
	assert.Contains(t, string(data), "Second item")
}

func TestTodoExporter_MarkerMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todos.md")

	startMarker := "<!-- hive:todos:start -->"
	endMarker := "<!-- hive:todos:end -->"

	// Create file with markers and existing content
	initial := "# My Todos\n\n" + startMarker + "\n- old item\n" + endMarker + "\n\n# Other Stuff\n"
	require.NoError(t, os.WriteFile(path, []byte(initial), 0o644))

	exp, err := NewTodoExporter(config.TodosExportConfig{
		Enabled: true,
		Path:    path,
		Markers: config.TodosExportMarkers{
			Start: startMarker,
			End:   endMarker,
		},
	}, zerolog.Nop())
	require.NoError(t, err)

	err = exp.Export([]todo.Todo{testTodo()})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "Review API research")
	assert.NotContains(t, content, "old item")
	assert.Contains(t, content, "# Other Stuff")
	assert.Contains(t, content, startMarker)
	assert.Contains(t, content, endMarker)
}

func TestTodoExporter_MarkerMode_MissingMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todos.md")

	require.NoError(t, os.WriteFile(path, []byte("no markers here"), 0o644))

	exp, err := NewTodoExporter(config.TodosExportConfig{
		Enabled: true,
		Path:    path,
		Markers: config.TodosExportMarkers{
			Start: "<!-- start -->",
			End:   "<!-- end -->",
		},
	}, zerolog.Nop())
	require.NoError(t, err)

	err = exp.Export([]todo.Todo{testTodo()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTodoExporter_CustomTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todos.md")
	tmplPath := filepath.Join(dir, "todo.tmpl")

	tmpl := `## {{ .Title }} ({{ .Category }})
`
	require.NoError(t, os.WriteFile(tmplPath, []byte(tmpl), 0o644))

	exp, err := NewTodoExporter(config.TodosExportConfig{
		Enabled:  true,
		Path:     path,
		Template: tmplPath,
	}, zerolog.Nop())
	require.NoError(t, err)

	err = exp.Export([]todo.Todo{testTodo()})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "## Review API research (review)")
}

func TestTodoExporter_MarkerMode_EndMarkerSubstring(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todos.md")

	startMarker := "<!-- hive:todos:start -->"
	endMarker := "<!-- hive:todos:end -->"

	// File has an unrelated section before our markers that contains similar
	// marker-like text. The end marker search must start after the matched
	// start marker to avoid false positives.
	initial := "<!-- hive:todos:end --><!-- not ours -->\n\n" +
		startMarker + "\n- old item\n" + endMarker + "\n\n# Other Stuff\n"
	require.NoError(t, os.WriteFile(path, []byte(initial), 0o644))

	exp, err := NewTodoExporter(config.TodosExportConfig{
		Enabled: true,
		Path:    path,
		Markers: config.TodosExportMarkers{
			Start: startMarker,
			End:   endMarker,
		},
	}, zerolog.Nop())
	require.NoError(t, err)

	err = exp.Export([]todo.Todo{testTodo()})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "Review API research")
	assert.NotContains(t, content, "old item")
	assert.Contains(t, content, "# Other Stuff")
}

func TestTodoExporter_NoRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "todos.md")

	exp, err := NewTodoExporter(config.TodosExportConfig{
		Enabled: true,
		Path:    path,
	}, zerolog.Nop())
	require.NoError(t, err)

	item := testTodo()
	item.Ref = ""
	err = exp.Export([]todo.Todo{item})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "Review API research")
	assert.NotContains(t, content, "`\n")
}
