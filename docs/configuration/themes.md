---
icon: lucide/palette
---

# Themes

Hive ships with five built-in color themes. Each theme defines 9 semantic colors that drive all TUI styles.

```yaml
tui:
  theme: catppuccin
```

## Available Themes

| Theme         | Description                |
| ------------- | -------------------------- |
| `tokyo-night` | Default — cool blue/purple |
| `gruvbox`     | Warm retro                 |
| `catppuccin`  | Catppuccin Mocha           |
| `kanagawa`    | Kanagawa Wave              |
| `onedark`     | One Dark                   |

## Semantic Color Roles

| Role         | Usage                                                    |
| ------------ | -------------------------------------------------------- |
| `Primary`    | Selections, borders, active elements                     |
| `Secondary`  | IDs, branches, links                                     |
| `Foreground` | Main text                                                |
| `Muted`      | De-emphasized text, help text, dividers                  |
| `Background` | Base background                                          |
| `Surface`    | Elevated surfaces (modals, selections, status bar)       |
| `Success`    | Positive states (active agent, open PRs, clean git)      |
| `Warning`    | Caution states (needs approval, dirty git)               |
| `Error`      | Error states, destructive actions, search highlights     |

!!! tip "Live preview"
    Use the `:ThemePreview` command in the TUI to cycle through available themes and see them applied in real time.

## Adding a Theme

Add a new palette to `internal/core/styles/themes.go`:

```go
"my-theme": {
    Primary:    lipgloss.Color("#hex"),
    Secondary:  lipgloss.Color("#hex"),
    Foreground: lipgloss.Color("#hex"),
    Muted:      lipgloss.Color("#hex"),
    Background: lipgloss.Color("#hex"),
    Surface:    lipgloss.Color("#hex"),
    Success:    lipgloss.Color("#hex"),
    Warning:    lipgloss.Color("#hex"),
    Error:      lipgloss.Color("#hex"),
},
```

All 70+ lipgloss styles are rebuilt from these 9 colors by `SetTheme()`, so adding a palette entry is all that's needed.
