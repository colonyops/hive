---
name: review-tests
description: Evaluate test quality and coverage to ensure tests provide value without redundancy
argument-hint: [file-or-function]
---

# Test Review - Quality Over Quantity

Evaluate tests for coverage quality, redundancy, and value. Every test should justify its existence.

## Philosophy

**The goal is confidence, not coverage percentage.**

- A test that exercises the same code path as another test adds maintenance burden without value
- Tests should document behavior, not implementation details
- Fewer high-quality tests beat many low-quality tests

## When to Use This Skill

- Before writing new tests for a function/module
- When reviewing a PR that adds tests
- When tests are failing and you're unsure if they're worth fixing
- When refactoring and deciding which tests to keep

## Test Value Criteria

### A test provides value when it:

1. **Covers a distinct execution path** - Different branch, error condition, or state
2. **Documents expected behavior** - Someone can read it to understand requirements
3. **Catches real bugs** - Would fail if the code breaks in a meaningful way
4. **Is maintainable** - Doesn't break on unrelated changes

### A test does NOT provide value when it:

1. **Duplicates another test's path** - Same branches, same assertions, different inputs
2. **Tests implementation details** - Breaks when refactoring without behavior change
3. **Has flaky results** - Sometimes passes, sometimes fails
4. **Tests framework/library code** - Verifying that Go's `append` works

## Evaluation Process

When reviewing tests, start by collecting coverage data, then analyze each test case:

### Step 0: Collect Coverage Data (Go projects only)

Before analyzing tests manually, run the coverage tool to get objective data:

```bash
# Run with coverage profiling
go test ./... -coverprofile=coverage.out -covermode=atomic

# Per-function coverage breakdown
go tool cover -func=coverage.out

# For a specific package
go test ./path/to/pkg/... -coverprofile=coverage.out && go tool cover -func=coverage.out
```

Use the coverage output to:
- **Identify uncovered functions** - 0% coverage functions are gaps
- **Find partially covered functions** - low % signals untested branches
- **Validate existing tests** - high % with redundant tests confirms candidates for removal
- **Focus review effort** - prioritize functions with coverage below 70%

Interpret coverage numbers critically — 100% line coverage does not mean all logical paths are tested (e.g., different error values hitting the same return statement). Use coverage as a floor check, not a ceiling.

### Step 1: Map Execution Paths

```
Function: ProcessOrder(order Order) error
Paths:
  1. order.Items is empty → return ErrEmptyOrder
  2. order.Total < 0 → return ErrInvalidTotal
  3. order.Customer is nil → return ErrNoCustomer
  4. happy path → process and return nil
```

### Step 2: Map Test Cases to Paths

```
TestProcessOrder_EmptyItems     → Path 1 ✓
TestProcessOrder_NoItems        → Path 1 ✗ REDUNDANT
TestProcessOrder_ZeroItems      → Path 1 ✗ REDUNDANT
TestProcessOrder_NegativeTotal  → Path 2 ✓
TestProcessOrder_NilCustomer    → Path 3 ✓
TestProcessOrder_Success        → Path 4 ✓
```

### Step 3: Identify Gaps and Redundancy

- **Redundant**: Multiple tests covering Path 1 with trivially different inputs
- **Gap**: No test for Path 2 with exactly zero total (boundary)
- **Action**: Remove redundant tests, add boundary test if meaningful

## Table Tests: When and How

### Use table tests when:
- Testing the same function with multiple valid inputs
- Each case exercises a DIFFERENT path or boundary
- Cases are truly independent

### Structure table tests by path:

```go
func TestProcessOrder(t *testing.T) {
    tests := []struct {
        name    string
        order   Order
        wantErr error
    }{
        // Error paths - one per distinct error condition
        {"empty items", Order{Items: nil}, ErrEmptyOrder},
        {"negative total", Order{Items: items, Total: -1}, ErrInvalidTotal},
        {"nil customer", Order{Items: items, Customer: nil}, ErrNoCustomer},

        // Happy path - minimal case that succeeds
        {"valid order", validOrder(), nil},
    }
    // ...
}
```

### Anti-pattern: Redundant table entries

```go
// BAD: These all test the same path (empty items)
{"nil items", Order{Items: nil}, ErrEmptyOrder},
{"empty slice", Order{Items: []Item{}}, ErrEmptyOrder},
{"zero length", Order{Items: make([]Item, 0)}, ErrEmptyOrder},
```

## Boundary Testing

Test boundaries that matter:

| Boundary | Test if... |
|----------|-----------|
| Zero/Empty | Code has explicit zero handling |
| One element | Code has single-element special case |
| Max value | Code has upper limit logic |
| Nil vs Empty | Code distinguishes between them |

