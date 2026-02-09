package diff

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/stretchr/testify/assert"
)

func TestDiffViewerOpenInEditor(t *testing.T) {
	// Save original EDITOR env var and restore after test
	originalEditor := os.Getenv("EDITOR")
	defer func() {
		if originalEditor != "" {
			_ = os.Setenv("EDITOR", originalEditor)
		} else {
			_ = os.Unsetenv("EDITOR")
		}
	}()

	// Set a test editor
	_ = os.Setenv("EDITOR", "testEditor")

	file := &gitdiff.File{
		OldName: "old.go",
		NewName: "new.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    1,
				NewPosition: 1,
				NewLines:    1,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)
	loadFileSync(&m, file)

	// Test opening editor with 'e' key
	result, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'e'}))
	m = result

	// Should return a command (ExecProcess)
	assert.NotNil(t, cmd, "should return editor command")

	// Test with nil file - should not crash
	m = NewDiffViewer(nil)
	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'e'}))
	assert.Nil(t, cmd, "should return nil command when no file is set")

	// Test with deleted file (NewName is /dev/null) - should use OldName
	deletedFile := &gitdiff.File{
		OldName: "deleted.go",
		NewName: "/dev/null",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    1,
				NewPosition: 0,
				NewLines:    0,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpDelete, Line: "package main\n"},
				},
			},
		},
	}

	m = NewDiffViewer(deletedFile)
	loadFileSync(&m, deletedFile)
	_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'e'}))
	assert.NotNil(t, cmd, "should return editor command for deleted file with old name")
}

func TestDiffViewerEditorFinishedMsg(t *testing.T) {
	file := &gitdiff.File{
		NewName: "test.go",
		TextFragments: []*gitdiff.TextFragment{
			{
				OldPosition: 1,
				OldLines:    1,
				NewPosition: 1,
				NewLines:    1,
				Lines: []gitdiff.Line{
					{Op: gitdiff.OpContext, Line: "package main\n"},
				},
			},
		},
	}

	m := NewDiffViewer(file)
	loadFileSync(&m, file)

	// Test handling editorFinishedMsg
	result, cmd := m.Update(editorFinishedMsg{err: nil})
	m = result

	// Should handle the message without crashing
	assert.Nil(t, cmd, "should not return a command after editor finishes")

	// Test with error
	result, cmd = m.Update(editorFinishedMsg{err: assert.AnError})
	m = result
	assert.Nil(t, cmd, "should not return a command even with error")
}
