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

Add a new palette to `cmd/hive/internal/theme/theme.go`:

```go
"my-theme": {
    Primary:    hex("#rrggbb"),
    Secondary:  hex("#rrggbb"),
    Foreground: hex("#rrggbb"),
    Muted:      hex("#rrggbb"),
    Background: hex("#rrggbb"),
    Surface:    hex("#rrggbb"),
    SurfaceLow: hex("#rrggbb"),
    Success:    hex("#rrggbb"),
    Warning:    hex("#rrggbb"),
    Error:      hex("#rrggbb"),
},
```

All 70+ lipgloss styles are rebuilt from these 10 colors by `SetTheme()`, so adding a palette entry is all that's needed.
