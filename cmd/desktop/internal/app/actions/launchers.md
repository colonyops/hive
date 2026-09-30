A **launcher** opens the pop-up terminal straight into a program instead of a
bare shell: `lazygit` where the terminal you are looking at is, `btop`, a test
watcher. Something you want on screen for as long as you are using it,
and gone afterwards.

Launchers are the `launchers:` list in this same file, beside `actions:`. They
are deliberately **not** actions and carry none of the action envelope — no
`type`, `targets`, `applies_to`, `show_in_detail` or `inputs`. An action runs
something on your behalf and reports how it went; a launcher hands you a
terminal.

```yaml
launchers:
  - id: lazygit
    label: lazygit
    icon: git-branch
    command: lazygit
  - id: dotfiles
    label: Edit dotfiles
    icon: folder
    cwd: ~/.dotfiles
    command: $EDITOR .
```

## Fields

- `id` (required) — a slug, unique among launchers. Launcher ids and action ids
  are separate namespaces, so a launcher may share an id with an action.
- `label` (required) — the name shown in the command palette.
- `command` (required) — the command line the terminal opens into.
- `cwd` — pin the launcher to one directory (a leading `~` is expanded). Omit it
  to follow the terminal you are looking at, which is what makes `lazygit` open
  on the repository at its prompt — a `cd` into another checkout takes the
  launcher with it. Any terminal answers, including the ones under **Terminals**
  and a pinned chat, so a launcher is not limited to a session. A launcher with
  no `cwd` is **session-scoped**: it is offered only while a terminal is open,
  and it is not in the command palette or dispatched from its shortcut anywhere
  else. Pin a `cwd` for a launcher you want to reach from anywhere.
- `icon` — the palette glyph: `terminal` (the default), `git-branch`,
  `git-compare`, `folder`, `file-text`, `search`, `database`, `gauge`,
  `activity`, `flask-conical`, `hammer`, `container`, `cloud`, `bug`, `zap`,
  `package`.

## Reaching one

Every launcher appears in the command palette, and each is bindable under
`launcher.<id>` — unbound until you bind it. The `lazygit` launcher above gets
a shortcut by adding this to `settings.yaml`:

```yaml
keybindings:
  launcher.lazygit: [alt+g]
```

## Not templates, and not shell actions

`command` and `cwd` are used exactly as written — they are not Go templates,
which is why neither carries the `_template` suffix the rendered action fields
do. There is no triggering item to render over, and none is needed: the command
runs through a **login shell** in the working directory, so your own PATH,
aliases, functions and `$PWD` resolve it. Anything you would type in a terminal
is a valid `command`.

Use a `shell` action for something with a result to report — it runs
non-interactively, its output is captured, and its exit status becomes a job
outcome. Use a launcher for something to sit in front of.

## Lifetime

A pop-up terminal dies with the app and is never re-attachable; anything that
has to survive a restart is a hive session, not a pop-up. Quitting the program
takes the panel down with it, which is what makes quitting `lazygit` the way to
dismiss it. One pop-up is open at a time: invoking a different launcher replaces
whatever the panel is holding, and invoking the one already on screen hides it.
