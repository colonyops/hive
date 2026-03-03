---
name: review-design
description: Review code for design quality including API surface, naming, structure, separation of concerns, and adherence to project conventions. Use when reviewing PRs, refactoring, or evaluating architecture decisions.
argument-hint: [file-or-directory]
---

# Design Review

Evaluate code design: naming, structure, abstractions, and separation of concerns. Good design makes code easy to change.

## Philosophy

**Simple beats clever. Obvious beats implicit.**

- Code should be easy to delete, not easy to extend
- The best abstraction is often no abstraction
- Naming is design -- bad names signal bad boundaries

## When to Use This Skill

- When reviewing a PR for structural quality
- When evaluating whether an abstraction is justified
- When assessing API surface or public interfaces

## Design Criteria

### Good design exhibits:

1. **Clear boundaries** - Each package/module has a single, obvious responsibility
2. **Honest naming** - Names accurately describe what things do, not what they might do
3. **Minimal API surface** - Only expose what consumers need
4. **Direct dependencies** - Depend on concrete types, not unnecessary interfaces
5. **Obvious data flow** - Reader can trace how data moves without jumping between files

### Design smells:

1. **Premature abstraction** - Interface with one implementation, generic code used once
2. **Shotgun surgery** - One change requires edits across many files
3. **Feature envy** - Function mostly operates on another package's data
4. **God object** - One type/package knows too much or does too much
5. **Indirection without purpose** - Wrappers, factories, or registries that add complexity without value
6. **Naming lies** - `utils`, `helpers`, `common`, `base`, `manager` -- vague names hiding unclear responsibilities

## Evaluation Process

### Step 1: Assess Boundaries

For each new or modified package/type, ask:

| Question | Red Flag |
|----------|----------|
| Can I describe its purpose in one sentence? | Needs "and" to explain it |
| Does it depend on its consumers? | Circular or upward dependencies |
| Could I delete it without rewriting callers? | Tightly coupled to internals |
| Does it have a clear owner? | Shared mutable state across boundaries |

### Step 2: Evaluate Naming

- **Functions**: Should describe the action and imply the return. `ParseConfig` > `HandleConfig` > `DoConfig`
- **Types**: Should describe what it IS, not what it does. `User` > `UserManager`
- **Packages**: Should be short, singular nouns. `auth` > `authentication` > `authHelpers`
- **Variables**: Scope determines length. `i` in a 3-line loop is fine. `i` across 50 lines is not.

### Step 3: Check Abstraction Level

For each abstraction (interface, generic, factory):

1. **Count implementations** - One implementation means the interface is speculative
2. **Check call sites** - If callers always use the concrete type, the interface adds nothing
3. **Test-only interfaces** - Acceptable only if the real dependency is expensive (network, DB)

### Step 4: Evaluate Helper Functions

For any function named `helper`, placed in a `helpers`/`utils`/`common` package, or that exists solely to wrap another call, apply these tests:

| Question | Verdict |
|----------|---------|
| Is it called in more than one place? | Single call site — inline it |
| Does it have a name that describes *why* it exists, not *what* it does? | Vague name (`formatHelper`) — the abstraction has no identity |
| Does extracting it make the caller clearer, or just shorter? | Shorter-but-obscure — not worth it |
| Does it own any logic, or just delegate? | Pure delegation — remove the wrapper |
| Would a future reader know where to find this without searching? | Buried in `utils.go` — wrong location |

**Helper functions justify their existence when:**
- Called from 3+ distinct locations
- The extracted name communicates *intent* the call site cannot (`sanitizeUserInput` vs `strings.TrimSpace(strings.ToLower(...))`)
- They encapsulate a non-obvious decision (not just syntax compression)

**Inline when:** The helper is only called once, or its body is as readable as its name.

### Step 5: Review API Surface

- Are unexported things that should be? (Default to unexported)
- Do exported functions have clear input/output contracts?
- Are options/config types growing unbounded? (Consider builder or functional options)
- Could the API be misused easily? (Pit of success vs pit of despair)

## Examples

### Unnecessary Abstraction

```go
// BAD: Interface with one implementation, no external consumers
type UserRepository interface {
    GetUser(id string) (*User, error)
}
type userRepo struct{ db *sql.DB }

// GOOD: Just use the concrete type
type UserStore struct{ db *sql.DB }
func (s *UserStore) GetUser(id string) (*User, error) { ... }
```

### Naming That Communicates

```go
// BAD: What does "process" mean?
func ProcessOrder(o Order) error

// GOOD: Says exactly what happens
func ValidateAndSubmitOrder(o Order) error

// BETTER: Split into two clear functions
func ValidateOrder(o Order) error
func SubmitOrder(o Order) error
```

### Boundary Violation

```go
// BAD: HTTP handler knows about SQL
func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
    db.Exec("INSERT INTO users ...")
}

// GOOD: Handler delegates to domain logic
func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
    user := decodeUser(r)
    err := userStore.Create(user)
    // ...
}
```

## Review Checklist

When /review-design is invoked, evaluate:

1. **Boundaries**: Does each unit have a clear, single responsibility?
2. **Naming**: Do names honestly describe what things do?
3. **Abstractions**: Is every interface/generic justified by multiple consumers?
4. **Helpers**: Is every helper called from multiple sites, named for intent, and non-trivially useful?
5. **API surface**: Are exports minimal and hard to misuse?
6. **Data flow**: Can you trace data without a debugger?
7. **Coupling**: Could you change one module without cascading edits?

## Output Format

```markdown
## Design Review: [scope]

### Findings
| Location | Issue | Severity | Recommendation |
|----------|-------|----------|----------------|
| pkg/auth | God package - handles auth, sessions, and permissions | Critical | Split into auth, session, permission packages |
| UserManager | Vague name | Suggestion | Rename to UserStore or UserService based on actual role |
| IProcessor | Single implementation | Suggestion | Remove interface, use concrete ProcessorImpl directly |

### Positive Patterns
- [Note any good design decisions worth calling out]

### Summary
[Brief assessment of overall design quality and top priority changes]
```

## Guidelines

1. **Delete over deprecate** - Remove dead abstractions entirely
2. **Concrete over abstract** - Interfaces should be discovered, not predicted
3. **Small over clever** - A 10-line function beats a 3-line function nobody understands
4. **Flat over nested** - Prefer early returns; deep nesting hides logic
5. **Explicit over implicit** - Direct function calls over service locators or magic registration
