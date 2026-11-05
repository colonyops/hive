package mcpsrv

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/ops/chats"
	"github.com/colonyops/hive/internal/ops/mcpops"
	"github.com/colonyops/hive/pkg/logutils"
)

// ChatsPathPrefix must agree with the chats toolset's MCPPath and with
// mcpcatalog's hive-chats RuntimePath.
const ChatsPathPrefix = "/mcp/chats"

// chatsServerName is also the id a workspace names in its mcps: list.
const chatsServerName = "hive-chats"

// ChatsController serves the chat operations. Unlike the other two servers it
// requires a bearer token, because chat_create runs a workspace's command. A
// web page can send a request to the loopback port; only the user's own
// processes can read the token from the endpoint file.
type ChatsController struct {
	core  *app.App
	log   zerolog.Logger
	opts  Options
	token string
}

// MintChatsToken returns a fresh per-run token for the chats server.
func MintChatsToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("mint chats token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// NewChats panics on an empty token: a guarded server with no token would
// let every request through.
func NewChats(core *app.App, log zerolog.Logger, opts Options, token string) *ChatsController {
	if token == "" {
		panic("mcpsrv: the chats server needs a token")
	}
	return &ChatsController{core: core, log: logutils.Component(log, "mcp.chats"), opts: opts, token: token}
}

func (ctrl *ChatsController) Server() *mcp.Server {
	version := ctrl.opts.Version
	if version == "" {
		version = "dev"
	}
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    chatsServerName,
		Title:   "Hive Chats",
		Version: version,
		Description: "Start agent workspace chats with an opening prompt, list them, and pin or unpin them in the Code sidebar. " +
			"Starting a chat runs the workspace's command with its permission settings.",
	}, nil)
	mcpops.Mount(srv, chats.ChatList, ctrl.ListChats, ctrl.toolError)
	mcpops.Mount(srv, chats.ChatCreate, ctrl.CreateChat, ctrl.toolError)
	mcpops.Mount(srv, chats.ChatPinPut, ctrl.PinChat, ctrl.toolError)
	mcpops.Mount(srv, chats.ChatPinDelete, ctrl.UnpinChat, ctrl.toolError)
	return srv
}

// Handler is stateless and plain JSON like the other servers, with the SDK's
// DNS-rebinding protection left on in front of the token check.
func (ctrl *ChatsController) Handler() http.Handler {
	srv := ctrl.Server()
	inner := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(presented)), []byte(ctrl.token)) != 1 {
			http.Error(w, "unauthenticated: hive-chats needs the bearer token from the desktop endpoint file; use hive mcp serve or hive dt", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

func (ctrl *ChatsController) toolError(err error) error { return toolError(ctrl.log, err) }

type chatView struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
	Name      string `json:"name"`
	Agent     string `json:"agent"`
	Slug      string `json:"slug"             jsonschema:"The chat's tmux session name."`
	Running   bool   `json:"running"          jsonschema:"Whether a terminal runs the chat now."`
	PinnedAt  int64  `json:"pinnedAt"         jsonschema:"Unix milliseconds when the chat was pinned to the Code sidebar; 0 when it is not pinned."`
	Notice    string `json:"notice,omitempty"`
}

func toChatView(v app.SessionView) chatView {
	return chatView{
		ID: v.ID, Workspace: v.Workspace, Name: v.Name, Agent: v.Agent, Slug: v.Slug,
		Running: v.TerminalID != "", PinnedAt: v.PinnedAt, Notice: v.Notice,
	}
}

type chatListOutput struct {
	Chats []chatView `json:"chats"`
}

func (ctrl *ChatsController) ListChats(ctx context.Context, in chats.ChatListIn) (chatListOutput, error) {
	views, err := ctrl.core.AgentWorkspaces.ListChats(ctx, in.Workspace)
	if err != nil {
		return chatListOutput{}, err
	}
	out := chatListOutput{Chats: make([]chatView, 0, len(views))}
	for _, v := range views {
		out.Chats = append(out.Chats, toChatView(v))
	}
	return out, nil
}

func (ctrl *ChatsController) CreateChat(ctx context.Context, in chats.ChatCreateIn) (chatView, error) {
	view, err := ctrl.core.AgentWorkspaces.CreateChat(ctx, app.CreateChat{
		Workspace: in.Workspace, Name: in.Name, Prompt: in.Prompt, PinToCode: in.PinToCode,
	})
	if err != nil {
		return chatView{}, err
	}
	return toChatView(view), nil
}

func (ctrl *ChatsController) PinChat(ctx context.Context, in chats.ChatPinIn) (chatView, error) {
	view, err := ctrl.core.AgentWorkspaces.PinChat(ctx, in.ID, true)
	if err != nil {
		return chatView{}, err
	}
	return toChatView(view), nil
}

func (ctrl *ChatsController) UnpinChat(ctx context.Context, in chats.ChatPinIn) (chatView, error) {
	view, err := ctrl.core.AgentWorkspaces.PinChat(ctx, in.ID, false)
	if err != nil {
		return chatView{}, err
	}
	return toChatView(view), nil
}
