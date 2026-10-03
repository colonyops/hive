---
name: cli-tui
description: Build or change the hive CLI's Bubble Tea TUI under cmd/hive/internal/tui -- views, modals, keybindings, async loads -- and test it with component and golden-file tests. Use when adding a view, a modal, a key, or a palette command to the TUI, when a TUI test or golden file fails, or when asked how the TUI is put together.
---

# Work on the hive TUI

The TUI is Bubble Tea v2 (`charm.land/bubbletea/v2`, `bubbles/v2`,
`lipgloss/v2`). For general Bubble Tea guidance, read the upstream docs; this
skill covers how this TUI is put together and the rules it is reviewed
against.

## Where things live

```
cmd/hive/internal/tui/
├── model.go              # Model, Update loop, message types, key routing
├── model_handlers.go     # one handler per message or action
├── model_render.go       # View composition
├── keybindings.go        # KeybindingResolver (config -> action)
├── modal_coordinator.go  # every modal, pending action, and stream state
├── command_palette.go    # the `:` palette
├── components/           # reusable widgets: dialogs, form, status bar
├── views/                # one package per view: sessions, tasks, messages, review, shared
├── sourcepicker/         # the issue/PR source picker
└── testutil/             # RequireGolden, StripANSI, fixtures
cmd/hive/internal/styles/ # lipgloss styles and the active theme
```

The UI owns no business logic. It calls the service layer
(`internal/hive`) and shared packages in `internal/`, which must never import
charm (depguard fails the lint).

## Views

Each view is its own package under `views/`, with its own model, `Update`,
`View`, `SetSize`, and message types. A view that needs to resolve keys
declares a small `KeyResolver` interface (`views/sessions/key_resolver.go`)
that the parent's `KeybindingResolver` satisfies. That interface exists to
break the import cycle between `tui` and the view; do not import `tui` from a
view.

`ViewType` in `view.go` names the views. Its `String()` value is the scope
name that `cfg.Views.<View>.Keybindings` matches against.

## Keys

The precedence is in the root `AGENTS.md` ("Keybinding Precedence"). The rule
that matters most: **a user-overridable key is never an `if keyStr == "x"`
block in view code.** To add one:

1. register an `action.Type` in `cmd/hive/internal/action/type.go` and run
   `mise run generate:enums`;
2. add a default `UserCommand` to `defaultUserCommands`
   (`cmd/hive/internal/config/config.go`);
3. bind it in `defaultViewsConfig` (`cmd/hive/internal/config/config_views.go`);
4. dispatch it from `model_handlers.go`.

Hardcoded keys are only the layer-1 set: `ctrl+c`, `esc`, `tab`,
`shift+tab`, and modal dismissal. Document a new default on
`docs/docs/cli/configuration/keybindings.md` (the `docs-audit` skill).

## Modals

All modal state lives on `ModalCoordinator`. A new modal is a field there, a
`UIState` value for "this modal is open", a key handler in `model.go` that
routes keys to it in that state, and a case in `ModalCoordinator.Overlay`.
Do not keep modal state on a view.
`NewDangerousModal` is the confirm dialog for anything that destroys work: the
user types the verb to enable it.

## Async work

`Update` never blocks. Anything that touches git, tmux, the database, or the
network runs in a `tea.Cmd` that returns a message, and the handler applies
the result. Batch fan-out work into one completion message
(`GitStatusBatchCompleteMsg`) rather than one message per item. Polling is a
tick message that schedules the next tick (`sessions.TerminalPollTickMsg`).

Background failures cannot reach the user directly, so log them: `debug` for
expected or transient failures, `warn` for configuration problems. Show a
degraded state (a `StatusMissing` indicator) instead of dropping the item.

## Testing

Test behavior through the public surface: build the model, call `SetSize`,
send messages through `Update`, and assert on state or on `View()` output.
Always call `SetSize` before rendering; a zero-size view renders nothing
useful.

**Golden files** cover rendered output (`views/review/view_golden_test.go`,
`modal_golden_test.go`):

```go
view := New(docs, "", nil, nil, 0)
view.SetSize(80, 24)
testutil.RequireGolden(t, testutil.StripANSI(view.View()))
```

Strip ANSI so output does not depend on the terminal's color profile. A
package whose output still depends on the theme pins one in `TestMain`
(`styles.SetTheme` with `tokyo-night`, as `modal_golden_test.go` does). Fix
timestamps in fixtures (`testutil.CreateTestComment`).

Golden files live in each package's `testdata/`. Regenerate them only after
you have read the diff and agree the new output is right:

```bash
go test ./cmd/hive/internal/tui/views/review -run TestView_ListMode -update
```

Do not test cosmetic details in assertions; a golden file is the place for
layout. End-to-end CLI behavior belongs in the Docker integration tests
(`mise run cli:integration`), never run on the host.

## Verify

```bash
go test ./cmd/hive/...
mise run lint
mise run cli:dev     # run the TUI against the dev config and .data/
```

For anything that needs real tmux sessions, use `mise run cli:container`.
