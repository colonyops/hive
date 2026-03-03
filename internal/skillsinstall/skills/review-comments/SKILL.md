---
name: review-comments
description: Review code comments for relevance and purpose. Identifies comments that restate what code does instead of explaining why. Use when reviewing PRs, refactoring, or when the user asks to evaluate comment quality.
argument-hint: [file-or-directory]
---

# Comment Review - Why Over What

Evaluate comments for purpose and relevance. Every comment should earn its place by adding context the code cannot express on its own.

## Philosophy

**Assume the reader can read code.** Comments exist to capture intent, constraints, trade-offs, and non-obvious context that code alone cannot convey.

- A comment that restates code is noise
- A comment that explains _why_ a decision was made is documentation
- No comment is better than a misleading or stale comment

## When to Use This Skill

- When reviewing a PR that adds or modifies comments
- When refactoring and deciding which comments to keep
- When auditing a file or module for comment quality

## Comment Value Criteria

### A comment provides value when it:

1. **Explains why** - Captures intent, motivation, or constraints behind a decision
2. **Warns about non-obvious behavior** - Edge cases, gotchas, or surprising side effects
3. **Provides context the code cannot** - Links to specs, tickets, or external requirements
4. **Clarifies unintuitive logic** - Only when the code genuinely cannot be made clearer through naming or structure

### A comment does NOT provide value when it:

1. **Restates the code** - `// increment counter` above `counter++`
2. **Describes what is obvious** - `// loop through users` above `for _, user := range users`
3. **Repeats the function/variable name** - `// GetUser gets a user`
4. **Is stale or wrong** - Describes behavior that no longer matches the code
5. **Compensates for bad naming** - The fix is renaming, not commenting

## Evaluation Process

### Step 1: Classify Each Comment

For every comment in scope, classify it:

| Category | Keep? | Example |
|----------|-------|---------|
| **Why** - Explains reasoning or constraints | Yes | `// Use insertion sort here; n is always < 10 and cache locality matters` |
| **Warning** - Flags non-obvious risk | Yes | `// This must run before InitDB; order matters due to schema migration lock` |
| **Context** - Links to external info | Yes | `// Per RFC 7231 §6.5.1, return 400 for malformed request bodies` |
| **What** - Restates code | No | `// Create a new HTTP client` |
| **Filler** - Noise or decoration | No | `// ====== HELPERS ======` |
| **Stale** - No longer accurate | No | `// Returns nil if not found` (but code now returns an error) |

### Step 2: Evaluate "What" Comments

Some "what" comments deserve a second look. Keep them only if:

- The code is genuinely unintuitive (bit manipulation, complex regex, unusual algorithms)
- Renaming or restructuring cannot make the code self-explanatory
- Domain knowledge is required that a general developer would not have

### Step 3: Check for Missing Comments

Good code sometimes _lacks_ comments where they would help:

- Non-obvious performance choices
- Workarounds for bugs in dependencies
- Business rules that drive control flow
- Concurrency or ordering constraints
- Magic numbers or thresholds

## Examples

### Good Comments

```go
// We retry 3 times because the upstream billing API intermittently
// returns 503 during their rolling deploys (see INCIDENT-1234).
resp, err := retryRequest(billingURL, 3)
```

```go
// Sorting by CreatedAt descending so the UI shows newest first.
// The API contract guarantees this order (see docs/api-spec.md).
sort.Slice(items, func(i, j int) bool {
    return items[i].CreatedAt.After(items[j].CreatedAt)
})
```

```go
// Must hold mu before calling; callers are responsible for locking.
func (c *Cache) evictOldest() { ... }
```

### Bad Comments

```go
// Create a new server
srv := &http.Server{Addr: ":8080"}
```

```go
// GetUserByID gets a user by ID
func GetUserByID(id string) (*User, error) { ... }
```

```go
// Loop through all items and check if they are valid
for _, item := range items {
    if !item.Valid() { ... }
}
```

```go
// Set the timeout to 30 seconds
client.Timeout = 30 * time.Second
// Better: explain WHY 30 seconds
// 30s matches the upstream gateway timeout; going higher causes
// the LB to kill the connection before we get a response.
client.Timeout = 30 * time.Second
```

## Review Checklist

When /review-comments is invoked, evaluate:

1. **Purpose**: Does each comment explain why, not what?
2. **Accuracy**: Do comments match current code behavior?
3. **Necessity**: Could the comment be removed by improving naming or structure?
4. **Gaps**: Are there non-obvious decisions missing a comment?
5. **Staleness**: Do any comments reference outdated behavior?

## Output Format

```markdown
## Comment Review: [file/scope]

### Issues Found
| Location | Type | Comment | Recommendation |
|----------|------|---------|----------------|
| file.go:23 | What | `// Create new client` | Remove - obvious from code |
| file.go:45 | Stale | `// Returns nil on error` | Update or remove - code returns error |
| file.go:67 | What | `// Parse the config` | Remove - rename func to `ParseConfig` |

### Missing Comments
| Location | Suggestion |
|----------|-----------|
| file.go:89 | Explain why retry count is 5 |
| file.go:112 | Document concurrency constraint on `mu` |

### Summary
[N] comments reviewed → [X] remove, [Y] update, [Z] missing comments suggested
```

## Guidelines

1. **Why > What** - If a comment only says what, it should say why or be deleted
2. **Fix the code first** - If a comment compensates for bad naming, rename instead
3. **Stale comments are bugs** - Wrong documentation is worse than none
4. **Brevity matters** - Comments should be concise; a paragraph is usually too much
5. **Not every line needs a comment** - Well-written code with good names is self-documenting
6. **Godoc is different** - Exported symbol documentation follows its own rules and is expected
