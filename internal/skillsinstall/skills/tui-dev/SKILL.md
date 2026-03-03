---
name: tui-dev
description: >
  Expert guidance for building TUI applications with the Charm Bracelet ecosystem
  (Bubbletea, Lipgloss, Bubbles). Use when implementing components, styling, or
  working with MVU architecture.
---

# TUI Development Expert

Expert guidance for building Terminal User Interfaces with the Charm Bracelet ecosystem (Bubbletea, Lipgloss, Bubbles).

## When to Use This Skill

Activate this skill when:
- Building or debugging TUI applications
- Implementing Bubbletea components
- Styling with Lipgloss
- Creating dialogs or overlays
- Testing TUI rendering
- Implementing keyboard/mouse handling
- Managing application state with MVU pattern

## Core Principles

### 1. Model-View-Update (MVU) Architecture

**Always follow the MVU pattern:**

```go
type Model struct {
    // Pure state only - no I/O, no channels, no goroutines
    value    string
    cursor   int
    focused  bool
}

func (m Model) Init() tea.Cmd {
    // Return initial commands (I/O operations)
    return tea.Batch(
        fetchData(),
        startTimer(),
    )
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // Pure state updates only
    // Side effects returned as commands
    switch msg := msg.(type) {
    case tea.KeyPressMsg:
        // Update state
        m.cursor++
        // Return command for side effects
        return m, doSomething()
    }
    return m, nil
}

func (m Model) View() tea.View {
    // Pure rendering function
    // No side effects, just transform state to visual output
    var v tea.View
    v.Content = lipgloss.NewStyle().Render(m.value)
    v.AltScreen = true
    v.MouseMode = tea.MouseModeCellMotion
    return v
}
```

**Key Rules:**
- Model holds pure state (no channels, mutexes, goroutines)
- Update is pure (no I/O, returns new model + command)
- View is pure (no side effects, just rendering)
- View returns tea.View struct with Content field
- Commands handle all I/O and side effects

### 2. Component Structure

**Standard component pattern:**

```go
type Component struct {
    // State
    value    string
    focused  bool

    // Sub-components (embed, don't wrap in pointers)
    textarea textarea.Model
    spinner  spinner.Model

    // Configuration
    width    int
    styles   Styles
}

// Constructor with functional options
func New(opts ...Option) Component {
    c := Component{
        width: 80,
        styles: DefaultStyles(),
    }
    for _, opt := range opts {
        opt(&c)
    }
    return c
}

type Option func(*Component)

func WithWidth(w int) Option {
    return func(c *Component) { c.width = w }
}

// Standard methods
func (c *Component) Focus() tea.Cmd
func (c *Component) Blur()
func (c Component) Focused() bool
func (c *Component) SetValue(v string)
func (c Component) Value() string
```

### 3. Message Propagation

**Route messages correctly:**

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    // Handle own messages first
    switch msg := msg.(type) {
    case tea.KeyPressMsg:
        if msg.String() == "tab" {
            m.activeView = (m.activeView + 1) % 2
            return m, nil
        }
    }

    // Broadcast to all (e.g., window size)
    if _, ok := msg.(tea.WindowSizeMsg); ok {
        m.header, _ = m.header.Update(msg)
        m.content, _ = m.content.Update(msg)
    }

    // Route to active component only
    var cmd tea.Cmd
    switch m.activeView {
    case 0:
        m.editor, cmd = m.editor.Update(msg)
    case 1:
        m.table, cmd = m.table.Update(msg)
    }
    cmds = append(cmds, cmd)

    return m, tea.Batch(cmds...)
}
```

### 4. Styling with Lipgloss

**Organize styles in structs:**

```go
type Styles struct {
    Header    lipgloss.Style
    Content   lipgloss.Style
    Error     lipgloss.Style
    Button    lipgloss.Style
    ButtonActive lipgloss.Style
}

func DefaultStyles() Styles {
    base := lipgloss.NewStyle()

    button := base.Copy().
        Padding(0, 3).
        Background(lipgloss.Color("240"))

    return Styles{
        Header: base.Copy().
            Bold(true).
            Foreground(lipgloss.Color("230")),
        Content: base.Copy().
            Padding(1, 2),
        Button: button,
        ButtonActive: button.Copy().
            Background(lipgloss.Color("63")).
            Underline(true),
        Error: base.Copy().
            Foreground(lipgloss.Color("196")),
    }
}

// Make themeable
func NewThemedStyles(hasDarkBG bool) Styles {
    lightDark := lipgloss.LightDark(hasDarkBG)

    return Styles{
        Header: lipgloss.NewStyle().
            Foreground(lightDark(
                lipgloss.Color("#000"),
                lipgloss.Color("#FFF"),
            )),
    }
}
```

**Styling rules:**
- Never embed ANSI codes in strings
- Styles are immutable (methods return copies)
- Use `lipgloss.Println()` for automatic color downsampling
- Organize styles in theme structs
- Support light/dark backgrounds

### 5. Dialog Pattern

**Implement overlay system for dialogs:**

```go
// Dialog interface
type Dialog interface {
    ID() string
    HandleMsg(msg tea.Msg) Action
    Draw(scr Screen, area Rectangle) *tea.Cursor
}

