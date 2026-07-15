package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/sources"
)

const (
	defaultTimeout         = 15 * time.Second
	maxReportedStderrBytes = 4 << 10
	stderrTruncatedMarker  = "\n... [stderr truncated]"
)

var environmentReferencePattern = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)

// Source implements sources.Source by starting the configured command once for
// each initialize, search, or fetchDetail call.
type Source struct {
	id      string
	command []string
	env     map[string]string
	timeout time.Duration
	runner  ProcessRunner
	nextID  atomic.Int64
}

var _ sources.Source = (*Source)(nil)

// New constructs a subprocess source using ExecProcessRunner. A zero configured
// timeout uses a 15-second default.
func New(cfg config.ExternalSourceConfig) (*Source, error) {
	return NewWithRunner(cfg, ExecProcessRunner{})
}

// NewWithRunner constructs a subprocess source with an injectable process
// runner. It is intended for tests and callers that provide equivalent process
// isolation. A zero configured timeout uses a 15-second default.
func NewWithRunner(cfg config.ExternalSourceConfig, runner ProcessRunner) (*Source, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("external source name is required")
	}
	if len(cfg.Command) == 0 || strings.TrimSpace(cfg.Command[0]) == "" {
		return nil, fmt.Errorf("external source %q: command is required", cfg.Name)
	}
	if cfg.Timeout < 0 {
		return nil, fmt.Errorf("external source %q: timeout must be nonnegative", cfg.Name)
	}
	if runner == nil {
		return nil, fmt.Errorf("external source %q: process runner is required", cfg.Name)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &Source{
		id:      cfg.Name,
		command: append([]string(nil), cfg.Command...),
		env:     cloneEnvironmentOverrides(cfg.Env),
		timeout: timeout,
		runner:  runner,
	}, nil
}

// Name returns the configured source identifier.
func (s *Source) Name() string {
	return s.id
}

// Available reports whether command[0] can be resolved using exec.LookPath.
func (s *Source) Available(_ context.Context) bool {
	_, err := exec.LookPath(s.command[0])
	return err == nil
}

// Initialize requests the subprocess manifest. The configured source name is
// always enforced as the manifest ID; it is also the display-name fallback when
// the subprocess returns an empty or whitespace-only display name.
func (s *Source) Initialize(ctx context.Context) (sources.Manifest, error) {
	var manifest sources.Manifest
	if err := s.call(ctx, MethodInitialize, struct{}{}, &manifest); err != nil {
		return sources.Manifest{}, err
	}
	manifest.ID = s.id
	if strings.TrimSpace(manifest.DisplayName) == "" {
		manifest.DisplayName = s.id
	}
	return manifest, nil
}

// Search sends params to the subprocess search method and returns its result.
func (s *Source) Search(ctx context.Context, params sources.SearchParams) (sources.SearchResult, error) {
	wireParams := searchParams{
		Query:  params.Query,
		Scope:  params.Scope,
		Dir:    params.Dir,
		Cursor: params.Cursor,
	}
	var result sources.SearchResult
	if err := s.call(ctx, MethodSearch, wireParams, &result); err != nil {
		return sources.SearchResult{}, err
	}
	return result, nil
}

// FetchDetail sends params to the subprocess fetchDetail method. Callers use
// the initialize manifest capability to decide whether to invoke it; the method
// itself always performs the RPC request.
func (s *Source) FetchDetail(ctx context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	wireParams := fetchDetailParams{
		ID:    params.ID,
		Scope: params.Scope,
		URI:   params.URI,
		Dir:   params.Dir,
	}
	var detail sources.Detail
	if err := s.call(ctx, MethodFetchDetail, wireParams, &detail); err != nil {
		return sources.Detail{}, err
	}
	return detail, nil
}

