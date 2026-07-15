package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runnerCall struct {
	argv    []string
	env     []string
	stdin   []byte
	request Request
}

type fakeRunner struct {
	mu    sync.Mutex
	calls []runnerCall
	run   func(context.Context, runnerCall) ([]byte, []byte, error)
}

func (f *fakeRunner) Run(ctx context.Context, argv, env []string, stdin []byte) ([]byte, []byte, error) {
	var request Request
	_ = json.Unmarshal(stdin, &request)
	call := runnerCall{
		argv:    append([]string(nil), argv...),
		env:     append([]string(nil), env...),
		stdin:   append([]byte(nil), stdin...),
		request: request,
	}
	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	if f.run == nil {
		return nil, nil, errors.New("unexpected process call")
	}
	return f.run(ctx, call)
}

func (f *fakeRunner) Calls() []runnerCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := make([]runnerCall, len(f.calls))
	copy(calls, f.calls)
	return calls
}

func testSource(t *testing.T, runner ProcessRunner, mutate ...func(*config.ExternalSourceConfig)) *Source {
	t.Helper()
	cfg := config.ExternalSourceConfig{
		Name:    "alerts",
		Command: []string{"alert-source", "--json-rpc"},
		Timeout: time.Second,
	}
	for _, fn := range mutate {
		fn(&cfg)
	}
	source, err := NewWithRunner(cfg, runner)
	require.NoError(t, err)
	return source
}

