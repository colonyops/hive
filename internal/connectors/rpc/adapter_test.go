package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/connectors"
)

// fakeRunnerCall records one invocation of fakeProcessRunner.Run.
type fakeRunnerCall struct {
	command []string
	request Request
}

// fakeProcessRunner is a canned ProcessRunner for adapter tests. respond is
// consulted per call; if nil, staticStdout/staticStderr/staticErr are used.
type fakeProcessRunner struct {
	calls []fakeRunnerCall

	respond func(call fakeRunnerCall) (stdout, stderr []byte, err error)

	staticStdout []byte
	staticStderr []byte
	staticErr    error

	// honorCtx makes Run block until ctx is done and return ctx.Err(),
	// simulating a hung subprocess for timeout/cancellation tests.
	honorCtx bool
}

func (f *fakeProcessRunner) Run(ctx context.Context, command []string, stdin []byte) ([]byte, []byte, error) {
	var req Request
	_ = json.Unmarshal(stdin, &req)
	call := fakeRunnerCall{command: command, request: req}
	f.calls = append(f.calls, call)

	if f.honorCtx {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}

	if f.respond != nil {
		return f.respond(call)
	}
	return f.staticStdout, f.staticStderr, f.staticErr
}

func responseLine(t *testing.T, resp Response) []byte {
	t.Helper()
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	return append(raw, '\n')
}

func TestSubprocessConnector_Initialize_HappyPath(t *testing.T) {
	runner := &fakeProcessRunner{
		respond: func(call fakeRunnerCall) ([]byte, []byte, error) {
			manifest := Manifest{
				ID:          "ref",
				DisplayName: "Reference",
				Picker: Picker{
					Layout: "list",
					Columns: []Column{
						{Key: "title", Label: "Title"},
					},
					Search:      Search{Mode: "remote", DebounceMS: 100},
					HidePreview: true,
				},
			}
			raw, err := json.Marshal(manifest)
			require.NoError(t, err)
			resp := Response{JSONRPC: Version, ID: call.request.ID, Result: raw}
			return responseLine(t, resp), nil, nil
		},
	}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
	require.NoError(t, err)

	manifest, err := conn.Initialize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ref", manifest.ID)
	assert.Equal(t, "Reference", manifest.DisplayName)
	assert.Equal(t, "list", string(manifest.Picker.Layout))
	assert.Equal(t, "remote", string(manifest.Picker.Search.Mode))
	assert.Equal(t, 100, manifest.Picker.Search.DebounceMS)
	assert.True(t, manifest.Picker.HidePreview, "hidePreview must survive the wire mapping")

	require.Len(t, runner.calls, 1)
	req := runner.calls[0].request
	assert.Equal(t, Version, req.JSONRPC)
	assert.Equal(t, MethodInitialize, req.Method)
	assert.Equal(t, int64(1), req.ID)
}

func TestSubprocessConnector_Search_HappyPath(t *testing.T) {
	var capturedParams SearchParams
	runner := &fakeProcessRunner{
		respond: func(call fakeRunnerCall) ([]byte, []byte, error) {
			require.NoError(t, json.Unmarshal(call.request.Params, &capturedParams))
			result := SearchResult{
				Items: []Item{
					{ID: "1", Title: "One", Fields: map[string]any{"n": float64(1)}},
					{ID: "2", Title: "Two"},
				},
				NextCursor: "cursor-2",
			}
			raw, err := json.Marshal(result)
			require.NoError(t, err)
			return responseLine(t, Response{JSONRPC: Version, ID: call.request.ID, Result: raw}), nil, nil
		},
	}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
	require.NoError(t, err)

	result, err := conn.Search(context.Background(), connectors.SearchParams{Query: "hello", Scope: "scope-a", Cursor: "cursor-1"})
	require.NoError(t, err)

	assert.Equal(t, "hello", capturedParams.Query)
	assert.Equal(t, "scope-a", capturedParams.Scope)
	assert.Equal(t, "cursor-1", capturedParams.Cursor)

	require.Len(t, result.Items, 2)
	assert.Equal(t, "1", result.Items[0].ID)
	assert.Equal(t, "One", result.Items[0].Title)
	assert.Equal(t, "cursor-2", result.NextCursor)

	require.Len(t, runner.calls, 1)
	assert.Equal(t, MethodSearch, runner.calls[0].request.Method)
	assert.Equal(t, int64(1), runner.calls[0].request.ID)
}

