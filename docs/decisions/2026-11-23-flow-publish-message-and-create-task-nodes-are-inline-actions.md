# Flow publish-message and create-task nodes are inline actions

- **Status:** accepted
- **Date:** 2026-11-23

## Context

A flow could reach the hive messaging bus only through an `action` node that
names a `publish-message` entry in `actions.yml`, and it could not create a
hive task at all. Agents that coordinate through `hive msg` and `hive hc`
need both without a second file per flow.

## Decision

`publish-message` (`topic`, `message`) and `create-task` (`title`,
`description`, `repo`, `parent`) are flow terminals that carry their config
inline, as launch nodes do (ADR a-flow-launch-node-is-a-launch-session-action-declared-inline).
Each commits an output under a synthetic action id, `publish:<flow>/<node>` or
`task:<flow>/<node>`, which `dispatch.FlowActions` resolves from the live flow
set. A publish runs the existing publish-message executor. A task runs a
create-task executor that has no `actions.yml` type.

A publish dedups on the occurrence key: a message reports an event, so an item
that changes publishes again. A task dedups on the item key: it is work that
should exist once, like a launch.

The topic stays a literal and the parent a literal id, for the same reason the
publish-message action's topic is literal: the author picks the routing when
they write the flow.

## Consequences

- Retention keeps a terminal `task:` row while its item exists, because that
  row is the only once-per-item guard.
- A task's `repo` must render as `owner/repo` or the command fails, so a bad
  template cannot file tasks under a key no repository lists.