func resultResponse(t *testing.T, id int64, result string) []byte {
	t.Helper()
	var compact bytes.Buffer
	require.NoError(t, json.Compact(&compact, []byte(result)))
	return []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`+"\n", id, compact.String()))
}

func envMap(entries []string) map[string]string {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			result[name] = value
		}
	}
	return result
}

func TestSourceMethodsSendExactRequestsAndMapResults(t *testing.T) {
	runner := &fakeRunner{}
	runner.run = func(_ context.Context, call runnerCall) ([]byte, []byte, error) {
		switch call.request.Method {
		case MethodInitialize:
			return resultResponse(t, call.request.ID, `{
				"id":"remote-id",
				"displayName":"Remote Alerts",
				"capabilities":{"fetchDetail":true},
				"picker":{"search":{"debounceMS":125}}
			}`), nil, nil
		case MethodSearch:
			return resultResponse(t, call.request.ID, `{
				"items":[{"id":"a-1","title":"CPU high","subtitle":"api","uri":"alert://a-1","fields":{"severity":"critical"}}],
				"nextCursor":"next-page"
			}`), nil, nil
		case MethodFetchDetail:
			return resultResponse(t, call.request.ID, `{"markdown":{"content":"# CPU high"}}`), nil, nil
		default:
			return nil, nil, fmt.Errorf("unexpected method %q", call.request.Method)
		}
	}
	source := testSource(t, runner)

	manifest, err := source.Initialize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "alerts", manifest.ID, "configured identity must override the remote id")
	assert.Equal(t, "Remote Alerts", manifest.DisplayName)
	assert.True(t, manifest.Capabilities.FetchDetail)
	assert.Equal(t, 125, manifest.Picker.Search.DebounceMS)

	searchResult, err := source.Search(context.Background(), sources.SearchParams{
		Query: "state:firing", Scope: "prod", Dir: "/repo", Cursor: "page-1",
	})
	require.NoError(t, err)
	require.Len(t, searchResult.Items, 1)
	assert.Equal(t, "a-1", searchResult.Items[0].ID)
	assert.Equal(t, "CPU high", searchResult.Items[0].Title)
	assert.Equal(t, "critical", searchResult.Items[0].Fields["severity"])
	assert.Equal(t, "next-page", searchResult.NextCursor)

	detail, err := source.FetchDetail(context.Background(), sources.FetchDetailParams{
		ID: "a-1", Scope: "prod", URI: "alert://a-1", Dir: "/repo",
	})
	require.NoError(t, err)
	require.NotNil(t, detail.Markdown)
	assert.Equal(t, "# CPU high", detail.Markdown.Content)

	calls := runner.Calls()
	require.Len(t, calls, 3, "each method call must start one process")
	assert.Equal(t, []string{"alert-source", "--json-rpc"}, calls[0].argv)
	assertRequestJSON(t, calls[0].stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	assertRequestJSON(t, calls[1].stdin, `{"jsonrpc":"2.0","id":2,"method":"search","params":{"query":"state:firing","scope":"prod","dir":"/repo","cursor":"page-1"}}`)
	assertRequestJSON(t, calls[2].stdin, `{"jsonrpc":"2.0","id":3,"method":"fetchDetail","params":{"id":"a-1","scope":"prod","uri":"alert://a-1","dir":"/repo"}}`)
}

func assertRequestJSON(t *testing.T, request []byte, expected string) {
	t.Helper()
	require.NotEmpty(t, request)
	assert.Equal(t, 1, bytes.Count(request, []byte{'\n'}), "request must use exactly one newline frame")
	assert.Equal(t, byte('\n'), request[len(request)-1], "request must be newline-terminated")
	assert.JSONEq(t, expected, string(request))
}

func TestInitializeEnforcesConfiguredManifestFallbacks(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, call runnerCall) ([]byte, []byte, error) {
		return resultResponse(t, call.request.ID, `{"id":"other","displayName":"  "}`), nil, nil
	}}
	source := testSource(t, runner)

	manifest, err := source.Initialize(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "alerts", manifest.ID)
	assert.Equal(t, "alerts", manifest.DisplayName)
}

func TestFetchDetailAlwaysSendsRPC(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, call runnerCall) ([]byte, []byte, error) {
		return resultResponse(t, call.request.ID, `{}`), nil, nil
	}}
	source := testSource(t, runner)

	_, err := source.FetchDetail(context.Background(), sources.FetchDetailParams{ID: "a-1"})
	require.NoError(t, err)
	require.Len(t, runner.Calls(), 1)
	assert.Equal(t, MethodFetchDetail, runner.Calls()[0].request.Method)
}

func TestSourceRequestIDsAreMonotonicAndConcurrencySafe(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, call runnerCall) ([]byte, []byte, error) {
		return resultResponse(t, call.request.ID, `{"items":[]}`), nil, nil
	}}
	source := testSource(t, runner)

	const calls = 25
	var wg sync.WaitGroup
	for range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := source.Search(context.Background(), sources.SearchParams{})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	seen := make(map[int64]bool, calls)
	for _, call := range runner.Calls() {
		seen[call.request.ID] = true
	}
	for id := int64(1); id <= calls; id++ {
		assert.True(t, seen[id], "missing request id %d", id)
	}
}

func TestSourceProtocolFailures(t *testing.T) {
	tests := []struct {
		name    string
		stdout  string
		wantErr string
	}{
		{name: "empty output", wantErr: "empty stdout"},
		{name: "malformed json", stdout: "not-json\n", wantErr: "decode response"},
		{name: "missing newline framing", stdout: `{"jsonrpc":"2.0","id":1,"result":{}}`, wantErr: "newline-terminated"},
		{name: "extra stdout line", stdout: "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\nnoise\n", wantErr: "extra stdout lines"},
		{name: "wrong version", stdout: `{"jsonrpc":"1.0","id":1,"result":{}}` + "\n", wantErr: "unexpected jsonrpc version"},
		{name: "missing id", stdout: `{"jsonrpc":"2.0","result":{}}` + "\n", wantErr: "id is missing or null"},
		{name: "null id", stdout: `{"jsonrpc":"2.0","id":null,"result":{}}` + "\n", wantErr: "id is missing or null"},
		{name: "noninteger id", stdout: `{"jsonrpc":"2.0","id":"1","result":{}}` + "\n", wantErr: "invalid response id"},
		{name: "mismatched id", stdout: `{"jsonrpc":"2.0","id":9,"result":{}}` + "\n", wantErr: "does not match request id"},
		{name: "neither result nor error", stdout: `{"jsonrpc":"2.0","id":1}` + "\n", wantErr: "exactly one"},
		{name: "both result and error", stdout: `{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":1,"message":"bad"}}` + "\n", wantErr: "exactly one"},
		{name: "null error", stdout: `{"jsonrpc":"2.0","id":1,"error":null}` + "\n", wantErr: "error must be an object"},
		{name: "invalid error", stdout: `{"jsonrpc":"2.0","id":1,"error":"bad"}` + "\n", wantErr: "decode rpc error"},
		{name: "error missing message", stdout: `{"jsonrpc":"2.0","id":1,"error":{"code":1}}` + "\n", wantErr: "containing code and message"},
		{name: "null result", stdout: `{"jsonrpc":"2.0","id":1,"result":null}` + "\n", wantErr: "result must not be null"},
		{name: "invalid result", stdout: `{"jsonrpc":"2.0","id":1,"result":"bad"}` + "\n", wantErr: "decode result"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{run: func(_ context.Context, _ runnerCall) ([]byte, []byte, error) {
				return []byte(tt.stdout), nil, nil
			}}
			source := testSource(t, runner)

			_, err := source.Initialize(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSourceRPCErrorIncludesCodeAndMessage(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, call runnerCall) ([]byte, []byte, error) {
		return []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"error":{"code":-32602,"message":"invalid scope"}}`+"\n", call.request.ID)), nil, nil
	}}
	source := testSource(t, runner)

	_, err := source.Search(context.Background(), sources.SearchParams{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rpc error -32602: invalid scope")
	var rpcErr *RPCError
	require.ErrorAs(t, err, &rpcErr)
	assert.Equal(t, -32602, rpcErr.Code)
	assert.Equal(t, "invalid scope", rpcErr.Message)
}

func TestSourceValidatesIDBeforeRPCError(t *testing.T) {
	runner := &fakeRunner{run: func(_ context.Context, _ runnerCall) ([]byte, []byte, error) {
		return []byte(`{"jsonrpc":"2.0","id":99,"error":{"code":-32603,"message":"boom"}}` + "\n"), nil, nil
	}}
	source := testSource(t, runner)

	_, err := source.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match request id")
}

func TestSourcePropagatesProcessFailureAndStderr(t *testing.T) {
	secret := "configured-secret-must-not-appear"
	runner := &fakeRunner{run: func(_ context.Context, _ runnerCall) ([]byte, []byte, error) {
		return nil, []byte("plugin diagnostic"), errors.New("exit status 2")
	}}
	source := testSource(t, runner, func(cfg *config.ExternalSourceConfig) {
		cfg.Env = map[string]string{"TOKEN": secret}
	})

	_, err := source.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 2")
	assert.Contains(t, err.Error(), "plugin diagnostic")
	assert.NotContains(t, err.Error(), secret, "configured environment values must not be reported")
}

func TestSourceBoundsReportedProcessFailureStderr(t *testing.T) {
	const errorPrefix = `external source "alerts" initialize: subprocess failed: exit status 2: `

	t.Run("short stderr is unchanged", func(t *testing.T) {
		const stderr = "plugin diagnostic"
		runner := &fakeRunner{run: func(_ context.Context, _ runnerCall) ([]byte, []byte, error) {
			return nil, []byte(stderr), errors.New("exit status 2")
		}}
		source := testSource(t, runner)

		_, err := source.Initialize(context.Background())
		require.EqualError(t, err, errorPrefix+stderr)
	})

	t.Run("long stderr is truncated within report limit", func(t *testing.T) {
		visiblePrefixBytes := maxReportedStderrBytes - len(stderrTruncatedMarker)
		stderr := strings.Repeat("x", maxReportedStderrBytes) + "omitted-tail"
		runner := &fakeRunner{run: func(_ context.Context, _ runnerCall) ([]byte, []byte, error) {
			return nil, []byte(stderr), errors.New("exit status 2")
		}}
		source := testSource(t, runner)

		_, err := source.Initialize(context.Background())
		require.Error(t, err)
		expectedStderr := strings.Repeat("x", visiblePrefixBytes) + stderrTruncatedMarker
		assert.Equal(t, errorPrefix+expectedStderr, err.Error())
		assert.Len(t, err.Error(), len(errorPrefix)+maxReportedStderrBytes)
		assert.NotContains(t, err.Error(), "omitted-tail")
	})
}

func TestSourceSupportsResponsesLargerThan64KiB(t *testing.T) {
	content := strings.Repeat("x", 128<<10)
	runner := &fakeRunner{run: func(_ context.Context, call runnerCall) ([]byte, []byte, error) {
		result, err := json.Marshal(map[string]any{"markdown": map[string]string{"content": content}})
		require.NoError(t, err)
		return resultResponse(t, call.request.ID, string(result)), nil, nil
	}}
	source := testSource(t, runner)

	detail, err := source.FetchDetail(context.Background(), sources.FetchDetailParams{ID: "large"})
	require.NoError(t, err)
	require.NotNil(t, detail.Markdown)
	assert.Len(t, detail.Markdown.Content, len(content))
}

func TestSourceRejectsOversizedOutputStreams(t *testing.T) {
	tests := []struct {
		name    string
		stdout  []byte
		stderr  []byte
		wantErr string
	}{
		{name: "stdout", stdout: make([]byte, maxOutputBytes+1), wantErr: "stdout exceeds"},
		{name: "stderr", stderr: make([]byte, maxOutputBytes+1), wantErr: "stderr exceeds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{run: func(_ context.Context, call runnerCall) ([]byte, []byte, error) {
				stdout := tt.stdout
				if stdout == nil {
					stdout = resultResponse(t, call.request.ID, `{}`)
				}
				return stdout, tt.stderr, nil
			}}
			source := testSource(t, runner)

			_, err := source.Initialize(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSourceEnvironmentExpansion(t *testing.T) {
	t.Setenv("RPC_PARENT_VALUE", "parent")
	t.Setenv("RPC_PARENT_ONLY", "retained")
	t.Setenv("RPC_SHADOWED", "parent-shadow")
	missingName := "RPC_DEFINITELY_MISSING_7E874E4A"
	_ = os.Unsetenv(missingName)

	runner := &fakeRunner{run: func(_ context.Context, call runnerCall) ([]byte, []byte, error) {
		env := envMap(call.env)
		assert.Equal(t, "retained", env["RPC_PARENT_ONLY"], "parent environment must be retained")
		assert.Equal(t, "pre-parent-post", env["EXPANDED"])
		assert.Equal(t, "before--after", env["MISSING"], "missing variables expand to an empty string")
		assert.Equal(t, "parent-shadow", env["FROM_PARENT"], "expansion must read the parent, not configured overrides")
		assert.Equal(t, `$RPC_PARENT_VALUE $(echo unsafe) ${INVALID-NAME}`, env["LITERAL"], "non-${VAR} syntax must remain literal")
		return resultResponse(t, call.request.ID, `{}`), nil, nil
	}}
	source := testSource(t, runner, func(cfg *config.ExternalSourceConfig) {
		cfg.Env = map[string]string{
			"EXPANDED":     "pre-${RPC_PARENT_VALUE}-post",
			"MISSING":      "before-${" + missingName + "}-after",
			"RPC_SHADOWED": "configured-shadow",
			"FROM_PARENT":  "${RPC_SHADOWED}",
			"LITERAL":      `$RPC_PARENT_VALUE $(echo unsafe) ${INVALID-NAME}`,
		}
	})

	_, err := source.Initialize(context.Background())
	require.NoError(t, err)
}

func TestSourceTimeoutsAndCancellation(t *testing.T) {
	t.Run("zero config uses default timeout", func(t *testing.T) {
		runner := &fakeRunner{run: func(ctx context.Context, call runnerCall) ([]byte, []byte, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			remaining := time.Until(deadline)
			assert.Greater(t, remaining, 14*time.Second)
			assert.LessOrEqual(t, remaining, defaultTimeout)
			return resultResponse(t, call.request.ID, `{}`), nil, nil
		}}
		source := testSource(t, runner, func(cfg *config.ExternalSourceConfig) { cfg.Timeout = 0 })
		_, err := source.Initialize(context.Background())
		require.NoError(t, err)
	})

	t.Run("configured timeout is used", func(t *testing.T) {
		const configured = 3 * time.Second
		runner := &fakeRunner{run: func(ctx context.Context, call runnerCall) ([]byte, []byte, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			assert.LessOrEqual(t, time.Until(deadline), configured)
			return resultResponse(t, call.request.ID, `{}`), nil, nil
		}}
		source := testSource(t, runner, func(cfg *config.ExternalSourceConfig) { cfg.Timeout = configured })
		_, err := source.Initialize(context.Background())
		require.NoError(t, err)
	})

	t.Run("configured timeout expiration is preserved", func(t *testing.T) {
		runner := &fakeRunner{run: func(ctx context.Context, _ runnerCall) ([]byte, []byte, error) {
			<-ctx.Done()
			return nil, nil, ctx.Err()
		}}
		source := testSource(t, runner, func(cfg *config.ExternalSourceConfig) {
			cfg.Timeout = 20 * time.Millisecond
		})

		_, err := source.Initialize(context.Background())
		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Contains(t, err.Error(), "subprocess canceled")
	})

	t.Run("caller cancellation is preserved", func(t *testing.T) {
		started := make(chan struct{})
		runner := &fakeRunner{run: func(ctx context.Context, _ runnerCall) ([]byte, []byte, error) {
			close(started)
			<-ctx.Done()
			return nil, nil, ctx.Err()
		}}
		source := testSource(t, runner)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := source.Initialize(ctx)
			done <- err
		}()
		<-started
		cancel()

		err := <-done
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		assert.Contains(t, err.Error(), "subprocess canceled")
	})
}

func TestSourceAvailableUsesLookPath(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	runner := &fakeRunner{}

	available := testSource(t, runner, func(cfg *config.ExternalSourceConfig) {
		cfg.Command = []string{executable}
	})
	assert.True(t, available.Available(context.Background()))

	unavailable := testSource(t, runner, func(cfg *config.ExternalSourceConfig) {
		cfg.Command = []string{"hive-no-such-source-command-6f6e36aa"}
	})
	assert.False(t, unavailable.Available(context.Background()))
}

func TestNewUsesConfiguredName(t *testing.T) {
	source, err := New(config.ExternalSourceConfig{
		Name:    "alerts",
		Command: []string{"alert-source"},
	})
	require.NoError(t, err)
	assert.Equal(t, "alerts", source.Name())
}

func TestNewValidation(t *testing.T) {
	runner := &fakeRunner{}
	tests := []struct {
		name   string
		cfg    config.ExternalSourceConfig
		runner ProcessRunner
	}{
		{name: "missing name", cfg: config.ExternalSourceConfig{Command: []string{"cmd"}}, runner: runner},
		{name: "missing command", cfg: config.ExternalSourceConfig{Name: "id"}, runner: runner},
		{name: "negative timeout", cfg: config.ExternalSourceConfig{Name: "id", Command: []string{"cmd"}, Timeout: -time.Second}, runner: runner},
		{name: "missing runner", cfg: config.ExternalSourceConfig{Name: "id", Command: []string{"cmd"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWithRunner(tt.cfg, tt.runner)
			require.Error(t, err)
		})
	}
}

func TestLimitedBufferRetainsOverflowSignalWithinBound(t *testing.T) {
	var buffer limitedBuffer
	input := make([]byte, maxOutputBytes+1024)

	written, err := buffer.Write(input)
	require.NoError(t, err)
	assert.Equal(t, len(input), written, "the child must not receive a short write")
	assert.Len(t, buffer.Bytes(), maxOutputBytes+1, "one overflow byte signals that the limit was exceeded")
}

func TestExecProcessRunnerUsesDirectArgvAndSeparateStreams(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	runner := ExecProcessRunner{}
	literalArg := `literal; $(echo not-run)`
	env := buildEnvironment(map[string]string{
		"GO_WANT_RPC_HELPER_PROCESS": "1",
		"RPC_HELPER_VALUE":           "environment-value",
	})

	stdout, stderr, err := runner.Run(
		context.Background(),
		[]string{executable, "-test.run=TestRPCExecHelperProcess", "--", literalArg},
		env,
		[]byte("request-body"),
	)
	require.NoError(t, err)
	assert.Equal(t, literalArg+"|environment-value|request-body", string(stdout))
	assert.Equal(t, "helper diagnostic", string(stderr))
}

func TestExecProcessRunnerCancellationKillsProcess(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	runner := ExecProcessRunner{}
	env := buildEnvironment(map[string]string{
		"GO_WANT_RPC_HELPER_PROCESS": "1",
		"RPC_HELPER_WAIT":            "1",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, _, err = runner.Run(ctx, []string{executable, "-test.run=TestRPCExecHelperProcess"}, env, nil)
	require.Error(t, err)
	assert.Less(t, time.Since(started), 2*time.Second)
	assert.Error(t, ctx.Err())
}

func TestRPCExecHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_RPC_HELPER_PROCESS") != "1" {
		return
	}
	if os.Getenv("RPC_HELPER_WAIT") == "1" {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals)
		<-signals
		os.Exit(0)
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	arg := ""
	if len(os.Args) > 0 {
		arg = os.Args[len(os.Args)-1]
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s|%s|%s", arg, os.Getenv("RPC_HELPER_VALUE"), input)
	_, _ = fmt.Fprint(os.Stderr, "helper diagnostic")
	os.Exit(0)
}
