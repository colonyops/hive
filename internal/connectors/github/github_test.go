package github

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/connectors"
)

// fakeExecutor is a minimal executil.Executor test double that returns a
// caller-configured response per call, in call order, and records every
// invocation's args for assertion. Unlike executil.RecordingExecutor
// (keyed only by command name), this supports returning different outputs
// for a sequence of calls to the same command ("gh"), which Search and
// FetchDetail both need across separate test cases.
type fakeExecutor struct {
	calls     []fakeCall
	responses []fakeResponse
}

type fakeCall struct {
	cmd  string
	args []string
}

type fakeResponse struct {
	out []byte
	err error
}

func (f *fakeExecutor) Run(_ context.Context, cmd string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{cmd: cmd, args: args})
	idx := len(f.calls) - 1
	if idx < len(f.responses) {
		return f.responses[idx].out, f.responses[idx].err
	}
	return nil, nil
}

func (f *fakeExecutor) RunDir(ctx context.Context, _ string, cmd string, args ...string) ([]byte, error) {
	return f.Run(ctx, cmd, args...)
}

func (f *fakeExecutor) RunStream(context.Context, io.Writer, io.Writer, string, ...string) error {
	return fmt.Errorf("not implemented")
}

func (f *fakeExecutor) RunDirStream(context.Context, string, io.Writer, io.Writer, string, ...string) error {
	return fmt.Errorf("not implemented")
}

func TestSearchIssues(t *testing.T) {
	exec := &fakeExecutor{
		responses: []fakeResponse{
			{out: []byte(`[
				{"number":1,"title":"First issue","state":"OPEN","author":{"login":"alice"},"labels":[],"url":"https://github.com/o/r/issues/1"},
				{"number":2,"title":"Second issue","state":"CLOSED","author":{"login":"bob"},"labels":[],"url":"https://github.com/o/r/issues/2"}
			]`)},
		},
	}
	c := New(exec, nil)

	result, err := c.Search(context.Background(), connectors.SearchParams{Scope: "o/r", Query: "bug"})
	require.NoError(t, err)
	require.Len(t, exec.calls, 1)

	call := exec.calls[0]
	assert.Equal(t, "gh", call.cmd)
	assert.Equal(t, []string{
		"issue", "list",
		"--repo", "o/r",
		"--json", "number,title,state,author,labels,url",
		"--limit", "30",
		"--search", "bug",
	}, call.args)

	require.Len(t, result.Items, 2)
	assert.Equal(t, "1", result.Items[0].ID)
	assert.Equal(t, "First issue", result.Items[0].Title)
	assert.Equal(t, "#1 · OPEN", result.Items[0].Subtitle)
	assert.Equal(t, "https://github.com/o/r/issues/1", result.Items[0].URI)
	assert.Equal(t, map[string]any{
		"number": 1,
		"title":  "First issue",
		"state":  "OPEN",
		"url":    "https://github.com/o/r/issues/1",
		"author": "alice",
	}, result.Items[0].Fields)
}

func TestFetchDetail(t *testing.T) {
	exec := &fakeExecutor{
		responses: []fakeResponse{
			{out: []byte(`{"number":1,"title":"First issue","body":"issue body text","url":"https://github.com/o/r/issues/1","state":"OPEN"}`)},
		},
	}
	c := New(exec, nil)

	detail, err := c.FetchDetail(context.Background(), connectors.FetchDetailParams{ID: "1", Scope: "o/r"})
	require.NoError(t, err)
	require.NotNil(t, detail.Markdown)
	assert.Equal(t, "issue body text", detail.Markdown.Content)

	require.Len(t, exec.calls, 1)
	assert.Equal(t, []string{
		"issue", "view", "1",
		"--repo", "o/r",
		"--json", "number,title,body,url,state",
	}, exec.calls[0].args)
}

func TestAvailableChecksGH(t *testing.T) {
	c := New(&fakeExecutor{}, nil)

	t.Setenv("PATH", t.TempDir())
	assert.False(t, c.Available(context.Background()))
}

func TestSearchRequiresOwnerRepoScope(t *testing.T) {
	cases := []string{"", "no-slash", "too/many/slashes", "/name", "owner/"}

	for _, scope := range cases {
		t.Run(scope, func(t *testing.T) {
			exec := &fakeExecutor{}
			c := New(exec, nil)

			_, err := c.Search(context.Background(), connectors.SearchParams{Scope: scope})
			require.Error(t, err)
			assert.Empty(t, exec.calls, "no gh call should be made for an invalid scope")
		})
	}
}

func TestSearchGHNonZeroExit(t *testing.T) {
	exec := &fakeExecutor{
		responses: []fakeResponse{
			{out: []byte(""), err: fmt.Errorf("exec gh: exit status 1")},
		},
	}
	c := New(exec, nil)

	result, err := c.Search(context.Background(), connectors.SearchParams{Scope: "o/r"})
	require.Error(t, err)
	assert.Nil(t, result.Items)
}

func TestSearchEmptyResults(t *testing.T) {
	exec := &fakeExecutor{
		responses: []fakeResponse{
			{out: []byte(`[]`)},
		},
	}
	c := New(exec, nil)

	result, err := c.Search(context.Background(), connectors.SearchParams{Scope: "o/r"})
	require.NoError(t, err)
	assert.Nil(t, result.Items)
}

func TestSearchMalformedJSON(t *testing.T) {
	exec := &fakeExecutor{
		responses: []fakeResponse{
			{out: []byte("not json")},
		},
	}
	c := New(exec, nil)

	_, err := c.Search(context.Background(), connectors.SearchParams{Scope: "o/r"})
	require.Error(t, err)
}
