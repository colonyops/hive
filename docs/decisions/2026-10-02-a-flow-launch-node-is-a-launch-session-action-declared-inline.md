# A flow launch node is a launch-session action declared inline

- **Status:** accepted
- **Date:** 2026-10-02

## Context

A flow reached a session or chat only through an `action` node naming a
`launch-session` entry in `actions.yml`. When the flow is the only caller, the
author edits two files to express one launch, and the flow editor cannot show
what will start.

## Decision

`launch-session` (`repo`, `agent`, `sessionName`, `prompt`) and `launch-chat`
(`workspace`, `prompt`) are flow terminals that carry the launch inline. Like a
notify node, each commits a `launch` output enqueued under a synthetic action
id, `launch:<flow>/<node>`, which the output worker resolves from the live flow
set into a `launch-session` action. The existing executor runs it, so a node
launches exactly as the equivalent catalog action would. `sessionName` exists
only on the node; it travels on a dispatch-owned projection, not on the
`actions.yml` schema.

A launch dedups on the item key, not on the occurrence key a notify or action
node uses: a pull request that keeps changing must start one session, not one
per update.

A chat opened for an item, by a node or by a workspace `launch-session` action,
is linked to it in `item_chat`. This revises ADR
a-launch-session-action-targets-either-a-repository-or-an-agent-workspace,
under which a workspace launch recorded no item association. Unlike
`item_session`, the link carries a foreign key to the chat row, because the
chat lives in the same database and its delete should take the link with it.

## Consequences

A launch that only one flow performs needs no catalog entry, and the `action`
node remains for a launch also offered from the item menu. An item that leaves
and later re-enters a launch node does not launch again, since its key is
already queued. Retention keeps that queued row while the item exists, so the
guard ends only when retention deletes the item itself. The detail pane lists chats beside sessions.