// Action types for dialog responses
type Action any
type ActionClose struct{}
type ActionConfirm struct{ Value string }

// Overlay manager with stack
type Overlay struct {
    dialogs []Dialog
}

func (o *Overlay) OpenDialog(d Dialog) {
    o.dialogs = append(o.dialogs, d)
}

func (o *Overlay) Update(msg tea.Msg) Action {
    if len(o.dialogs) == 0 {
        return nil
    }
    // Only top dialog receives messages
    return o.dialogs[len(o.dialogs)-1].HandleMsg(msg)
}

// In main model Update:
if m.overlay.HasDialogs() {
    action := m.overlay.Update(msg)
    switch action := action.(type) {
    case ActionClose:
        m.overlay.CloseFrontDialog()
        return m, nil
    }
    if action != nil {
        return m, nil  // Dialog handled it
    }
}
```

### 6. Testing Strategy

**Write testable TUI code:**

```go
// 1. Golden file testing for rendering
func TestRender(t *testing.T) {
    m := Model{title: "Test", width: 80}
    output := m.View().Content
    golden.RequireEqual(t, output)
}

// 2. Test Update directly
func TestUpdateKeyPress(t *testing.T) {
    m := Model{cursor: 0}
    newModel, _ := m.Update(tea.KeyPressMsg{Type: tea.KeyDown})

    if newModel.(Model).cursor != 1 {
        t.Errorf("cursor = %v, want 1", newModel.(Model).cursor)
    }
}

// 3. Use teatest for integration
func TestProgram(t *testing.T) {
    tm := teatest.NewTestModel(t, NewModel(),
        teatest.WithInitialTermSize(80, 24),
    )
    t.Cleanup(func() { tm.Quit() })

    tm.Type("hello")
    tm.Send(tea.KeyPressMsg{Type: tea.KeyEnter})

    output := tm.FinalOutput(t)
    golden.RequireEqual(t, output)
}
```

**Testing rules:**
- Golden files for visual regression
- Test Update functions directly
- Mock I/O with `bytes.Buffer`
- Use table-driven tests
- Verify state, not implementation
- Access view content via `.Content` field

### 7. Keyboard Handling

**Implement consistent key bindings:**

```go
import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
    Up    key.Binding
    Down  key.Binding
    Quit  key.Binding
}

func DefaultKeyMap() KeyMap {
    return KeyMap{
        Up: key.NewBinding(
            key.WithKeys("up", "k"),
            key.WithHelp("↑/k", "move up"),
        ),
        Quit: key.NewBinding(
            key.WithKeys("q", "ctrl+c"),
            key.WithHelp("q", "quit"),
        ),
    }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:
        switch {
        case key.Matches(msg, m.keyMap.Up):
            m.cursor--
        case key.Matches(msg, m.keyMap.Quit):
            return m, tea.Quit
        }
    }
    return m, nil
}
```

**For safe cancellation (two-press pattern):**

```go
case tea.KeyPressMsg:
    switch msg.String() {
    case "esc":
        if m.isCanceling {
            // Second press - actually cancel
            return m, cancelWork()
        }
        // First press - start 2s timer
        m.isCanceling = true
        return m, cancelTimer(2 * time.Second)
    }

case cancelTimerExpiredMsg:
    m.isCanceling = false
```

## Common Patterns

### Focus Management

```go
type FocusState int

const (
    focusNone FocusState = iota
    focusEditor
    focusTable
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    if !m.focused {
        return m, nil  // Early return when not focused
    }

    // Handle input...
}

func (m *Model) Focus() tea.Cmd {
    m.focused = true
    return m.subComponent.Focus()
}

func (m *Model) Blur() {
    m.focused = false
    m.subComponent.Blur()
}
```

### State Machines

```go
type State int

const (
    stateLoading State = iota
    stateReady
    stateEditing
    stateConfirming
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // Route based on state
    switch m.state {
    case stateLoading:
        return m.handleLoading(msg)
    case stateEditing:
        return m.handleEditing(msg)
    }
    return m, nil
}
```

### Layout

```go
// Vertical layout
view := lipgloss.JoinVertical(
    lipgloss.Left,
    m.headerStyle.Render(header),
    m.contentStyle.Render(content),
    m.footerStyle.Render(footer),
)

// Horizontal layout
view := lipgloss.JoinHorizontal(
    lipgloss.Top,
    leftPanel,
    rightPanel,
)

// Centered
view := lipgloss.Place(
    m.width, m.height,
    lipgloss.Center, lipgloss.Middle,
    content,
)
```

### Commands

```go
// Simple command
func fetchData() tea.Msg {
    data := callAPI()
    return dataReceivedMsg{data}
}

// Parameterized command
func fetchDataByID(id int) tea.Cmd {
    return func() tea.Msg {
        data := callAPI(id)
        return dataReceivedMsg{id, data}
    }
}

