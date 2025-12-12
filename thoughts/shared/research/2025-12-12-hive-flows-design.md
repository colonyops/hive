---
date: 2025-12-12T00:00:00Z
researcher: Claude
git_commit: 453046ca522684930a9377a5f360b311c5ad3a74
branch: main
repository: hay-kot/hive
topic: "Hive Flows - Automated AI Agent Workflows"
tags: [research, codebase, flows, automation, background-processes, claude-code]
status: complete
last_updated: 2025-12-12
last_updated_by: Claude
---

# Research: Hive Flows - Automated AI Agent Workflows

**Date**: 2025-12-12
**Researcher**: Claude
**Git Commit**: 453046ca522684930a9377a5f360b311c5ad3a74
**Branch**: main
**Repository**: hay-kot/hive

## Research Question

How can hive provide a way to define "flows" - automated workflows that create sessions, run Claude Code scripts with configurable prompts, capture output as reviewable "assets", and manage long-running background processes without requiring the TUI to stay open?

## Summary

This research explores three interconnected features:

1. **Flow Definition**: YAML-based workflow configuration for automated AI agent tasks
2. **Background Execution**: Process management for long-running flows
3. **Asset Management**: Capturing and storing flow outputs for review

The existing hive architecture provides a solid foundation with session management, templated command execution, and hooks. The proposed flows feature would extend this by adding declarative workflow definitions, background process orchestration, and output capture.

## Detailed Findings

### 1. Current Hive Architecture

#### Session Management

Sessions are the core primitive in hive:

- **Session struct** (`internal/core/session/session.go:29-40`): ID, Name, Slug, Path, Remote, Prompt, State, timestamps
- **States**: `active` (in use) and `recycled` (available for reuse)
- **Storage**: JSON file store at `~/.local/share/hive/sessions.json`
- **Directory pattern**: `{repoName}-{slug}-{id}` (e.g., `hive-my-feature-a1b2c3`)

#### Command Execution Flow

1. **Spawn commands** (`internal/hive/spawner.go`): Templated commands executed after session creation
2. **Hooks** (`internal/hive/hooks.go`): Regex pattern-matched commands for repository setup
3. **Recycle commands**: Git reset/checkout/pull for session reuse

#### Template System

- Uses Go `text/template` with `shq` (shell quote) function
- **SpawnData variables**: `Path`, `Name`, `Slug`, `Prompt`
- **Keybinding variables**: `Path`, `Remote`, `ID`, `Name`

### 2. Claude Code CLI Integration

Claude Code provides headless execution modes ideal for automated workflows:

#### Key Flags for Automation

| Flag                                         | Purpose                                         |
| -------------------------------------------- | ----------------------------------------------- |
| `-p` / `--print`                             | Non-interactive mode, prints response and exits |
| `--output-format`                            | `text`, `json`, or `stream-json`                |
| `--max-turns`                                | Limit agentic turns                             |
| `--dangerously-skip-permissions`             | Skip permission prompts (use in containers)     |
| `--session-id`                               | Specific session UUID for tracking              |
| `--system-prompt` / `--append-system-prompt` | Custom instructions                             |
| `--json-schema`                              | Structured output validation                    |

#### Example Flow Invocation

```bash
# PR Review flow
claude -p \
  --output-format json \
  --max-turns 10 \
  --append-system-prompt "Review PR #523 for security issues" \
  "Review this pull request"
```

### 3. Background Process Management

#### Go Daemonization Challenge

Go's runtime doesn't support traditional Unix `fork()` daemonization due to thread pool interactions. Recommended approaches:

1. **External Process Managers** (Preferred)
   - systemd (Linux)
   - launchd (macOS)
   - supervisord (cross-platform)

2. **Self-managed Background Processes**
   - `os/exec.Command()` with detached process group
   - Write PID file for tracking
   - Redirect stdout/stderr to files

3. **kardianos/service Library**
   - Cross-platform service abstraction
   - Supports systemd, launchd, Windows services

#### Proposed Approach for Hive Flows

Given hive's CLI nature, a hybrid approach makes sense:

```
┌─────────────────────────────────────────────────────────┐
│                     hive flow                            │
├─────────────────────────────────────────────────────────┤
│  Foreground (default):                                   │
│    - Stream output to terminal                           │
│    - Ctrl+C to cancel                                    │
│                                                          │
│  Background (--background / --bg):                       │
│    - Spawn detached process                              │
│    - Write PID to ~/.local/share/hive/flows/{id}/pid     │
│    - Log to ~/.local/share/hive/flows/{id}/output.log    │
│    - Status via: hive flow status                        │
└─────────────────────────────────────────────────────────┘
```

