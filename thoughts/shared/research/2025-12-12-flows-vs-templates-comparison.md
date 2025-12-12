---
date: 2025-12-12T00:00:00Z
researcher: Claude
git_commit: 453046ca522684930a9377a5f360b311c5ad3a74
branch: main
repository: hay-kot/hive
topic: "Feature Comparison: Flows vs Session Templates"
tags: [research, comparison, flows, templates, design-decision]
status: complete
last_updated: 2025-12-12
last_updated_by: Claude
---

# Feature Comparison: Flows vs Session Templates

**Date**: 2025-12-12
**Researcher**: Claude

## Executive Summary

This document compares two potential features for hive:

| Feature                   | Flows                                         | Session Templates                         |
| ------------------------- | --------------------------------------------- | ----------------------------------------- |
| **Core Purpose**          | Multi-step automated workflows                | Structured prompt generation for sessions |
| **Complexity**            | High (new subsystems)                         | Low (extends existing)                    |
| **User Interaction**      | Fire-and-forget automation                    | Interactive Q&A before session            |
| **Implementation Effort** | 6+ new components                             | 2-3 component modifications               |
| **Risk**                  | High (background processes, state management) | Low (builds on proven patterns)           |

**Recommendation**: Start with Session Templates. They solve an immediate UX problem with minimal complexity and provide a foundation that Flows could later build upon.

---

## Feature Definitions

### Option A: Flows (from original research)

Multi-step automated workflows that:

- Define sequences of shell and Claude Code steps
- Run in foreground or background
- Capture outputs as reviewable "assets"
- Manage their own process lifecycle

```yaml
# Example Flow
flows:
  pr-review:
    args:
      - name: pr_number
        required: true
    steps:
      - name: fetch-pr
        type: shell
        command: "gh pr checkout {{ .Args.pr_number }}"
      - name: review
        type: claude
        prompt: "Review PR #{{ .Args.pr_number }}"
    output:
      asset:
        type: pr-review
```

### Option B: Session Templates

Structured forms that collect user input and render a prompt template:

- Define Q&A forms with typed fields
- Render answers into prompt templates
- Create sessions with the generated prompt
- No new execution model - uses existing spawn flow

```yaml
# Example Template
templates:
  pr-review:
    description: "Review a GitHub PR"
    fields:
      - name: pr_number
        label: "PR Number"
        type: string
        required: true
      - name: focus_areas
        label: "Focus Areas"
        type: multi-select
        options:
          - security
          - performance
          - code-style
    prompt: |
      Review PR #{{ .pr_number }}.
      Focus on: {{ .focus_areas | join ", " }}
```

---

## Architectural Comparison

### Current Architecture (Baseline)

```
User Input (name, prompt) → CreateSession → Hooks → Spawn Commands
                                ↓
                           Session Store
```

### Option A: Flows Architecture

```
Flow Definition → FlowRunner → StepRunner → [ShellStep | ClaudeStep]
      ↓              ↓             ↓
  FlowConfig    FlowState      Output Capture
      ↓              ↓             ↓
  Validation    PID/Logs       Asset Store
                    ↓
              Background Daemon
```

**New Components Required:**

1. `FlowConfig` - YAML parser for flow definitions
2. `FlowRunner` - Orchestrates multi-step execution
3. `StepRunner` - Executes individual steps
4. `ShellStep` - Shell command executor
5. `ClaudeStep` - Claude CLI wrapper with options
6. `FlowStateStore` - Tracks running/completed flows
7. `AssetStore` - Persists flow outputs
8. `BackgroundManager` - Daemon process handling
9. `flow` CLI subcommand tree
10. TUI flow panel (optional)

### Option B: Session Templates Architecture

```
Template Definition → Form Renderer → Prompt Template → CreateSession (existing)
        ↓                  ↓                ↓
    Validation        User Input       Template Engine (existing)
```

**New Components Required:**

1. `TemplateConfig` - YAML parser for template definitions
2. `FormRenderer` - Generate `huh` forms from config
3. `template` CLI subcommand (or extend `new`)

**Modified Components:**

1. `config.go` - Add `templates` section
2. `cmd_new.go` - Add `--template` flag

---

## Implementation Complexity

### Flows: High Complexity

| Component                | Complexity | Risk                               |
| ------------------------ | ---------- | ---------------------------------- |
| Multi-step orchestration | Medium     | Step failure handling, rollback    |
| Background execution     | High       | Process management, zombie cleanup |
| Asset storage            | Medium     | Schema design, retention policy    |
| Claude CLI integration   | Medium     | Flag mapping, output parsing       |
| State management         | High       | Concurrent access, crash recovery  |
| TUI integration          | Medium     | New panel, state synchronization   |

