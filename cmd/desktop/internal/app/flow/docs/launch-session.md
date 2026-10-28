# Launch session

A **launch-session** node is a terminal (one input, no outputs). Every item
that reaches it starts a repository-backed Hive coding session. It carries the
launch details itself, so a flow that is the only place a launch happens does
not need an entry in `actions.yml`. Use an `action` node instead when the same
launch is also offered from the item menu.

## Fields

- `repo` (required) — the remote URL of the repository the session is
  created against, a Go template. `{{ .ItemRemote }}` is the item's own
  repository, such as a pull request's, spelled as HTTPS or SSH
  by `git.clone_protocol` and `rules[].clone_protocol`; a plain URL launches every item in
  one repository. A remote that matches a configured checkout reuses it.
- `prompt` (required) — the session's initial prompt, a Go template.
- `agent` — a Hive agent profile (e.g. `claude`, `aider`). Omit it for the
  default agent.
- `sessionName` — the session's name, a Go template. The rendered text is
  turned into a valid session name. Omit it to derive one from the node id and
  the item key.
- `tags` — tags for the new session, a list of Go templates. An entry that
  renders blank is dropped. Tags let a later step find the session, such as
  an agent that lists the sessions taged `auto`.

```yaml
- id: review-prs
  type: launch-session
  repo: "{{ .ItemRemote }}"
  sessionName: "review-{{ .Payload.num }}"
  tags: [review, "pr-{{ .Payload.num }}"]
  prompt: |
    Review pull request #{{ .Payload.num }} in {{ .Payload.repo }}.

    {{ untrustedNotice }}

    {{ untrustedStart "source" "github" }}
    {{ .Payload.title }}

    {{ .Payload.body }}
    {{ untrustedEnd }}
```

## Templates

Every template renders over the same data a `launch-session` action sees:
`.Payload` is the item, `.Key` its source id, and `.Raw` the item's JSON. A
GitHub-shaped item exposes `{{ .Payload.repo }}`, `{{ .Payload.title }}`,
`{{ .Payload.url }}`, `{{ .Payload.body }}`, `{{ .Payload.num }}`, and
`{{ .Payload.author }}`.

Fence outside content in the prompt with `{{ untrustedStart }}` and
`{{ untrustedEnd }}`, and put `{{ untrustedNotice }}` before the fence, as the
`launch-session` action docs describe. Hive does not fence anything for you.

A prompt or repository that renders blank fails the launch rather than
starting an empty session. A tag longer than 40 characters, or one with a
comma, fails it too.

## One launch per item

The node launches once per item, not once per update. A pull request that
gets new commits, comments, or labels reaches the node again with the same
key, and Hive does not start a second session for it. Two launch nodes in the
same flow each launch once for the same item. An item that Hive deleted
(an archived item past retention) and that later returns counts as new.

A failed launch retries a few times and then stays failed; its error is in
Activity.

## On the item

The session is linked to the item that started it. The item's detail view
lists the session, and selecting it opens the session's terminal.

## Replay

Recomputing a flow on startup or deploy never launches anything: replay
commits feed memberships only.