**Don't test boundaries that don't exist in the code.**

## Integration vs Unit Tests

### Unit tests should:
- Test a single function/method in isolation
- Mock external dependencies
- Run fast (< 100ms)
- Be deterministic

### Integration tests should:
- Test component interactions
- Use real dependencies where practical
- Cover critical user flows
- Be clearly marked (separate file, build tag)

**One integration test covering a flow > many unit tests mocking everything**

## Red Flags in Test Code

### 1. Testing the mock

```go
// BAD: This tests that mockDB.Get returns what we told it to
mockDB.On("Get", "key").Return("value", nil)
result, _ := mockDB.Get("key")
assert.Equal(t, "value", result) // Useless!
```

### 2. Assertion-heavy tests

```go
// BAD: Testing implementation details
assert.Equal(t, 3, len(result.calls))
assert.Equal(t, "init", result.calls[0])
assert.Equal(t, "process", result.calls[1])
assert.Equal(t, "cleanup", result.calls[2])

// GOOD: Testing behavior
assert.NoError(t, result.Error())
assert.True(t, result.Completed())
```

### 3. Fragile setup

```go
// BAD: Test breaks if any field is added to Config
config := Config{Field1: "a", Field2: "b", Field3: "c", ...}

// GOOD: Only set what matters for this test
config := Config{RelevantField: "value"}
```

### 4. Time-dependent tests

```go
// BAD: Flaky
time.Sleep(100 * time.Millisecond)
assert.True(t, completed)

// GOOD: Synchronize properly
<-done
assert.True(t, completed)
```

**Go 1.24+: Use `testing/synctest` for concurrent code.**

The `testing/synctest` package (experimental, requires `GOEXPERIMENT=synctest`) provides
deterministic testing of concurrent code without `time.Sleep`. It runs goroutines in an
isolated "bubble" with a fake clock. `synctest.Wait()` blocks until all goroutines in the
bubble are durably blocked, giving you a reliable synchronization point.

```go
// BAD: Slow and flaky — time.Sleep is never the right synchronization
time.Sleep(100 * time.Millisecond)
if !called {
    t.Fatal("expected function to be called")
}

// GOOD: Deterministic — synctest.Wait() returns when all goroutines block
synctest.Run(func() {
    called := false
    go func() { called = true }()
    synctest.Wait()
    if !called {
        t.Fatal("expected function to be called")
    }
})
```

Key rules for `synctest`:
- Wrap test body in `synctest.Run(func() { ... })`
- Call `synctest.Wait()` before assertions to ensure goroutines have settled
- `time.Sleep` inside a bubble advances the fake clock instantly — useful for testing timeouts
- All goroutines in the bubble must exit before `Run` returns; clean up background goroutines
- Use `net.Pipe` for in-memory network connections (real I/O is not durably blocking)

See https://go.dev/blog/synctest for details.

## Review Checklist

When /review-tests is invoked, evaluate:

1. **Coverage baseline**: Run `go test -coverprofile` and check per-function percentages first
2. **Path coverage**: Does each test cover a distinct execution path?
3. **Redundancy**: Are there multiple tests covering the same path?
4. **Boundaries**: Are tested boundaries actually present in the code?
5. **Maintainability**: Will these tests break on refactors?
6. **Clarity**: Can someone understand the requirements from the tests?
7. **Speed**: Are unit tests fast? Are slow tests justified?

## Output Format

When reviewing tests, provide:

```markdown
## Test Review: [function/file]

### Coverage Report
| Function | Coverage | Status |
|----------|----------|--------|
| FuncA    | 87.5%    | ✓ Good |
| FuncB    | 33.3%    | ⚠ Low  |
| FuncC    | 0.0%     | ✗ Gap  |

### Execution Paths Identified
1. [path description]
2. [path description]
...

### Coverage Analysis
| Test Case | Path | Status |
|-----------|------|--------|
| TestX | 1 | ✓ Covers |
| TestY | 1 | ✗ Redundant with TestX |
| - | 3 | ✗ Gap - no coverage |

### Recommendations
- Remove: [TestY] - duplicates TestX
- Add: Test for [uncovered path]
- Refactor: [TestZ] tests implementation, not behavior

### Summary
Coverage: X% overall | [N] tests → [M] recommended (removed X redundant, added Y for gaps)
```

## Guidelines

1. **One test per path** - Additional tests for the same path rarely add value
2. **Test behavior, not implementation** - Tests should survive refactors
3. **Boundaries only when meaningful** - Don't test boundaries the code doesn't have
4. **Integration over excessive mocking** - Real interactions catch real bugs
5. **Delete flaky tests** - A flaky test is worse than no test
6. **Readability matters** - Tests are documentation