### 4. Proposed Flow Configuration Schema

#### Flow Definition (in hive config or separate files)

```yaml
# ~/.config/hive/config.yaml
flows:
  pr-review:
    description: "Review a GitHub PR"
    args:
      - name: pr_number
        required: true
        type: string

    # Session creation (optional - reuse existing or create new)
    session:
      # If remote not specified, detect from current directory
      remote: ""
      # Name template for session
      name: "pr-review-{{ .Args.pr_number }}"

    # Steps execute sequentially
    steps:
      - name: fetch-pr
        type: shell
        command: "gh pr checkout {{ .Args.pr_number }}"
        workdir: "{{ .Session.Path }}"

      - name: review
        type: claude
        prompt: |
          Review PR #{{ .Args.pr_number }}.
          Focus on:
          - Security vulnerabilities
          - Performance issues
          - Code style violations
        options:
          output_format: json
          max_turns: 20
          append_system_prompt: "Be thorough but concise"

    # Output handling
    output:
      # Save to asset store
      asset:
        type: pr-review
        name: "PR #{{ .Args.pr_number }} Review"
      # Also print to console
      console: true

  lint-fix:
    description: "Fix all lint errors in the codebase"
    steps:
      - name: fix-lint
        type: claude
        prompt: "Fix all lint errors. Run the linter after each fix to verify."
        options:
          dangerously_skip_permissions: true
          max_turns: 50
```

#### Standalone Flow Files

```yaml
# ~/.config/hive/flows/pr-review.yaml
name: pr-review
description: "Review a GitHub PR"
# ... flow definition
```

### 5. Proposed Asset Management

Assets capture flow outputs for later review:

```yaml
# Asset structure
~/.local/share/hive/
├── sessions.json
├── assets.json          # Asset metadata
├── repos/
│   └── ...
├── flows/               # Running/completed flow state
│   └── {flow-run-id}/
│       ├── state.json   # Flow execution state
│       ├── output.log   # Combined stdout/stderr
│       └── pid          # PID file (if background)
└── assets/              # Asset content
    └── {asset-id}/
        ├── meta.json    # Asset metadata
        └── content.json # Flow output
```

#### Asset Schema

```go
type Asset struct {
    ID          string            `json:"id"`
    Type        string            `json:"type"`        // "pr-review", "lint-fix", etc.
    Name        string            `json:"name"`
    FlowRunID   string            `json:"flow_run_id"` // Link to flow execution
    SessionID   string            `json:"session_id"`  // Link to hive session
    CreatedAt   time.Time         `json:"created_at"`
    Status      string            `json:"status"`      // "pending", "complete", "failed"
    ContentPath string            `json:"content_path"`
    Metadata    map[string]string `json:"metadata"`    // Flow-specific data
}
```

### 6. Proposed CLI Commands

```bash
# Run a flow (foreground)
hive flow pr-review 523

# Run a flow in background
hive flow pr-review 523 --background

# List running/recent flows
hive flow status

# View flow output
hive flow logs {flow-run-id}

# Cancel a running flow
hive flow cancel {flow-run-id}

# List assets
hive assets ls
hive assets ls --type pr-review

# View an asset
hive assets view {asset-id}

# Delete an asset
hive assets delete {asset-id}
```

