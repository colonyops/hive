package commands

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/platform/desktopendpoint"
)

const (
	desktopCallTimeout = 10 * time.Second

	callerFlag      = "caller"
	desktopURLFlag  = "desktop-url"
	desktopTokenEnv = "HIVE_DESKTOP_TOKEN"
)

// desktopNotRunningError is a type, not errors.New, because the message starts
// with the product name and staticcheck rejects a capitalized error string.
type desktopNotRunningError struct{}

func (desktopNotRunningError) Error() string {
	return "Hive Desktop is not running; start it, or pass --desktop-url"
}

// DesktopClient calls tools on the running Hive Desktop. It reads the
// endpoint file on every call, because the desktop may restart on a new port.
type DesktopClient struct {
	dataDir     string
	urlOverride string
	version     string
}

// NewDesktopClient finds the desktop through the endpoint file in dataDir, or
// at urlOverride when that is set.
func NewDesktopClient(dataDir, urlOverride, version string) *DesktopClient {
	return &DesktopClient{dataDir: dataDir, urlOverride: urlOverride, version: version}
}

// Call runs one tool at mcpPath and answers the desktop's raw result, refused
// or not. An error means the call did not complete: the desktop is not
// reachable (a desktopNotRunningError or *desktopUnreachableError), or it
// answered with a protocol error, such as a tool this desktop does not have.
func (d *DesktopClient) Call(ctx context.Context, mcpPath, tool string, args map[string]any) (*mcp.CallToolResult, error) {
	baseURL, token, err := d.endpoint()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, desktopCallTimeout)
	defer cancel()

	// The standalone SSE stream carries server-initiated messages, which a
	// one-shot call never reads. Skipping it saves a request, and a negative
	// MaxRetries stops a refused connection from retrying inside the timeout.
	// The HTTP client carries its own timeout because Close sends the session
	// DELETE on a context of its own, outside ctx.
	auth := &bearerTransport{token: token}
	transport := &mcp.StreamableClientTransport{
		Endpoint:             strings.TrimRight(baseURL, "/") + mcpPath,
		HTTPClient:           &http.Client{Timeout: desktopCallTimeout, Transport: auth},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "hive", Version: d.version}, nil)
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		if auth.refused.Load() {
			return nil, d.unauthenticated(mcpPath, token)
		}
		return nil, desktopConnectError(baseURL, err)
	}
	defer func() { _ = cs.Close() }()

	// The desktop answered the connection, so a failure here is the call's
	// own: a tool this desktop does not have, or arguments it refused.
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", tool, err)
	}
	return res, nil
}

// endpoint answers the desktop's base URL and the token of its guarded MCP
// servers. An override URL takes its token from desktopTokenEnv, never from
// the endpoint file, so this desktop's token does not go to another address.
func (d *DesktopClient) endpoint() (string, string, error) {
	if d.urlOverride != "" {
		return d.urlOverride, os.Getenv(desktopTokenEnv), nil
	}
	e, err := desktopendpoint.Read(d.dataDir)
	if errors.Is(err, desktopendpoint.ErrNotRunning) {
		return "", "", desktopNotRunningError{}
	}
	if err != nil {
		return "", "", err
	}
	return e.URL, e.Token, nil
}

func (d *DesktopClient) unauthenticated(mcpPath, token string) error {
	switch {
	case d.urlOverride != "" && token == "":
		return ops.Errorf(ops.KindUnauthenticated, "Hive Desktop at %s needs a token for %s; set %s to the token in %s", d.urlOverride, mcpPath, desktopTokenEnv, desktopendpoint.Path(d.dataDir))
	case d.urlOverride != "":
		return ops.Errorf(ops.KindUnauthenticated, "Hive Desktop at %s refused the token in %s for %s", d.urlOverride, desktopTokenEnv, mcpPath)
	}
	return ops.Errorf(ops.KindUnauthenticated, "Hive Desktop refused the token in %s for %s; the file may belong to another Hive Desktop on this data directory", desktopendpoint.Path(d.dataDir), mcpPath)
}

