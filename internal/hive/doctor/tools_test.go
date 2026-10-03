package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolsCheck_BothPresent(t *testing.T) {
	orig := lookPathFunc
	t.Cleanup(func() { lookPathFunc = orig })

	lookPathFunc = func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}

	check := NewToolsCheck()
	result := check.Run(context.Background())

	assert.Equal(t, "Dependencies", result.Name)
	require.Len(t, result.Items, 2)

	assert.Equal(t, "git", result.Items[0].Label)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Equal(t, "/usr/bin/git", result.Items[0].Detail)

	assert.Equal(t, "tmux", result.Items[1].Label)
	assert.Equal(t, StatusPass, result.Items[1].Status)
	assert.Equal(t, "/usr/bin/tmux", result.Items[1].Detail)
}

func TestToolsCheck_GitMissing(t *testing.T) {
	orig := lookPathFunc
	t.Cleanup(func() { lookPathFunc = orig })

	lookPathFunc = func(file string) (string, error) {
		if file == "git" {
			return "", &exec.Error{Name: file, Err: fmt.Errorf("not found")}
		}
		return "/usr/bin/" + file, nil
	}

	check := NewToolsCheck()
	result := check.Run(context.Background())

	require.Len(t, result.Items, 2)
	assert.Equal(t, StatusFail, result.Items[0].Status)
	assert.Equal(t, "git", result.Items[0].Label)
	assert.Equal(t, StatusPass, result.Items[1].Status)
}

func TestToolsCheck_TmuxMissing(t *testing.T) {
	orig := lookPathFunc
	t.Cleanup(func() { lookPathFunc = orig })

	lookPathFunc = func(file string) (string, error) {
		if file == "tmux" {
			return "", &exec.Error{Name: file, Err: fmt.Errorf("not found")}
		}
		return "/usr/bin/" + file, nil
	}

	check := NewToolsCheck()
	result := check.Run(context.Background())

	require.Len(t, result.Items, 2)
	assert.Equal(t, StatusPass, result.Items[0].Status)
	assert.Equal(t, StatusWarn, result.Items[1].Status)
	assert.Contains(t, result.Items[1].Detail, "not found on PATH")
}
