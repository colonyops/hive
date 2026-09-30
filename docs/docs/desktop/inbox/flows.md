---
icon: lucide/git-branch
description: Create and edit the graph that routes source items into feeds, notifications, and actions.
---

# Flows

A flow is a graph that connects sources to filters and destinations. Each Inbox workspace is backed by one flow file.

## Use the editor

Open a workspace's flow editor to add nodes and connect them. Select **Deploy** to save the graph.

- **Source** nodes collect items. See [Sources](sources.md) for supported providers and generic inputs.
- **Filter** nodes pass matching items and can route rejected items through a second output.
- **Function** nodes use JavaScript to reshape or split messages.
- **Feed** nodes add items to an Inbox list.
- **Notify** nodes show a banner or toast.
- **Action** nodes run a named action.

The editor and YAML file are two views of the same flow. Changes reload when the file is saved. If validation fails, Hive keeps the last valid version active and reports the error in Activity.

## A small flow

This flow puts GitHub review requests into a feed:

```yaml
version: 1
name: Reviews
nodes:
  - id: prs
    type: sources.github
    credential: github/octocat
    kind: search
    query: "is:open is:pr review-requested:@me"
  - id: reviews
    type: feed
    icon: git-branch
wires:
  - { from: prs, to: reviews }
```

Every node needs an `id` and `type`. Type-specific fields sit beside them. Wires name the source and destination node IDs.

Add a filter between the source and feed when you need narrower routing. The GitHub filter can match CI states such as `passing` or `failing` and review states such as `approved` or `changes_requested`. Branch one output to several destinations when an item should enter a feed and raise a notification or action.

## Flow files

Flows live in `~/.config/hive/desktop/flows/` by default. The filename supplies the flow ID. Hive stores canvas positions in a sibling `.ui.yaml` file.

```text
flows/
├── reviews.yaml
└── reviews.ui.yaml
```

Flow YAML is strict. Unknown fields, invalid connections, and duplicate node IDs prevent deployment.

!!! tip "Ask the Hive workspace"
    Open the **Hive** workspace in **Chats** and describe the feed you want. Its `hive-flows` skill can write and validate the flow.

## Testing a flow

The Hive workspace can dry-run a flow against sample input through the app's MCP server. A dry run reports what each node received and emitted without changing feeds or running actions.

Deploying or restarting recalculates feed membership. It does not replay notifications or actions for existing items.