func TestSubprocessConnector_FetchDetail_HappyPath(t *testing.T) {
	runner := &fakeProcessRunner{
		respond: func(call fakeRunnerCall) ([]byte, []byte, error) {
			detail := Detail{Markdown: &MarkdownDetail{Content: "# hi"}}
			raw, err := json.Marshal(detail)
			require.NoError(t, err)
			return responseLine(t, Response{JSONRPC: Version, ID: call.request.ID, Result: raw}), nil, nil
		},
	}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
	require.NoError(t, err)

	detail, err := conn.FetchDetail(context.Background(), connectors.FetchDetailParams{ID: "item-1"})
	require.NoError(t, err)
	require.NotNil(t, detail.Markdown)
	assert.Equal(t, "# hi", detail.Markdown.Content)
}

func TestSubprocessConnector_IncrementingRequestIDs(t *testing.T) {
	runner := &fakeProcessRunner{
		respond: func(call fakeRunnerCall) ([]byte, []byte, error) {
			return responseLine(t, Response{JSONRPC: Version, ID: call.request.ID, Result: json.RawMessage(`{}`)}), nil, nil
		},
	}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
	require.NoError(t, err)

	_, err = conn.Initialize(context.Background())
	require.NoError(t, err)
	_, err = conn.Search(context.Background(), connectors.SearchParams{})
	require.NoError(t, err)

	require.Len(t, runner.calls, 2)
	assert.Equal(t, int64(1), runner.calls[0].request.ID)
	assert.Equal(t, int64(2), runner.calls[1].request.ID)
}

func TestSubprocessConnector_JSONRPCErrorResponse(t *testing.T) {
	runner := &fakeProcessRunner{
		respond: func(call fakeRunnerCall) ([]byte, []byte, error) {
			resp := Response{
				JSONRPC: Version,
				ID:      call.request.ID,
				Error:   &Error{Code: ErrCodeInvalidParams, Message: "bad scope"},
			}
			return responseLine(t, resp), nil, nil
		},
	}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
	require.NoError(t, err)

	_, err = conn.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad scope")
	assert.Contains(t, err.Error(), "-32602")
}