**Total Estimate**: 1500-2500 lines of new code

**Key Risks:**

- Background process management in Go is non-trivial
- State consistency between foreground/background modes
- Error handling across multiple step types
- Testing automation without hitting Claude API

### Session Templates: Low Complexity

| Component               | Complexity | Risk                     |
| ----------------------- | ---------- | ------------------------ |
| Template config parsing | Low        | Existing YAML patterns   |
| Form generation         | Low        | `huh` library handles UI |
| Prompt rendering        | Low        | Existing `tmpl.Render()` |
| CLI integration         | Low        | Single flag addition     |

**Total Estimate**: 300-500 lines of new code

**Key Risks:**

- Form field type coverage (text, select, multi-select)
- Template validation (missing variables)
- Edge cases in prompt escaping

---

## User Experience Comparison

### Flows UX

```bash
# Define flow once in config
$ vim ~/.config/hive/config.yaml

# Run flow
$ hive flow pr-review 523

# Check status (if background)
$ hive flow status

# View output
$ hive assets view abc123
```

**Pros:**

- Fire-and-forget automation
- Capture output for later review
- Composable multi-step operations

**Cons:**

- Learning curve for flow definition syntax
- Mental model shift from interactive to batch
- Debugging failed flows requires log inspection

### Session Templates UX

```bash
# Define template once in config
$ vim ~/.config/hive/config.yaml

# Use template (interactive form appears)
$ hive new --template pr-review

┌─────────────────────────────────────┐
│ PR Review                           │
├─────────────────────────────────────┤
│ PR Number: [523_____________]       │
│                                     │
│ Focus Areas:                        │
│ [x] security                        │
│ [ ] performance                     │
│ [x] code-style                      │
│                                     │
│ [Submit]  [Cancel]                  │
└─────────────────────────────────────┘

# Session created with rendered prompt, user interacts normally
```

**Pros:**

- Familiar session workflow
- Interactive, can adjust before committing
- No new mental model required
- User stays in control of Claude interaction

**Cons:**

- Still requires user attention during execution
- No output capture (session transcript is the output)
- Single-step only (prompt generation, not workflow)

---

## Use Case Analysis

### Use Cases Better Served by Flows

| Use Case                    | Why Flows                 |
| --------------------------- | ------------------------- |
| Scheduled batch jobs        | Background execution      |
| CI/CD integration           | Headless, exit codes      |
| Long-running tasks (>30min) | Don't need terminal open  |
| Multi-repo operations       | Sequential step execution |
| Output archival             | Asset capture             |

### Use Cases Better Served by Templates

| Use Case                    | Why Templates                                    |
| --------------------------- | ------------------------------------------------ |
| Consistent PR reviews       | Structured prompt, interactive review            |
| Onboarding new repos        | Form guides required info                        |
| Team prompt standards       | Share templates, ensure consistency              |
| Complex prompt construction | Multi-field forms easier than remembering syntax |
| Exploratory work            | User guides Claude interactively                 |

### Use Cases Served by Either

| Use Case                 | Notes                                                |
| ------------------------ | ---------------------------------------------------- |
| Code review              | Templates: interactive review; Flows: batch capture  |
| Bug investigation        | Templates: guided questions; Flows: automated triage |
| Documentation generation | Either works well                                    |

---

## Integration with Existing Architecture

### Flows Integration Points

| Existing Component | Integration Needed                              |
| ------------------ | ----------------------------------------------- |
| Session Store      | New flow may create session, or run in existing |
| Template System    | Reuse for flow step templating                  |
| Hooks              | Unclear - do flows run hooks?                   |
| Spawn Commands     | Not used - flows have own execution             |
| TUI                | New panel required                              |
| Config             | New top-level `flows` section                   |

**Concern**: Flows introduce a parallel execution model that doesn't naturally compose with the existing session-centric architecture.

### Templates Integration Points

| Existing Component | Integration Needed                     |
| ------------------ | -------------------------------------- |
| Session Store      | None - uses existing CreateSession     |
| Template System    | Extend with form variable rendering    |
| Hooks              | None - hooks run as normal             |
| Spawn Commands     | None - prompt passed through SpawnData |
| TUI                | Optional - could add template picker   |
| Config             | New `templates` section                |

**Advantage**: Templates slot cleanly into the existing flow: `Template Form → Rendered Prompt → CreateSession → Hooks → Spawn`

---

## Path Forward: Templates as Foundation

Session Templates can serve as a stepping stone to Flows:

### Phase 1: Session Templates