### 7. Implementation Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        hive flow                              │
├──────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐       │
│  │ FlowConfig  │───▶│ FlowRunner  │───▶│ StepRunner  │       │
│  │   (YAML)    │    │             │    │             │       │
│  └─────────────┘    └─────────────┘    └─────────────┘       │
│                            │                  │               │
│                            │                  ▼               │
│                            │           ┌─────────────┐       │
│                            │           │ ShellStep   │       │
│                            │           │ ClaudeStep  │       │
│                            │           │ ...         │       │
│                            │           └─────────────┘       │
│                            │                  │               │
│                            ▼                  ▼               │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐       │
│  │   Session   │◀───│  FlowState  │───▶│   Asset     │       │
│  │   Store     │    │   Store     │    │   Store     │       │
│  └─────────────┘    └─────────────┘    └─────────────┘       │
│                                                               │
└──────────────────────────────────────────────────────────────┘
```

#### Key Components

1. **FlowConfig**: Parsed YAML flow definition
2. **FlowRunner**: Orchestrates flow execution
   - Creates/reuses sessions
   - Executes steps sequentially
   - Manages background execution
   - Writes state and logs
3. **StepRunner**: Executes individual steps
   - `ShellStep`: Run shell commands
   - `ClaudeStep`: Run Claude Code with options
4. **FlowState Store**: Track running/completed flows
5. **Asset Store**: Persist flow outputs

### 8. Background Execution Design

```go
// FlowRunner background execution
func (r *FlowRunner) RunBackground(ctx context.Context, flow FlowConfig, args map[string]string) (string, error) {
    runID := generateID()

    // Create flow directory
    flowDir := filepath.Join(r.dataDir, "flows", runID)
    os.MkdirAll(flowDir, 0755)

    // Create log file
    logFile, _ := os.Create(filepath.Join(flowDir, "output.log"))

    // Fork new process
    cmd := exec.Command(os.Args[0], "flow", "--run-id", runID, "--internal")
    cmd.Stdout = logFile
    cmd.Stderr = logFile
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Setpgid: true,  // New process group
    }

    if err := cmd.Start(); err != nil {
        return "", err
    }

    // Write PID file
    os.WriteFile(
        filepath.Join(flowDir, "pid"),
        []byte(strconv.Itoa(cmd.Process.Pid)),
        0644,
    )

    return runID, nil
}
```

### 9. TUI Integration

The TUI could be extended to show flows:

```
┌─────────────────────────────────────────────────────────────┐
│  HIVE                                    [Sessions] [Flows]  │
├─────────────────────────────────────────────────────────────┤
│  > pr-review-523        running    2m ago                    │
│    lint-fix-main        complete   1h ago                    │
│    code-review-abc      failed     3h ago                    │
│                                                              │
├─────────────────────────────────────────────────────────────┤
│  [v]iew  [c]ancel  [d]elete  [a]ssets                        │
└─────────────────────────────────────────────────────────────┘
```

## Code References

- `internal/core/session/session.go:29-40` - Session struct definition
- `internal/hive/service.go:62-164` - CreateSession flow
- `internal/hive/spawner.go:14-20` - SpawnData template context
- `internal/hive/hooks.go:34-77` - Hook execution pattern
- `internal/core/config/config.go:32-82` - Config struct (extension point)
- `pkg/tmpl/tmpl.go:31-43` - Template rendering
- `pkg/executil/exec.go:11-21` - Command execution interface

## Architecture Insights

### Existing Patterns to Leverage

1. **Session reuse**: Flows should leverage the recycling system
2. **Template rendering**: Use existing `tmpl.Render()` for flow templates
3. **Hook pattern**: Step execution follows the same sequential pattern as hooks
4. **Executor interface**: Use existing `executil.Executor` for shell steps

### New Components Needed

1. **Flow configuration parser**: Parse flow definitions from YAML
2. **Flow runner**: Orchestrate step execution
3. **Claude step executor**: Wrapper around Claude CLI invocation
4. **Background process manager**: Handle detached execution
5. **Asset store**: Persist flow outputs
6. **Flow state store**: Track running flows

### Design Decisions

| Decision                  | Options                       | Recommendation                              |
| ------------------------- | ----------------------------- | ------------------------------------------- |
| Flow definition location  | Config file vs separate files | Both - inline for simple, files for complex |
| Background implementation | OS daemon vs self-managed     | Self-managed (simpler, more portable)       |
| Asset storage             | Files vs database             | JSON files (consistent with sessions)       |
| Step types                | Plugin vs built-in            | Built-in initially (shell, claude)          |
| Session handling          | Always create vs optional     | Optional - can run in existing session      |

## Open Questions

1. **Flow composition**: Should flows be able to call other flows?
2. **Parallelism**: Should steps support parallel execution?
3. **Retry logic**: How to handle failed steps?
4. **Notifications**: Should flows support webhooks/notifications on completion?
5. **Asset retention**: How long to keep assets? Auto-prune policy?
6. **Security**: Should `--dangerously-skip-permissions` require explicit config flag?
7. **Session lifecycle**: Should flow sessions auto-recycle on completion?
8. **TUI vs CLI**: Primary interface for flow management?

## Related Resources

- [Claude Code CLI Reference](https://code.claude.com/docs/en/cli-reference)
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [kardianos/service](https://github.com/kardianos/service) - Go service management
- [Go daemon patterns](https://github.com/golang/go/issues/227) - Go daemonization discussion
