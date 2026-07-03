package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/colonyops/hive/internal/sources"
)

// maxResponseLineBytes raises the response line scanner's buffer limit well
// past bufio.Scanner's 64KB default token cap so large detail payloads (e.g.
// long markdown bodies) decode without a "token too long" error.
const maxResponseLineBytes = 8 << 20 // 8MB

// maxCaptureBytes caps how much child stdout/stderr is buffered in memory,
// as a robustness bound against a runaway source spewing output within
// its timeout window. It is deliberately larger than maxResponseLineBytes
// so a valid maximum-size response line is never truncated.
const maxCaptureBytes = 16 << 20 // 16MB

// execWaitDelay bounds how long Run waits for the child's stdout/stderr
// pipes to close after the context is cancelled. Without it, a source
// that forks a background process inheriting stdout/stderr keeps the pipes
// open after the direct child is killed, and Run — and therefore the
// per-call timeout — would block forever (see exec.Cmd.WaitDelay).
const execWaitDelay = 5 * time.Second

// cappedBuffer is an io.Writer that appends to buf up to maxCaptureBytes
// and silently discards the rest, so a misbehaving child cannot grow
// memory without bound. Write never returns an error: the child must not
// be killed by a full capture buffer (SIGPIPE), only truncated.
type cappedBuffer struct {
	buf bytes.Buffer
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if remaining := maxCaptureBytes - b.buf.Len(); remaining > 0 {
		b.buf.Write(p[:min(len(p), remaining)])
	}
	return len(p), nil
}

// ProcessRunner starts a source process for a single request/response
// exchange: it writes stdin, waits for the process to exit, and returns the
// captured stdout/stderr. The real implementation wraps exec.CommandContext;
// tests inject a fake that returns canned output.
type ProcessRunner interface {
	Run(ctx context.Context, command []string, stdin []byte) (stdout, stderr []byte, err error)
}

// ExecProcessRunner is the real ProcessRunner implementation, spawning the
// source command via exec.CommandContext.
type ExecProcessRunner struct{}

// Run executes command[0] with command[1:] as arguments, writes stdin to the
// process's stdin, and returns its captured stdout/stderr. A non-zero exit
// is returned as an error wrapping the underlying *exec.ExitError.
func (ExecProcessRunner) Run(ctx context.Context, command []string, stdin []byte) (stdout, stderr []byte, err error) {
	if len(command) == 0 {
		return nil, nil, fmt.Errorf("empty command")
	}

	c := exec.CommandContext(ctx, command[0], command[1:]...)
	c.Stdin = bytes.NewReader(stdin)
	c.WaitDelay = execWaitDelay

	var stdoutBuf, stderrBuf cappedBuffer
	c.Stdout = &stdoutBuf
	c.Stderr = &stderrBuf

	runErr := c.Run()
	if runErr != nil {
		return stdoutBuf.buf.Bytes(), stderrBuf.buf.Bytes(), fmt.Errorf("exec %s: %w", command[0], runErr)
	}
	return stdoutBuf.buf.Bytes(), stderrBuf.buf.Bytes(), nil
}

// SubprocessSource implements sources.Source by spawning an
// external command once per method call and exchanging a single JSON-RPC
// 2.0 request/response pair over the process's stdin/stdout. The
// source's own diagnostic output must go to stderr; stdout carries only
// the response line.
type SubprocessSource struct {
	id      string
	command []string
	runner  ProcessRunner
	timeout time.Duration
	nextID  atomic.Int64
}

// NewSubprocessSource constructs a SubprocessSource for the given
// source id and launch command. timeout bounds each individual RPC call;
// it is combined with the caller's context so whichever fires first wins.
func NewSubprocessSource(id string, command []string, runner ProcessRunner, timeout time.Duration) (*SubprocessSource, error) {
	if id == "" {
		return nil, fmt.Errorf("source id is required")
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("source %q: command is required", id)
	}
	if runner == nil {
		return nil, fmt.Errorf("source %q: runner is required", id)
	}

	return &SubprocessSource{
		id:      id,
		command: command,
		runner:  runner,
		timeout: timeout,
	}, nil
}

// Name returns the source's configured id.
func (c *SubprocessSource) Name() string {
	return c.id
}

// Available reports whether the source's command binary can be resolved
// on PATH. It does not spawn the process.
func (c *SubprocessSource) Available(_ context.Context) bool {
	_, err := exec.LookPath(c.command[0])
	return err == nil
}

// Initialize calls the source's initialize method and returns its
// picker manifest.
func (c *SubprocessSource) Initialize(ctx context.Context) (sources.Manifest, error) {
	var wire Manifest
	if err := c.call(ctx, MethodInitialize, InitializeParams{}, &wire); err != nil {
		return sources.Manifest{}, err
	}
	return manifestFromWire(wire), nil
}

// Search calls the source's search method and returns matching items.
func (c *SubprocessSource) Search(ctx context.Context, params sources.SearchParams) (sources.SearchResult, error) {
	wireParams := SearchParams{
		Query:  params.Query,
		Scope:  params.Scope,
		Cursor: params.Cursor,
	}

	var wire SearchResult
	if err := c.call(ctx, MethodSearch, wireParams, &wire); err != nil {
		return sources.SearchResult{}, err
	}

	items := make([]sources.Item, 0, len(wire.Items))
	for _, item := range wire.Items {
		items = append(items, itemFromWire(item))
	}

	return sources.SearchResult{
		Items:      items,
		NextCursor: wire.NextCursor,
	}, nil
}

