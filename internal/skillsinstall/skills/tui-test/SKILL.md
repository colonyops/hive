---
name: tui-test
description: >
  Specialized guidance for testing TUI applications with Bubbletea, including
  golden file testing, component testing, and integration testing with teatest.
---

# TUI Testing Expert

Specialized guidance for testing Terminal User Interface applications with Bubbletea, focusing on golden file testing, component testing, and integration testing.

## When to Use This Skill

Activate when:

- Writing tests for TUI applications
- Setting up golden file testing
- Testing rendering output
- Testing state transitions
- Debugging test failures
- Implementing test fixtures

## Testing Strategy

### 1. Golden File Testing (Visual Regression)

**Primary method for testing TUI rendering:**

```go
package myapp

import (
    "testing"
    "github.com/charmbracelet/x/exp/golden"
)

func TestRender(t *testing.T) {
    m := Model{
        title: "Test App",
        width: 80,
        items: []string{"one", "two", "three"},
    }

    output := m.View()

    // Compares with testdata/TestRender.golden
    golden.RequireEqual(t, output)
}

// Run tests: go test
// Update golden files: go test -update
```

**Directory structure:**

```
myapp/
├── component.go
├── component_test.go
└── testdata/
    ├── TestRender.golden
    ├── TestRenderEmpty.golden
    └── TestRenderWithScroll.golden
```

**Best practices:**

- One golden file per test case
- Use descriptive test names (becomes filename)
- Update golden files when intentionally changing UI
- Review diffs carefully before updating
- Strip ANSI codes for comparison if needed

### 2. Testing Update Functions

**Test state transitions directly:**

```go
func TestUpdateNavigation(t *testing.T) {
    tests := []struct {
        name         string
        initialModel Model
        msg          tea.Msg
        wantCursor   int
        wantCmd      bool
    }{
        {
            name:         "down arrow increases cursor",
            initialModel: Model{cursor: 0, items: []string{"a", "b"}},
            msg:          tea.KeyPressMsg{Type: tea.KeyDown},
            wantCursor:   1,
            wantCmd:      false,
        },
        {
            name:         "down at bottom wraps to top",
            initialModel: Model{cursor: 1, items: []string{"a", "b"}},
            msg:          tea.KeyPressMsg{Type: tea.KeyDown},
            wantCursor:   0,
            wantCmd:      false,
        },
        {
            name:         "enter selects item",
            initialModel: Model{cursor: 0, items: []string{"a", "b"}},
            msg:          tea.KeyPressMsg{Type: tea.KeyEnter},
            wantCursor:   0,
            wantCmd:      true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m := tt.initialModel
            newModel, cmd := m.Update(tt.msg)

            got := newModel.(Model)

            if got.cursor != tt.wantCursor {
                t.Errorf("cursor = %v, want %v", got.cursor, tt.wantCursor)
            }

            if (cmd != nil) != tt.wantCmd {
                t.Errorf("cmd = %v, wantCmd = %v", cmd != nil, tt.wantCmd)
            }
        })
    }
}
```

### 3. Testing Components in Isolation

```go
func TestTextInputComponent(t *testing.T) {
    ti := textinput.New()
    ti.SetValue("initial")
    ti.Focus()

    // Test typing
    ti, _ = ti.Update(tea.KeyPressMsg{
        Runes: []rune{'x'},
        Type:  tea.KeyRunes,
    })

    if ti.Value() != "initialx" {
        t.Errorf("value = %q, want %q", ti.Value(), "initialx")
    }

    // Test backspace
    ti, _ = ti.Update(tea.KeyPressMsg{Type: tea.KeyBackspace})

    if ti.Value() != "initial" {
        t.Errorf("value = %q, want %q", ti.Value(), "initial")
    }

    // Test view contains expected text
    view := ti.View()
    if !strings.Contains(view, "initial") {
        t.Error("view doesn't contain expected text")
    }
}
```

### 4. Integration Testing with teatest

