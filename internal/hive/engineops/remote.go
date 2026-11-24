package engineops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/sessions"
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
	if wait, ok := args["wait"].(float64); ok && wait > 0 {
		call.Wait = time.Duration(wait * float64(time.Second))
	}
	res, err := client.CallEngine(ctx, call)
	if err != nil {
		return nil, opError(err, fmt.Sprintf("%s on %s", info.ToolName, target))
	}
	if res.IsError {
		return nil, refusedOn(res, target)
	}
	return op.Decode(res)
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
		return nil, ops.Errorf(ops.KindInternal, "decode the server's answer: %v", errors.Unwrap(err))
	}
	return out, nil
}
