// Package chats declares the agent workspace chat operations. They are served
// on their own token-guarded MCP server, because starting a chat runs the
// workspace's command, which can bypass the agent's permission prompts.
package chats

import "github.com/colonyops/hive/internal/ops"

var Toolset = ops.Toolset{Name: "chats", MCPPath: "/mcp/chats"}

func All() []ops.Op {
	return []ops.Op{ChatList, ChatCreate, ChatPinPut, ChatPinDelete}
}

type ChatListIn struct {
	Workspace string `json:"workspace,omitempty" jsonschema:"Only the chats of this workspace, by its directory name as workspace_list reports it in dir. Omit for every workspace."`
}

var ChatList = ops.Spec[ChatListIn]{
	Name: "chat.list", Owner: ops.OwnerDesktop, Effect: ops.EffectRead, Toolset: Toolset,
	Summary: "List agent workspace chats",
	Guidance: "List chats newest first, with each one's workspace, name, whether a terminal is running it, and pinnedAt: " +
		"the Unix time in milliseconds when it was pinned to the Code sidebar, or 0 when it is not pinned. " +
		"A workspace that does not exist is not_found.",
}

type ChatCreateIn struct {
	Workspace string `json:"workspace"           jsonschema:"The workspace directory name, as workspace_list reports it in dir."`
	Name      string `json:"name"                jsonschema:"The chat's name, shown in the Chats sidebar."`
	Prompt    string `json:"prompt,omitempty"    jsonschema:"The agent's opening message, as plain text. Omit to start the agent with no message. The workspace command must pass .Prompt to take one."`
	PinToCode bool   `json:"pinToCode,omitempty" jsonschema:"true pins the new chat to the Code sidebar, as the New chat dialog's Pin to Code does."`
}

var ChatCreate = ops.Spec[ChatCreateIn]{
	Name: "chat.create", Owner: ops.OwnerDesktop, Effect: ops.EffectExec, Toolset: Toolset,
	Summary: "Start an agent workspace chat",
	Guidance: "Start a new chat in a workspace, the same launch the New chat dialog makes, and return it with its id. " +
		"It runs the workspace's command, with the workspace's MCP servers and permission settings, so a workspace whose command bypasses permissions " +
		"starts an agent that acts without asking. The chat runs in the background until someone opens it. " +
		"Unlike a scheduled chat, Hive does not frame the prompt or tell the agent to end the session. " +
		"A workspace that does not exist is not_found. A prompt for a workspace whose command has no .Prompt is invalid, and nothing starts.",
}

type ChatPinIn struct {
	ID string `json:"id" jsonschema:"The chat id, from chat_list or chat_create."`
}

var ChatPinPut = ops.Spec[ChatPinIn]{
	Name: "chat_pin.put", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Pin a chat to the Code sidebar",
	Guidance: "Pin one chat to the Code sidebar, after the chats already pinned, and return the chat. A chat that is already pinned keeps its place. " +
		"This changes the sidebar only: it starts and stops nothing. An id that matches no chat is not_found.",
	Args: []string{"id"},
}

var ChatPinDelete = ops.Spec[ChatPinIn]{
	Name: "chat_pin.delete", Owner: ops.OwnerDesktop, Effect: ops.EffectWrite, Toolset: Toolset,
	Summary: "Unpin a chat from the Code sidebar",
	Guidance: "Remove one chat from the Code sidebar and return the chat. A chat that is not pinned is left as it is. " +
		"The chat itself, and any terminal that runs it, stays. An id that matches no chat is not_found.",
	Args: []string{"id"},
}
