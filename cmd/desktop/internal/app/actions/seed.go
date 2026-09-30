package actions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const defaultActionsYAML = `version: 1

# This file holds two lists. "actions" is your action catalog: the buttons on an
# item's detail pane and the targets a flow action node can fire. Every action
# has a type — launch-session, shell, publish-message, or clipboard — and its
# templates are rendered over the triggering item with Go text/template.
# "launchers" is at the bottom, and opens a terminal rather than running
# something for you. This starter set demonstrates each; edit or delete anything
# here to fit your workflow.
#
# GitHub items expose {{ .Payload.repo }}, {{ .Payload.num }},
# {{ .Payload.title }}, {{ .Payload.author }}, {{ .Payload.url }},
# {{ .Payload.body }}, and {{ .Payload.labels }}.
actions:
  - id: review-pr
    label: Review PR
    type: launch-session
    show_in_detail: true
    applies_to: [pr]
    repo_template: "https://github.com/{{ .Payload.repo }}.git"
    prompt_template: |
      Review pull request #{{ .Payload.num }}: {{ .Payload.title }}
      Repository: {{ .Payload.repo }}
      {{ if .Payload.author }}Author: {{ .Payload.author }}
      {{ end }}{{ if .Payload.labels }}Labels: {{ range $i, $l := .Payload.labels }}{{ if $i }}, {{ end }}{{ $l }}{{ end }}
      {{ end }}{{ .Payload.url }}

      {{ .Payload.body }}

      Review this pull request and report back to me — do not push commits or
      submit the review on GitHub.

      - Check out the branch and read the full diff before forming an opinion.
      - Check CI: if any checks are failing, explain what broke and how to fix it.
      - Summarize what the change does, then flag anything risky, unclear, or
        missing test coverage.
      - Draft specific, line-level comments where you would request changes:
        quote the code and suggest the fix.
      - Call out what is clearly correct too, so the review is not only criticism.
  - id: address-review-feedback
    label: Address review feedback
    type: launch-session
    show_in_detail: true
    applies_to: [pr]
    repo_template: "https://github.com/{{ .Payload.repo }}.git"
    prompt_template: |
      Address the review feedback on pull request #{{ .Payload.num }}: {{ .Payload.title }}
      Repository: {{ .Payload.repo }}
      {{ .Payload.url }}

      This is my pull request and it has review feedback to resolve.

      - Check out the branch for this PR.
      - Read the review comments and unresolved conversations
        (gh pr view {{ .Payload.num }} -R {{ .Payload.repo }} --comments).
      - Make the change each comment calls for, then run the project's tests and
        linters before finishing.
      - Summarize what you changed in response to each piece of feedback.

      Ask me before force-pushing or resolving conversations on GitHub.
  - id: start-implementation
    label: Start work on issue
    type: launch-session
    show_in_detail: true
    applies_to: [issue]
    repo_template: "https://github.com/{{ .Payload.repo }}.git"
    prompt_template: |
      Start work on issue #{{ .Payload.num }}: {{ .Payload.title }}
      Repository: {{ .Payload.repo }}
      {{ if .Payload.author }}Reported by: {{ .Payload.author }}
      {{ end }}{{ if .Payload.labels }}Labels: {{ range $i, $l := .Payload.labels }}{{ if $i }}, {{ end }}{{ $l }}{{ end }}
      {{ end }}{{ .Payload.url }}

      {{ .Payload.body }}

      Before writing any code:
      - Restate the problem in your own words and list your assumptions and any
        open questions.
      - Explore the relevant code and outline a short implementation plan.
      - Share the plan and wait for my confirmation on anything ambiguous.

      Then implement the change, add or update tests, and run the project's tests
      and linters before summarizing what you did.
  # clipboard renders a template over the item and puts the result on the
  # clipboard — a ready-to-paste command, with no shell and no pbcopy. It is a
  # detail-pane affordance only, and re-copying the same item never prompts.
  - id: copy-checkout
    label: Copy checkout command
    type: clipboard
    show_in_detail: true
    applies_to: [pr]
    text_template: "gh pr checkout {{ .Payload.num }} -R {{ .Payload.repo }}"
  # shell runs any local command, rendered over the item. Pipe interpolated
  # values through shq so a repo or title with spaces or quotes cannot break
  # the command. This one opens the item on GitHub with the gh CLI.
  - id: open-on-github
    label: Open on GitHub
    type: shell
    show_in_detail: true
    applies_to: [pr, issue]
    command_template: "gh browse {{ .Payload.num }} -R {{ .Payload.repo | shq }}"
  # publish-message publishes a durable message to a fixed topic another hive
  # agent or session can subscribe to. The topic is a constant chosen here, not
  # computed from the item.
  - id: share-to-team
    label: Share to team channel
    type: publish-message
    show_in_detail: true
    applies_to: [pr, issue]
    topic: team.updates
    message_template: |
      {{ .Payload.title }} — {{ .Payload.repo }} #{{ .Payload.num }}
      {{ .Payload.url }}

# Launchers are the other list in this file, and they are not actions: a
# launcher opens the pop-up terminal straight into a program, in the checkout of
# the session you are looking at, and quitting the program closes the pop-up.
# Each one shows up in the command palette and is bindable under launcher.<id> —
# add "launcher.lazygit: [alt+g]" under keybindings in settings.yaml to give
# this one a shortcut. The command runs through a login shell, so your own PATH
# and aliases resolve it.
#
# A launcher without a "cwd" runs in the session you are looking at, so it is
# offered only while one is open. Give it a "cwd" to reach it from anywhere.
launchers:
  - id: lazygit
    label: lazygit
    icon: git-branch
    command: lazygit
`

// SeedDefaultsIfMissing installs the exact starter catalog only if path does
// not exist. It never interprets or replaces a present file, including an
// empty or invalid one. Hard-link installation is exclusive, so a concurrent
// writer that wins the race keeps its own bytes intact. The returned boolean
// reports whether this invocation installed the catalog.
func SeedDefaultsIfMissing(path string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat actions seed target: %w", err)
	}
	dir := filepath.Dir(path)
	if err := actionFS.mkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("create actions seed directory: %w", err)
	}
	f, err := actionFS.createTemp(dir, ".actions-seed-*")
	if err != nil {
		return false, fmt.Errorf("create actions seed temp: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = actionFS.remove(tmp) }()
	if err = actionFS.chmod(f, 0o600); err == nil {
		_, err = actionFS.write(f, []byte(defaultActionsYAML))
	}
	if err == nil {
		err = actionFS.sync(f)
	}
	if closeErr := actionFS.close(f); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, fmt.Errorf("write actions seed: %w", err)
	}
	if err := actionFS.link(tmp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("install actions seed: %w", err)
	}
	d, err := actionFS.open(dir)
	if err == nil {
		err = actionFS.sync(d)
		closeErr := actionFS.close(d)
		if err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return false, fmt.Errorf("sync actions seed directory: %w", err)
	}
	return true, nil
}