func (s *Source) call(ctx context.Context, method string, params, result any) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("external source %q %s: encode params: %w", s.id, method, err)
	}

	id := s.nextID.Add(1)
	request, err := json.Marshal(Request{
		JSONRPC: Version,
		ID:      id,
		Method:  method,
		Params:  rawParams,
	})
	if err != nil {
		return fmt.Errorf("external source %q %s: encode request: %w", s.id, method, err)
	}
	request = append(request, '\n')

	stdout, stderr, runErr := s.runner.Run(
		ctx,
		append([]string(nil), s.command...),
		buildEnvironment(s.env),
		request,
	)
	if err := checkOutputLimits(stdout, stderr); err != nil {
		return fmt.Errorf("external source %q %s: %w", s.id, method, err)
	}
	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("external source %q %s: subprocess canceled: %w", s.id, method, ctxErr)
		}
		if message := reportedStderr(stderr); message != "" {
			return fmt.Errorf("external source %q %s: subprocess failed: %w: %s", s.id, method, runErr, message)
		}
		return fmt.Errorf("external source %q %s: subprocess failed: %w", s.id, method, runErr)
	}

	response, err := decodeResponse(stdout)
	if err != nil {
		return fmt.Errorf("external source %q %s: %w", s.id, method, err)
	}
	if response.JSONRPC != Version {
		return fmt.Errorf("external source %q %s: unexpected jsonrpc version %q", s.id, method, response.JSONRPC)
	}

	responseID, err := decodeResponseID(response.ID)
	if err != nil {
		return fmt.Errorf("external source %q %s: %w", s.id, method, err)
	}
	if responseID != id {
		return fmt.Errorf("external source %q %s: response id %d does not match request id %d", s.id, method, responseID, id)
	}

	hasResult := len(response.Result) != 0
	hasError := len(response.Error) != 0
	if hasResult == hasError {
		return fmt.Errorf("external source %q %s: response must contain exactly one of result or error", s.id, method)
	}
	if hasError {
		rpcErr, err := decodeRPCError(response.Error)
		if err != nil {
			return fmt.Errorf("external source %q %s: decode rpc error: %w", s.id, method, err)
		}
		return fmt.Errorf("external source %q %s: rpc error %d: %w", s.id, method, rpcErr.Code, rpcErr)
	}

	if bytes.Equal(bytes.TrimSpace(response.Result), []byte("null")) {
		return fmt.Errorf("external source %q %s: result must not be null", s.id, method)
	}
	if err := json.Unmarshal(response.Result, result); err != nil {
		return fmt.Errorf("external source %q %s: decode result: %w", s.id, method, err)
	}
	return nil
}

func decodeResponse(stdout []byte) (Response, error) {
	if len(stdout) == 0 {
		return Response{}, fmt.Errorf("empty stdout: expected one response line")
	}
	if stdout[len(stdout)-1] != '\n' {
		return Response{}, fmt.Errorf("response is not newline-terminated")
	}

	line := stdout[:len(stdout)-1]
	if bytes.ContainsRune(line, '\n') {
		return Response{}, fmt.Errorf("extra stdout lines")
	}
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Response{}, fmt.Errorf("empty stdout: expected one response line")
	}

	var response Response
	if err := json.Unmarshal(line, &response); err != nil {
		return Response{}, fmt.Errorf("decode response: %w", err)
	}
	return response, nil
}

func decodeRPCError(raw json.RawMessage) (*RPCError, error) {
	var wire struct {
		Code    *int            `json:"code"`
		Message *string         `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}
	if wire.Code == nil || wire.Message == nil {
		return nil, fmt.Errorf("error must be an object containing code and message")
	}
	return &RPCError{Code: *wire.Code, Message: *wire.Message, Data: wire.Data}, nil
}

func decodeResponseID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return 0, fmt.Errorf("response id is missing or null")
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, fmt.Errorf("invalid response id: %w", err)
	}
	return id, nil
}

func reportedStderr(stderr []byte) string {
	message := bytes.TrimSpace(stderr)
	if len(message) <= maxReportedStderrBytes {
		return string(message)
	}

	prefixBytes := maxReportedStderrBytes - len(stderrTruncatedMarker)
	return string(message[:prefixBytes]) + stderrTruncatedMarker
}

func checkOutputLimits(stdout, stderr []byte) error {
	if len(stdout) > maxOutputBytes {
		return fmt.Errorf("stdout exceeds %d-byte limit", maxOutputBytes)
	}
	if len(stderr) > maxOutputBytes {
		return fmt.Errorf("stderr exceeds %d-byte limit", maxOutputBytes)
	}
	return nil
}

func cloneEnvironmentOverrides(overrides map[string]string) map[string]string {
	if len(overrides) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(overrides))
	for name, value := range overrides {
		cloned[name] = value
	}
	return cloned
}

// buildEnvironment expands only ${VAR} references in configured values. Each
// reference reads the parent process environment; a missing variable expands
// to an empty string. No shell syntax is evaluated, and configured overrides
// do not affect expansion of other overrides.
func buildEnvironment(overrides map[string]string) []string {
	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		if found {
			environment[name] = value
		}
	}
	for name, value := range overrides {
		environment[name] = environmentReferencePattern.ReplaceAllStringFunc(value, func(reference string) string {
			return os.Getenv(reference[2 : len(reference)-1])
		})
	}

	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+environment[name])
	}
	return result
}
