package engineops

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/messages"
	"github.com/colonyops/hive/internal/ops/sessions"
	"github.com/colonyops/hive/internal/ops/tasks"
)

// CallOn runs op on the named server, or on the server of a "<server>:<id>"
// caller when server is empty, so --server and a qualified caller pick the
// server the same way for every engine operation. Empty or
// session.LocalServer runs it in this process. The session operations carry
// their own server field and always run here, where the fleet is.
func (h *Handlers) CallOn(ctx context.Context, op Op, server string, args map[string]any) (any, error) {
	info := op.Spec.Info()
	if info.Toolset == sessions.Toolset || info.Toolset == sessions.ControlToolset {
		return op.Call(ctx, args)
	}
	caller, _ := args[ops.CallerField].(string)
	target, err := routeServer(server, caller)
	if err != nil {
		return nil, err
	}
	if target == session.LocalServer {
		if _, qualified := qualifiedCaller(caller); qualified && info.Caller != ops.CallerRuleNone {
			args[ops.CallerField] = strings.TrimPrefix(caller, session.LocalServer+":")
		}
		return op.Call(ctx, args)
	}
	client, err := h.serverClient(target)
	if err != nil {
		return nil, err
	}
	if info.Caller != ops.CallerRuleNone {
		args[ops.CallerField] = callerThere(caller)
	}
	call := daemonclient.EngineCall{Tool: info.ToolName, Args: args, Read: info.Effect == ops.EffectRead}
	call.Wait = waitOf(args)
	res, err := client.CallEngine(ctx, call)
	if err != nil {
		return nil, opError(err, fmt.Sprintf("%s on %s", info.ToolName, target))
	}
	if res.IsError {
		return nil, refusedOn(res, target)
	}
	out, err := op.Decode(res)
	if err != nil {
		return nil, err
	}
	return restoreHidden(out, call), nil
}

// restoreHidden sets the answer fields that do not cross as JSON. Blocked
// follows from the listed items: a task's open blockers are in blocker_ids,
// and its open children are in the same list.
func restoreHidden(out any, call daemonclient.EngineCall) any {
	switch out := out.(type) {
	case tasks.TaskListOut:
		markBlocked(out.Tasks)
		return out
	case messages.MessageListOut:
		out.Wait = call.Wait
		return out
	}
	return out
}

func markBlocked(items []hc.Item) {
	openChildren := map[string]bool{}
	for _, item := range items {
		if item.ParentID != "" && (item.Status == hc.StatusOpen || item.Status == hc.StatusInProgress) {
			openChildren[item.ParentID] = true
		}
	}
	for i := range items {
		items[i].Blocked = len(items[i].BlockerIDs) > 0 || openChildren[items[i].ID]
	}
}

// waitOf reads the seconds a call waits on purpose, as the CLI (int64) and
// MCP (float64) decode them.
func waitOf(args map[string]any) time.Duration {
	switch wait := args["wait"].(type) {
	case float64:
		return time.Duration(wait * float64(time.Second))
	case int64:
		return time.Duration(wait) * time.Second
	case int:
		return time.Duration(wait) * time.Second
	}
	return 0
}

// routeServer is the server a call runs on: server, else the server of a
// qualified caller, else this machine.
func routeServer(server, caller string) (string, error) {
	ref, qualified := qualifiedCaller(caller)
	switch {
	case !qualified:
		return cmpOr(server, session.LocalServer), nil
	case server == "" || server == ref.Server:
		return ref.Server, nil
	}
	return "", ops.Errorf(ops.KindInvalid, "the caller %s is on server %s, and the call names server %s", caller, ref.Server, server)
}

// qualifiedCaller reads a "<server>:<id>" caller. A path is never one, even
// with a colon in it.
func qualifiedCaller(caller string) (session.Ref, bool) {
	if caller == "" || filepath.IsAbs(caller) || !strings.Contains(caller, ":") {
		return session.Ref{}, false
	}
	ref, err := session.ParseRef(caller, session.LocalServer)
	if err != nil {
		return session.Ref{}, false
	}
	return ref, true
}

// callerThere is the caller a server reads: the id of a qualified caller, and
// global for a path, which names nothing on another machine. A bare id is a
// session or chat on that server, as a bare session id on --server is.
func callerThere(caller string) string {
	if ref, ok := qualifiedCaller(caller); ok {
		return ref.ID
	}
	if caller == "" || filepath.IsAbs(caller) {
		return sessionsvc.CallerGlobal
	}
	return caller
}

func (h *Handlers) serverClient(name string) (*daemonclient.Client, error) {
	if fleet := h.currentFleet(); fleet != nil {
		if srv, ok := fleet.Server(name); ok {
			return srv.Client(), nil
		}
	}
	return nil, ops.Errorf(ops.KindNotFound, "unknown server %q: it is not under servers: in config.yaml", name)
}

// refusedOn is a server's tool error as the *ops.Error it crossed as.
func refusedOn(res *mcp.CallToolResult, server string) error {
	var parts []string
	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok && text.Text != "" {
			parts = append(parts, text.Text)
		}
	}
	text := strings.Join(parts, "\n")
	kind, ok := ops.ParseKind(text)
	if !ok {
		return ops.Errorf(ops.KindInternal, "server %s: %s", server, cmpOr(text, "the call failed"))
	}
	return ops.Errorf(kind, "%s", strings.TrimSpace(strings.TrimPrefix(text, string(kind)+":")))
}

// decodeResult reads a tool result as Out: the structured content, else the
// JSON text the SDK writes beside it.
func decodeResult[Out any](res *mcp.CallToolResult) (any, error) {
	var raw []byte
	if res.StructuredContent != nil {
		encoded, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return nil, ops.Errorf(ops.KindInternal, "encode the server's answer: %v", err)
		}
		raw = encoded
	} else {
		for _, content := range res.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				raw = []byte(text.Text)
				break
			}
		}
	}
	var out Out
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, ops.Errorf(ops.KindInternal, "decode the server's answer: %v", err)
	}
	return out, nil
}
