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
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/platform/desktopendpoint"
)

const (
	desktopCallTimeout = 10 * time.Second

	callerFlag     = "caller"
	desktopURLFlag = "desktop-url"
)

// desktopNotRunningError is a type, not errors.New, because the message starts
// with the product name and staticcheck rejects a capitalized error string.
type desktopNotRunningError struct{}

func (desktopNotRunningError) Error() string {
	return "Hive Desktop is not running; start it, or pass --desktop-url"
}

// call runs one desktop tool and answers its result. A refused call comes back
// as an *ops.Error with the tool's own text, so the message reads the same
// here as it does to an agent over MCP, and the exit code follows its kind.
func (cmd *DesktopCmd) call(ctx context.Context, c *cli.Command, mcpPath, tool string, args map[string]any) (any, error) {
	baseURL, err := cmd.desktopURL(c)
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
	transport := &mcp.StreamableClientTransport{
		Endpoint:             strings.TrimRight(baseURL, "/") + mcpPath,
		HTTPClient:           &http.Client{Timeout: desktopCallTimeout},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "hive", Version: cmd.app.Build.Version}, nil)
	cs, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, desktopConnectError(baseURL, err)
	}
	defer func() { _ = cs.Close() }()

	// The desktop answered the connection, so a failure here is the call's
	// own: a tool this desktop does not have, or arguments it refused.
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", tool, err)
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

func (cmd *DesktopCmd) desktopURL(c *cli.Command) (string, error) {
	if u := c.String(desktopURLFlag); u != "" {
		return u, nil
	}
	e, err := desktopendpoint.Read(cmd.app.Config.DataDir)
	if errors.Is(err, desktopendpoint.ErrNotRunning) {
		return "", desktopNotRunningError{}
	}
	if err != nil {
		return "", err
	}
	return e.URL, nil
}

func desktopCaller(c *cli.Command) (string, error) {
	if s := c.String(callerFlag); s != "" {
		return s, nil
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
	return fmt.Errorf("reach Hive Desktop at %s: %w", baseURL, err)
}

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