func TestSubprocessConnector_MalformedResponses(t *testing.T) {
	tests := []struct {
		name    string
		stdout  []byte
		wantErr string
	}{
		{
			name:    "invalid json line",
			stdout:  []byte("not json\n"),
			wantErr: "decode response line",
		},
		{
			name:    "wrong jsonrpc version",
			stdout:  []byte(`{"jsonrpc":"1.0","id":1,"result":{}}` + "\n"),
			wantErr: "unexpected jsonrpc version",
		},
		{
			name:    "id mismatch",
			stdout:  []byte(`{"jsonrpc":"2.0","id":999,"result":{}}` + "\n"),
			wantErr: "does not match request id",
		},
		{
			name:    "neither result nor error",
			stdout:  []byte(`{"jsonrpc":"2.0","id":1}` + "\n"),
			wantErr: "neither result nor error",
		},
		{
			name:    "empty stdout",
			stdout:  []byte(""),
			wantErr: "no response line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeProcessRunner{staticStdout: tt.stdout}
			conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
			require.NoError(t, err)

			_, err = conn.Initialize(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSubprocessConnector_Available(t *testing.T) {
	runner := &fakeProcessRunner{}

	t.Run("resolvable command", func(t *testing.T) {
		conn, err := NewSubprocessConnector("ref", []string{"sh"}, runner, time.Second)
		require.NoError(t, err)
		assert.True(t, conn.Available(context.Background()))
	})

	t.Run("unresolvable command", func(t *testing.T) {
		conn, err := NewSubprocessConnector("ref", []string{"hive-definitely-not-a-real-binary-xyz"}, runner, time.Second)
		require.NoError(t, err)
		assert.False(t, conn.Available(context.Background()))
	})
}

func TestSubprocessConnector_NonZeroExit(t *testing.T) {
	runner := &fakeProcessRunner{
		staticStderr: []byte("connector: boom"),
		staticErr:    errors.New("exit status 1"),
	}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
	require.NoError(t, err)

	_, err = conn.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1")
	assert.Contains(t, err.Error(), "connector: boom")
}

func TestSubprocessConnector_ContextTimeout(t *testing.T) {
	runner := &fakeProcessRunner{honorCtx: true}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, 20*time.Millisecond)
	require.NoError(t, err)

	_, err = conn.Initialize(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSubprocessConnector_ContextCancellation(t *testing.T) {
	runner := &fakeProcessRunner{honorCtx: true}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Minute)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err = conn.Initialize(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSubprocessConnector_LargeStdoutPayload(t *testing.T) {
	big := strings.Repeat("x", 200*1024) // 200KB, well past the 64KB scanner default.

	runner := &fakeProcessRunner{
		respond: func(call fakeRunnerCall) ([]byte, []byte, error) {
			detail := Detail{Markdown: &MarkdownDetail{Content: big}}
			raw, err := json.Marshal(detail)
			require.NoError(t, err)
			return responseLine(t, Response{JSONRPC: Version, ID: call.request.ID, Result: raw}), nil, nil
		},
	}

	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
	require.NoError(t, err)

	detail, err := conn.FetchDetail(context.Background(), connectors.FetchDetailParams{ID: "big"})
	require.NoError(t, err)
	require.NotNil(t, detail.Markdown)
	assert.Len(t, detail.Markdown.Content, len(big))
}

func TestNewSubprocessConnector_Validation(t *testing.T) {
	runner := &fakeProcessRunner{}

	_, err := NewSubprocessConnector("", []string{"cmd"}, runner, time.Second)
	require.Error(t, err)

	_, err = NewSubprocessConnector("id", nil, runner, time.Second)
	require.Error(t, err)

	_, err = NewSubprocessConnector("id", []string{"cmd"}, nil, time.Second)
	require.Error(t, err)
}

// TestSubprocessConnector_ErrorResponseWithNullID verifies that a
// spec-compliant error response with a null id (mandated by JSON-RPC 2.0
// when the server could not parse the request) surfaces the connector's
// error message instead of an id-mismatch error.
func TestSubprocessConnector_ErrorResponseWithNullID(t *testing.T) {
	stdout := []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error: bad request line"}}` + "\n")
	runner := &fakeProcessRunner{staticStdout: stdout}
	conn, err := NewSubprocessConnector("ref", []string{"ref-connector"}, runner, time.Second)
	require.NoError(t, err)

	_, err = conn.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse error: bad request line")
	assert.NotContains(t, err.Error(), "does not match request id")
}

// TestCappedBuffer verifies child output capture is truncated at
// maxCaptureBytes without erroring (which would kill the child mid-write).
func TestCappedBuffer(t *testing.T) {
	var b cappedBuffer

	n, err := b.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", b.buf.String())

	big := make([]byte, maxCaptureBytes)
	n, err = b.Write(big)
	require.NoError(t, err, "a full capture buffer must not surface a write error")
	assert.Equal(t, len(big), n, "Write must report full consumption to avoid breaking the child")
	assert.Equal(t, maxCaptureBytes, b.buf.Len(), "capture must be capped")

	n, err = b.Write([]byte("overflow"))
	require.NoError(t, err)
	assert.Equal(t, 8, n)
	assert.Equal(t, maxCaptureBytes, b.buf.Len())
}