// FetchDetail calls the source's fetchDetail method and returns the
// detail view for a single item.
func (c *SubprocessSource) FetchDetail(ctx context.Context, params sources.FetchDetailParams) (sources.Detail, error) {
	wireParams := FetchDetailParams{
		ID:    params.ID,
		Scope: params.Scope,
		URI:   params.URI,
	}

	var wire Detail
	if err := c.call(ctx, MethodFetchDetail, wireParams, &wire); err != nil {
		return sources.Detail{}, err
	}
	return detailFromWire(&wire), nil
}

// call runs one request/response exchange for method: it encodes params
// into a Request with an incrementing id, spawns the source process via
// c.runner, decodes exactly one Response line from stdout, validates the
// jsonrpc version and id match, maps a JSON-RPC error to a Go error, and
// unmarshals the result into out.
func (c *SubprocessSource) call(ctx context.Context, method string, params any, out any) error {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	rawParams, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("source %q: encode %s params: %w", c.id, method, err)
	}

	id := c.nextID.Add(1)
	req := Request{
		JSONRPC: Version,
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("source %q: encode %s request: %w", c.id, method, err)
	}
	reqBytes = append(reqBytes, '\n')

	stdout, stderr, err := c.runner.Run(ctx, c.command, reqBytes)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("source %q: %s: %w", c.id, method, ctxErr)
		}
		msg := strings.TrimSpace(string(stderr))
		if msg != "" {
			return fmt.Errorf("source %q: %s: %w: %s", c.id, method, err, msg)
		}
		return fmt.Errorf("source %q: %s: %w", c.id, method, err)
	}

	resp, err := decodeResponseLine(stdout)
	if err != nil {
		return fmt.Errorf("source %q: %s: %w", c.id, method, err)
	}

	if resp.JSONRPC != Version {
		return fmt.Errorf("source %q: %s: unexpected jsonrpc version %q", c.id, method, resp.JSONRPC)
	}

	// Check the error before the id: JSON-RPC 2.0 mandates a null id on
	// error responses the server could not parse, and surfacing the
	// source's actual error message beats reporting an id mismatch.
	if resp.Error != nil {
		return fmt.Errorf("source %q: %s: rpc error %d: %s", c.id, method, resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != id {
		return fmt.Errorf("source %q: %s: response id %d does not match request id %d", c.id, method, resp.ID, id)
	}
	if resp.Result == nil {
		return fmt.Errorf("source %q: %s: response has neither result nor error", c.id, method)
	}

	if out != nil {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("source %q: %s: decode result: %w", c.id, method, err)
		}
	}

	return nil
}

// decodeResponseLine reads exactly one JSON-RPC response line from stdout.
// stdout must contain only the response line (source diagnostics belong
// on stderr); an empty stdout or a non-JSON line is an error.
func decodeResponseLine(stdout []byte) (Response, error) {
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(make([]byte, 0, 64*1024), maxResponseLineBytes)

	var resp Response
	seenResponse := false
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if seenResponse {
			return Response{}, fmt.Errorf("multiple response lines on stdout")
		}

		if err := json.Unmarshal(line, &resp); err != nil {
			return Response{}, fmt.Errorf("decode response line: %w", err)
		}
		seenResponse = true
	}
	if err := scanner.Err(); err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	if !seenResponse {
		return Response{}, fmt.Errorf("no response line on stdout")
	}

	return resp, nil
}

// manifestFromWire converts the wire Manifest shape into the domain type.
func manifestFromWire(wire Manifest) sources.Manifest {
	columns := make([]sources.Column, 0, len(wire.Picker.Columns))
	for _, col := range wire.Picker.Columns {
		columns = append(columns, sources.Column{
			Key:   col.Key,
			Label: col.Label,
			Width: col.Width,
			Flex:  col.Flex,
		})
	}

	return sources.Manifest{
		ID:          wire.ID,
		DisplayName: wire.DisplayName,
		Capabilities: sources.Capabilities{
			FetchDetail: wire.Capabilities.FetchDetail,
		},
		Picker: sources.PickerManifest{
			Layout:      sources.LayoutMode(wire.Picker.Layout),
			Columns:     columns,
			HidePreview: wire.Picker.HidePreview,
			Search: sources.SearchManifest{
				Mode:       sources.SearchMode(wire.Picker.Search.Mode),
				DebounceMS: wire.Picker.Search.DebounceMS,
			},
		},
	}
}

// itemFromWire converts the wire Item shape into the domain type.
func itemFromWire(wire Item) sources.Item {
	item := sources.Item{
		ID:       wire.ID,
		Title:    wire.Title,
		Subtitle: wire.Subtitle,
		URI:      wire.URI,
		Fields:   wire.Fields,
	}
	if wire.Detail != nil {
		item.Detail = detailFromWire(wire.Detail)
	}
	return item
}

// detailFromWire converts the wire Detail shape into the domain type. A nil
// or empty wire Detail maps to a zero-value Detail (DetailKindNone).
func detailFromWire(wire *Detail) sources.Detail {
	if wire == nil {
		return sources.Detail{}
	}

	var detail sources.Detail
	if wire.Markdown != nil {
		detail.Markdown = &sources.MarkdownDetail{Content: wire.Markdown.Content}
	}
	if wire.KV != nil {
		sections := make([]sources.KVSection, 0, len(wire.KV.Sections))
		for _, section := range wire.KV.Sections {
			pairs := make([]sources.KVPair, 0, len(section.Pairs))
			for _, pair := range section.Pairs {
				pairs = append(pairs, sources.KVPair{Key: pair.Key, Value: pair.Value})
			}
			sections = append(sections, sources.KVSection{Heading: section.Heading, Pairs: pairs})
		}
		detail.KV = &sources.KVDetail{Sections: sections}
	}
	return detail
}