```go
import (
    "testing"
    "time"
    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/x/exp/teatest"
)

func TestFullProgram(t *testing.T) {
    m := NewModel()

    tm := teatest.NewTestModel(
        t, m,
        teatest.WithInitialTermSize(80, 24),
    )

    t.Cleanup(func() {
        if err := tm.Quit(); err != nil {
            t.Fatal(err)
        }
    })

    // Wait for initialization
    time.Sleep(100 * time.Millisecond)

    // Simulate user input
    tm.Type("hello world")
    tm.Send(tea.KeyPressMsg{Type: tea.KeyEnter})

    // Wait for specific output
    teatest.WaitFor(
        t,
        tm.Output(),
        func(bts []byte) bool {
            return bytes.Contains(bts, []byte("Success"))
        },
        teatest.WithCheckInterval(50*time.Millisecond),
        teatest.WithDuration(3*time.Second),
    )

    // Get final output
    output := tm.FinalOutput(t)
    golden.RequireEqual(t, output)

    // Verify final model state
    fm := tm.FinalModel(t)
    finalModel, ok := fm.(Model)
    if !ok {
        t.Fatal("wrong model type")
    }

    if finalModel.state != stateComplete {
        t.Errorf("state = %v, want %v", finalModel.state, stateComplete)
    }
}
```

### 5. Mock I/O Testing

```go
func TestProgramWithMockIO(t *testing.T) {
    var output bytes.Buffer
    var input bytes.Buffer

    // Pre-fill input
    input.WriteString("test input\n")
    input.WriteByte('q')

    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()

    m := NewModel()
    p := tea.NewProgram(m,
        tea.WithContext(ctx),
        tea.WithInput(&input),
        tea.WithOutput(&output),
    )

    finalModel, err := p.Run()
    if err != nil {
        t.Fatal(err)
    }

    // Verify output
    if output.Len() == 0 {
        t.Error("no output produced")
    }

    // Verify final state
    fm, ok := finalModel.(Model)
    if !ok {
        t.Fatal("wrong model type")
    }

    if fm.value != "test input" {
        t.Errorf("value = %q, want %q", fm.value, "test input")
    }
}
```

## Test Helpers

### Key Press Helper

```go
func keyPress(key rune) tea.Msg {
    return tea.KeyPressMsg{
        Runes: []rune{key},
        Type:  tea.KeyRunes,
    }
}

func keyPressString(s string) tea.Msg {
    return tea.KeyPressMsg{
        Runes: []rune(s),
        Type:  tea.KeyRunes,
    }
}

func keyDown() tea.Msg {
    return tea.KeyPressMsg{Type: tea.KeyDown}
}

func keyEnter() tea.Msg {
    return tea.KeyPressMsg{Type: tea.KeyEnter}
}
```

### String Input Helper

```go
func sendString(m Model, s string) Model {
    for _, r := range s {
        m, _ = m.Update(keyPress(r))
    }
    return m
}
```

### ANSI Strip Helper

```go
import "github.com/charmbracelet/x/ansi"

func stripANSI(s string) string {
    s = ansi.Strip(s)
    lines := strings.Split(s, "\n")
    var result []string
    for _, line := range lines {
        trimmed := strings.TrimRight(line, " ")
        if trimmed != "" {
            result = append(result, trimmed)
        }
    }
    return strings.Join(result, "\n")
}
```

### Window Size Helper

```go
func windowSize(w, h int) tea.WindowSizeMsg {
    return tea.WindowSizeMsg{Width: w, Height: h}
}
```

## Table-Driven View Tests

```go
func TestView(t *testing.T) {
    tests := []struct {
        name     string
        setup    func(Model) Model
        wantView string
    }{
        {
            name: "empty state",
            wantView: heredoc.Doc(`
                > No items
                >
            `),
        },
        {
            name: "with items",
            setup: func(m Model) Model {
                m.items = []string{"one", "two"}
                return m
            },
            wantView: heredoc.Doc(`
                > one
                > two
                >
            `),
        },
        {
            name: "with cursor",
            setup: func(m Model) Model {
                m.items = []string{"one", "two"}
                m.cursor = 1
                return m
            },
            wantView: heredoc.Doc(`
                >   one
                > ❯ two
                >
            `),
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m := NewModel()
            if tt.setup != nil {
                m = tt.setup(m)
            }

            got := stripANSI(m.View())
            want := stripANSI(tt.wantView)

            if got != want {
                t.Errorf("View() =\n%v\nwant\n%v", got, want)
            }
        })
    }
}
```