```yaml
templates:
  pr-review:
    fields:
      - name: pr_number
        type: string
    prompt: "Review PR #{{ .pr_number }}"
```

### Phase 2: Templates with Defaults/Presets

```yaml
templates:
  pr-review:
    fields:
      - name: pr_number
        type: string
    prompt: "Review PR #{{ .pr_number }}"
    # New: preset spawn command options
    spawn_options:
      append_system_prompt: "Be thorough but concise"
```

### Phase 3: Templates with Pre/Post Steps

```yaml
templates:
  pr-review:
    fields:
      - name: pr_number
        type: string
    # New: setup before session
    pre_steps:
      - "gh pr checkout {{ .pr_number }}"
    prompt: "Review PR #{{ .pr_number }}"
    # New: cleanup after session ends
    post_steps:
      - "gh pr comment --body @review.md"
```

### Phase 4: Full Flows (if needed)

At this point, the gap to full Flows is smaller:

- `pre_steps` → Flow steps before Claude
- `post_steps` → Flow steps after Claude
- Add background execution
- Add asset capture

---

## Recommendation

**Start with Session Templates** for these reasons:

1. **Lower Risk**: Builds on proven patterns, no new execution model
2. **Faster Delivery**: 300-500 lines vs 1500-2500 lines
3. **Immediate Value**: Solves real UX problem (remembering complex prompts)
4. **User Alignment**: Keeps users in interactive mode they're familiar with
5. **Foundation for Future**: Templates can evolve toward Flows incrementally
6. **Simpler Testing**: No background processes, no Claude API mocking needed

**Flows may be valuable later** when:

- Users demonstrate need for unattended execution
- CI/CD integration becomes a priority
- Output archival is explicitly requested
- Multi-step orchestration proves necessary

---

## Proposed Template Schema (Detailed)

```yaml
# ~/.config/hive/config.yaml
templates:
  # Template identifier (used with --template flag)
  pr-review:
    # Human-readable description
    description: "Review a GitHub pull request"

    # Form fields presented to user
    fields:
      # Text input
      - name: pr_number
        label: "PR Number"
        type: string
        required: true
        placeholder: "e.g., 523"
        validation: "^[0-9]+$" # Optional regex

      # Single-select dropdown
      - name: review_depth
        label: "Review Depth"
        type: select
        default: "standard"
        options:
          - value: quick
            label: "Quick (5 min)"
          - value: standard
            label: "Standard (15 min)"
          - value: thorough
            label: "Thorough (30+ min)"

      # Multi-select checkboxes
      - name: focus_areas
        label: "Focus Areas"
        type: multi-select
        options:
          - value: security
            label: "Security vulnerabilities"
          - value: performance
            label: "Performance issues"
          - value: style
            label: "Code style"
          - value: tests
            label: "Test coverage"

      # Multi-line text
      - name: additional_context
        label: "Additional Context"
        type: text
        required: false
        placeholder: "Any specific concerns?"

    # Session name template (optional)
    name: "pr-{{ .pr_number }}"

    # Prompt template - rendered with field values
    prompt: |
      Review PR #{{ .pr_number }} with {{ .review_depth }} depth.

      Focus areas: {{ .focus_areas | join ", " }}

      {{ if .additional_context }}
      Additional context: {{ .additional_context }}
      {{ end }}
```

### CLI Usage

```bash
# Interactive form
$ hive new --template pr-review

# Non-interactive (all fields via flags)
$ hive new --template pr-review \
    --set pr_number=523 \
    --set review_depth=thorough \
    --set focus_areas=security,performance

# List available templates
$ hive templates list

# Show template details
$ hive templates show pr-review
```

---

## Open Questions for Templates

1. **Where to store templates?**
   - Config file only? (simpler)
   - Separate `~/.config/hive/templates/` directory? (more organized)
   - Both? (flexible)

2. **Template sharing?**
   - Git-based sharing (templates in repo's `.hive/templates/`)
   - Central registry?
   - Just documentation?

3. **Field types scope for v1?**
   - Minimum: `string`, `text`, `select`
   - Nice to have: `multi-select`, `boolean`
   - Later: `file` (file picker), `number` (with validation)

4. **Validation approach?**
   - Regex patterns only?
   - Custom validators?
   - Just required/optional for v1?

---

## Conclusion

Session Templates provide 80% of the value of Flows with 20% of the complexity. They solve the immediate problem of "I always forget the exact prompt format for PR reviews" while keeping users in the familiar interactive session model.

If automation needs emerge later, Templates provide a natural extension path toward Flows without requiring a ground-up rebuild.