// bearerTransport records a 401, because the SDK reports a refused request
// with the status text only.
type bearerTransport struct {
	token   string
	refused atomic.Bool
}

func (b *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if b.token != "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	res, err := http.DefaultTransport.RoundTrip(r)
	if err == nil && res.StatusCode == http.StatusUnauthorized {
		b.refused.Store(true)
	}
	return res, err
}

func (cmd *DesktopCmd) desktopClient(c *cli.Command) *DesktopClient {
	return NewDesktopClient(cmd.app.Config.DataDir, c.String(desktopURLFlag), cmd.app.Build.Version)
}

// call runs one desktop tool and answers its result. A refused call comes back
// as an *ops.Error with the tool's own text, so the message reads the same
// here as it does to an agent over MCP, and the exit code follows its kind.
func (cmd *DesktopCmd) call(ctx context.Context, c *cli.Command, mcpPath, tool string, args map[string]any) (any, error) {
	res, err := cmd.desktopClient(c).Call(ctx, mcpPath, tool, args)
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, toolError(res, tool)
	}
	return toolResultValue(res), nil
}

func toolError(res *mcp.CallToolResult, tool string) error {
	text := toolResultText(res, tool)
	kind, ok := ops.ParseKind(text)
	if !ok {
		return errors.New(text)
	}
	return &ops.Error{Kind: kind, Msg: strings.TrimSpace(strings.TrimPrefix(text, string(kind)+":"))}
}

func desktopCaller(c *cli.Command) (string, error) {
	return resolveCaller(c.String(callerFlag))
}

// resolveCaller answers who a call acts for: explicit when set, else
// HIVE_AGENT_SESSION, else the absolute current directory. It does no lookup:
// the desktop resolves the value.
func resolveCaller(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if env := strings.TrimSpace(os.Getenv("HIVE_AGENT_SESSION")); env != "" {
		return env, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("find current directory: %w", err)
	}
	return filepath.Abs(wd)
}

// desktopConnectError names the usual cause of a failed connection. A stale
// endpoint file left by a crashed desktop lands here too, since Read does not
// probe the PID.
func desktopConnectError(baseURL string, err error) error {
	if errors.Is(err, syscall.ECONNREFUSED) || strings.Contains(err.Error(), "connection refused") {
		return desktopNotRunningError{}
	}
	return &desktopUnreachableError{URL: baseURL, Err: err}
}

// desktopUnreachableError is a connection that failed for a reason other than
// a refusal, such as a timeout.
type desktopUnreachableError struct {
	URL string
	Err error
}

func (e *desktopUnreachableError) Error() string {
	return fmt.Sprintf("reach Hive Desktop at %s: %v", e.URL, e.Err)
}

func (e *desktopUnreachableError) Unwrap() error { return e.Err }

func toolResultText(res *mcp.CallToolResult, tool string) string {
	var parts []string
	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok && text.Text != "" {
			parts = append(parts, text.Text)
		}
	}
	if len(parts) == 0 {
		return tool + " failed"
	}
	return strings.Join(parts, "\n")
}

// toolResultValue prefers the structured result. An image answer, such as a
// profile's avatar, prints as its media type and base64 bytes. Otherwise it
// falls back to the text content, which holds the same JSON for tools that
// return a typed result.
func toolResultValue(res *mcp.CallToolResult) any {
	if res.StructuredContent != nil {
		return res.StructuredContent
	}
	for _, content := range res.Content {
		switch content := content.(type) {
		case *mcp.ImageContent:
			return map[string]any{"mimeType": content.MIMEType, "data": base64.StdEncoding.EncodeToString(content.Data)}
		case *mcp.TextContent:
			var v any
			if json.Unmarshal([]byte(content.Text), &v) == nil && v != nil {
				return v
			}
		}
	}
	return map[string]any{}
}
