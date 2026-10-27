# A flow launch node is a launch-session action declared inline

- **Status:** accepted
- **Date:** 2026-10-02

## Context

Without an inline node, a flow reaches a session or chat only through an
`action` node naming a `launch-session` entry in `actions.yml`. When the flow
is the only caller, the author edits two files for one launch, and the flow
editor cannot show what will start.

## Decision

`launch-session` (`repo`, `agent`, `sessionName`, `prompt`) and `launch-chat`
(`workspace`, `prompt`) are flow terminals that carry the launch inline. Each
commits a `launch` output under a synthetic action id, `launch:<flow>/<node>`,
which the output worker resolves from the live flow set into a
`launch-session` action (`dispatch.LaunchNodeActionConfig`). The existing
executor runs it, so a node launches exactly as the equivalent catalog action
would. `sessionName` exists only on the node, not on the `actions.yml` schema.

A launch dedups on the item key, not the occurrence key a notify or action
node uses: a pull request that keeps changing starts one session, not one per
update.

A chat opened for an item, by a node or by a workspace `launch-session`
action, is linked to it in `item_chat`. The link has a foreign key to the
chat row with `ON DELETE CASCADE`, so deleting the chat removes the link.

## Consequences

- The `action` node remains for a launch that is also offered from the item
  menu.
- An item that leaves and later re-enters a launch node does not launch
  again, since its key is already queued. Retention keeps that row while the
  item exists.
- The detail pane lists chats beside sessions.