## Testing Checklist

**Unit Tests:**

- [ ] Test Init returns expected commands
- [ ] Test Update with all message types
- [ ] Test state transitions
- [ ] Test cursor/selection logic
- [ ] Test validation/error handling
- [ ] Test edge cases (empty, full, boundaries)

**View Tests:**

- [ ] Golden file for default state
- [ ] Golden file for each significant state
- [ ] Test with different widths
- [ ] Test with different heights
- [ ] Test overflow/truncation

**Integration Tests:**

- [ ] Test full user flows
- [ ] Test keyboard navigation
- [ ] Test window resize
- [ ] Test with timeouts
- [ ] Test cancellation

**Component Tests:**

- [ ] Test focus/blur
- [ ] Test each public method
- [ ] Test with sub-components
- [ ] Test message delegation

## Common Test Patterns

### Test with Multiple Updates

```go
func TestSequentialUpdates(t *testing.T) {
    m := NewModel()

    // Apply sequence of updates
    updates := []tea.Msg{
        tea.KeyPressMsg{Type: tea.KeyDown},
        tea.KeyPressMsg{Type: tea.KeyDown},
        tea.KeyPressMsg{Type: tea.KeyEnter},
    }

    for _, msg := range updates {
        m, _ = m.(Model).Update(msg)
    }

    if m.(Model).cursor != 2 {
        t.Errorf("cursor = %v, want 2", m.(Model).cursor)
    }
}
```

### Test Command Execution

```go
func TestCommandExecution(t *testing.T) {
    m := NewModel()

    _, cmd := m.Update(tea.KeyPressMsg{Type: tea.KeyEnter})

    if cmd == nil {
        t.Fatal("expected command, got nil")
    }

    // Execute command
    msg := cmd()

    // Verify message type
    if _, ok := msg.(dataLoadedMsg); !ok {
        t.Errorf("expected dataLoadedMsg, got %T", msg)
    }
}
```

### Test Focus State

```go
func TestFocusHandling(t *testing.T) {
    m := NewModel()

    if m.Focused() {
        t.Error("should not be focused initially")
    }

    m.Focus()

    if !m.Focused() {
        t.Error("should be focused after Focus()")
    }

    // Should handle input when focused
    m, _ = m.Update(keyPress('a'))
    if m.value != "a" {
        t.Error("should accept input when focused")
    }

    m.Blur()

    if m.Focused() {
        t.Error("should not be focused after Blur()")
    }

    // Should ignore input when not focused
    originalValue := m.value
    m, _ = m.Update(keyPress('b'))
    if m.value != originalValue {
        t.Error("should ignore input when not focused")
    }
}
```

## Debugging Test Failures

**Golden file mismatch:**

1. Check if change was intentional
2. Review diff output carefully
3. Update with `go test -update` if correct
4. Never blindly update without reviewing

**Flaky tests:**

1. Use deterministic input (no time.Now(), rand.Intn())
2. Set fixed window size
3. Mock external dependencies
4. Use context timeouts

**Missing output:**

1. Check if renderer is disabled
2. Verify output buffer is captured
3. Check if view returns empty string

## Best Practices

1. **Test behavior, not implementation**
   - Don't test internal state unless necessary
   - Test observable behavior (view output, commands returned)

2. **Use descriptive test names**
   - Name describes what is being tested
   - Name becomes golden filename

3. **Keep tests fast**
   - Use direct Update calls over full programs
   - Mock slow operations
   - Parallelize independent tests

4. **Make tests deterministic**
   - No random values
   - Fixed window sizes
   - Mock time-dependent operations

5. **Test error paths**
   - Invalid input
   - Network failures
   - Cancellation
   - Edge cases

6. **Use table-driven tests**
   - More test cases with less code
   - Easy to add new cases
   - Clear test structure
