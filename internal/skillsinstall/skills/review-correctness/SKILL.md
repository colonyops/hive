---
name: review-correctness
description: Review code for correctness including logic bugs, error handling, edge cases, race conditions, and security vulnerabilities. Use when reviewing PRs or auditing code for bugs.
argument-hint: [file-or-directory]
---

# Correctness Review

Find bugs before users do. Evaluate logic, error handling, edge cases, concurrency, and security.

## Philosophy

**Assume the code has bugs. Prove it doesn't.**

- Every branch is a potential bug
- Every error path is a potential crash
- Every shared state is a potential race

## When to Use This Skill

- When reviewing a PR for functional correctness
- When auditing code after an incident
- When evaluating error handling and edge cases

## Correctness Criteria

### Check for:

1. **Logic errors** - Off-by-one, wrong operator, inverted condition, missing case
2. **Error handling** - Swallowed errors, missing context, panics in library code
3. **Edge cases** - Nil/zero/empty inputs, boundary values, integer overflow
4. **Concurrency** - Races, deadlocks, shared state without synchronization
5. **Security** - Injection, unvalidated input, hardcoded secrets, path traversal
6. **Resource leaks** - Unclosed files/connections, goroutine leaks, missing defers

## Evaluation Process

### Step 1: Trace the Happy Path

Walk through the intended flow. Verify:
- Inputs are validated before use
- Each transformation produces the expected result
- The return value matches the function contract

### Step 2: Break the Sad Paths

For each error/failure point:

| Check | What to Look For |
|-------|-----------------|
| Error propagation | Is the error wrapped with context? Or silently dropped? |
| Partial state | If step 3 of 5 fails, is state rolled back or left corrupt? |
| Caller expectations | Does the caller handle all documented error cases? |
| Panic safety | Could this panic in a goroutine without recovery? |

### Step 3: Probe Edge Cases

- What happens with nil, zero, empty string, empty slice?
- What happens at integer boundaries (0, -1, MaxInt)?
- What if the map key doesn't exist?
- What if the context is already cancelled?
- What if the input is valid but very large?

### Step 4: Concurrency Audit

For any shared mutable state:
- Is access synchronized (mutex, channel, atomic)?
- Can a goroutine outlive its parent and access freed resources?
- Are channels properly closed by the sender?
- Could `select` with `default` cause a busy loop?

### Step 5: Dead Code Analysis (Go projects only)

Run `deadcode` to find unreachable functions and methods:

```bash
# Install if needed
go install golang.org/x/tools/cmd/deadcode@latest

# Analyze from the main entry point(s)
deadcode -test ./...

# For a specific binary
deadcode ./cmd/myapp/...
```

Interpret findings critically — not all reported dead code is a bug:

| Finding | Likely Cause | Action |
|---------|-------------|--------|
| Unexported func, never called | Genuinely dead — remove | Delete |
| Exported func, never called internally | May be public API or used by external consumers | Investigate before removing |
| Interface method never called | Stale interface — check if interface is still needed | Remove method or interface |
| Function only called in deleted branch | Leftover from prior refactor | Delete |
| `//go:build` gated code | May be intentionally excluded in current build | Skip or verify |

Flag dead code as a correctness issue when it:
- Suggests a missing call site (e.g., a cleanup function that was supposed to be deferred)
- Indicates an unreachable error branch that should be reachable
- Points to orphaned logic after a refactor that left callers removed

### Step 6: Security Scan

| Category | Check |
|----------|-------|
| Input validation | User input validated and sanitized before use? |
| SQL/command injection | Using parameterized queries? Avoiding string interpolation? |
| Path traversal | User-controlled paths joined with `filepath.Join` and validated? |
| Secrets | No hardcoded tokens, passwords, or keys? |
| Crypto | Using `crypto/rand` not `math/rand` for security-sensitive values? |
| Auth | Permission checks on every protected endpoint? |

## Examples

### Swallowed Error

```go
// BUG: Error silently ignored -- caller thinks success
data, err := fetchData(url)
if err != nil {
    log.Println(err)
    return nil // should return error
}
```

### Race Condition

```go
// BUG: concurrent map read/write
go func() { cache[key] = value }()
go func() { v := cache[key] }()

// FIX: use sync.Map or mutex
```

### Missing Nil Check

```go
// BUG: user could be nil if not found
user, _ := store.FindByID(id)
fmt.Println(user.Name) // panic

// FIX: check error and nil
user, err := store.FindByID(id)
if err != nil { return fmt.Errorf("find user %s: %w", id, err) }
```

### Resource Leak

```go
// BUG: body never closed on early return
resp, err := http.Get(url)
if err != nil { return err }
if resp.StatusCode != 200 {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    // resp.Body leaked
}
defer resp.Body.Close()

// FIX: defer immediately after nil check
resp, err := http.Get(url)
if err != nil { return err }
defer resp.Body.Close()
```

## Review Checklist

When /review-correctness is invoked, evaluate:

1. **Error handling**: Are all errors propagated with context?
2. **Nil safety**: Are pointer/interface values checked before use?
3. **Edge cases**: Does the code handle zero, empty, and boundary inputs?
4. **Concurrency**: Is shared state properly synchronized?
5. **Resource management**: Are all resources cleaned up (defer, close)?
6. **Security**: Are inputs validated and outputs sanitized?
7. **Dead code**: Run `deadcode -test ./...` — are there unreachable functions signaling missing call sites or orphaned logic?

## Output Format

```markdown
## Correctness Review: [scope]

### Bugs Found
| Location | Severity | Issue | Fix |
|----------|----------|-------|-----|
| api.go:45 | Critical | Swallowed error in HandleCreate | Return error to caller |
| cache.go:12 | Critical | Race on map without mutex | Add sync.RWMutex |
| user.go:89 | Warning | Nil deref if user not found | Check err before accessing |

### Security Issues
| Location | Category | Issue | Fix |
|----------|----------|-------|-----|
| query.go:34 | Injection | String interpolation in SQL | Use parameterized query |

### Dead Code
| Function | Location | Assessment | Action |
|----------|----------|------------|--------|
| processLegacy | legacy.go:88 | Orphaned after refactor | Delete |
| validateToken | auth.go:201 | Exported — verify no external callers | Investigate |

### Edge Cases
| Location | Input | Behavior | Expected |
|----------|-------|----------|----------|
| parse.go:12 | empty string | panic | return ErrEmptyInput |

### Summary
[N] issues found: [X] critical, [Y] warnings. Top priority: [most important fix]
```

## Guidelines

1. **Every error must be handled** - Logging is not handling
2. **Nil is the billion-dollar mistake** - Check before you dereference
3. **Shared state needs synchronization** - No exceptions
4. **Close what you open** - defer immediately after acquisition
5. **Validate at the boundary** - Trust nothing from outside your package
6. **Wrap errors with context** - `fmt.Errorf("doing X: %w", err)` always