// Tick command for animations
func tick() tea.Cmd {
    return tea.Tick(time.Second/20, func(time.Time) tea.Msg {
        return tickMsg{}
    })
}
```

### View Composition

**When composing sub-component views, access .Content field:**

```go
func (m Model) View() tea.View {
    var v tea.View

    // Get content from sub-components
    headerContent := m.header.View().Content
    contentContent := m.content.View().Content
    footerContent := m.footer.View().Content

    // Compose the layout
    v.Content = lipgloss.JoinVertical(
        lipgloss.Left,
        headerContent,
        contentContent,
        footerContent,
    )

    // Set view options for full-screen apps
    v.AltScreen = true
    v.MouseMode = tea.MouseModeCellMotion

    return v
}
```

## Anti-Patterns to Avoid

❌ **Don't put I/O in Update:**
```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    data := http.Get("...") // NO! Use a command
}
```

❌ **Don't use pointers for Bubbletea components:**
```go
type Model struct {
    textarea *textarea.Model // NO! Use value type
}
```

❌ **Don't embed ANSI codes:**
```go
return "\x1b[1mBold\x1b[0m" // NO! Use Lipgloss
```

❌ **Don't mutate in View:**
```go
func (m Model) View() tea.View {
    m.rendered = true // NO! View must be pure
}
```

❌ **Don't block in commands:**
```go
func badCommand() tea.Msg {
    time.Sleep(5 * time.Second) // NO! Use tea.Tick
}
```

❌ **Don't return string from View:**
```go
func (m Model) View() string { // NO! Return tea.View in v2
    return "content"
}
```

❌ **Don't compose views as strings:**
```go
func (m Model) View() tea.View {
    var v tea.View
    v.Content = m.header.View() + m.content.View() // NO! Access .Content
    return v
}
```

## Checklist for New TUI Apps

Before starting:
- [ ] Define model struct with pure state
- [ ] Implement Init, Update, View
- [ ] Create Styles struct for theming
- [ ] Define KeyMap for all keyboard shortcuts
- [ ] Plan component hierarchy
- [ ] Decide on dialog/overlay needs

During development:
- [ ] Keep Update pure (no I/O)
- [ ] Use commands for all side effects
- [ ] Handle tea.WindowSizeMsg
- [ ] Implement focus management
- [ ] Support light/dark themes
- [ ] Add help text (bubbles/help)
- [ ] Return tea.View from View() methods
- [ ] Access .Content when composing sub-component views

Before release:
- [ ] Write golden file tests
- [ ] Test all keyboard shortcuts
- [ ] Test window resize behavior
- [ ] Test color profiles (8, 256, truecolor)
- [ ] Add cancellation for long operations
- [ ] Handle errors gracefully

## Program Options Reference

```go
p := tea.NewProgram(model,
    // Testing
    tea.WithInput(&input),
    tea.WithOutput(&output),
    tea.WithWindowSize(80, 24),

    // Production
    tea.WithAltScreen(),
    tea.WithMouseCellMotion(),
    tea.WithColorProfile(colorprofile.TrueColor),

    // Daemon mode
    tea.WithoutRenderer(),

    // Cancellation
    tea.WithContext(ctx),

    // Message filtering
    tea.WithFilter(filterFunc),
)
```

## Quick Reference: Built-in Messages

```go
tea.KeyPressMsg      // Keyboard input
tea.KeyReleaseMsg
tea.MouseClickMsg    // Mouse events
tea.MouseWheelMsg
tea.WindowSizeMsg    // Terminal resize
tea.QuitMsg          // Program should quit
tea.InterruptMsg     // ctrl+c
tea.SuspendMsg       // ctrl+z
tea.ColorProfileMsg  // Color capability
```

## Installation

Install Bubbletea v2 packages (RC/Beta versions):

```bash
go get charm.land/bubbletea/v2@v2.0.0-rc.2
go get charm.land/lipgloss/v2@v2.0.0-beta.3
go get charm.land/bubbles/v2@v2.0.0-rc.1
```

Import in your code:

```go
import (
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
    "charm.land/bubbles/v2/textinput"
    "charm.land/bubbles/v2/textarea"
    "charm.land/bubbles/v2/spinner"
    "charm.land/bubbles/v2/list"
    "charm.land/bubbles/v2/table"
)
```

## Resources

- Bubbletea: https://github.com/charmbracelet/bubbletea
- Lipgloss: https://github.com/charmbracelet/lipgloss
- Bubbles: https://github.com/charmbracelet/bubbles
- Examples: https://github.com/charmbracelet/bubbletea/tree/master/examples

## Implementation Guidelines

When implementing TUI features:

1. **Start with the model** - Define state structure first
2. **Implement Init** - Return initial commands
3. **Implement Update** - Handle all message types
4. **Implement View** - Return tea.View with Content set
5. **Add tests** - Golden files for rendering, unit tests for Update
6. **Add keyboard shortcuts** - Use KeyMap pattern
7. **Add help** - Use bubbles/help component

When debugging:
- Log to file (not stdout)
- Use `tea.Println()` for debugging output
- Test with `WithInput/WithOutput` for reproducibility
- Check message flow with filter function

Remember: **Bubbletea is architecture, Lipgloss is styling, Bubbles is components.**
