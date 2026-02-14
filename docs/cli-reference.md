# CLI Reference

## Global Flags

| Flag           | Env Variable     | Default                      | Description                          |
| -------------- | ---------------- | ---------------------------- | ------------------------------------ |
| `--log-level`  | `HIVE_LOG_LEVEL` | `info`                       | Log level (debug, info, warn, error) |
| `--log-file`   | `HIVE_LOG_FILE`  | -                            | Path to log file                     |
| `--config, -c` | `HIVE_CONFIG`    | `~/.config/hive/config.yaml` | Config file path                     |
| `--data-dir`   | `HIVE_DATA_DIR`  | `~/.local/share/hive`        | Data directory path                  |

## `hive` (default)

Launches the interactive TUI for managing sessions.

**Features:**

- Tree view of sessions grouped by repository
- Real-time terminal status monitoring (with tmux integration)
- Preview sidebar showing live tmux pane output (`v` to toggle)
- Git status display (branch, additions, deletions)
- Filter sessions with `/` or by status via command palette
- Switch between Sessions and Messages views with `tab`

See [Commands & Keybindings](configuration/commands.md) for full TUI controls.

## `hive new`

Creates a new agent session.

| Flag       | Alias | Description                                     |
| ---------- | ----- | ----------------------------------------------- |
| `--name`   | `-n`  | Session name                                    |
| `--remote` | `-r`  | Git remote URL (auto-detected if not specified) |
| `--prompt` | `-p`  | AI prompt to pass to spawn command              |

```bash
hive new                                    # Interactive mode
hive new -n feature-auth -p "Add OAuth2"   # Non-interactive
```

## `hive ls`

Lists all sessions in a table format.

| Flag     | Description    |
| -------- | -------------- |
| `--json` | Output as JSON |

## `hive prune`

Removes recycled sessions exceeding the `max_recycled` limit.

| Flag    | Alias | Description                  |
| ------- | ----- | ---------------------------- |
| `--all` | `-a`  | Delete all recycled sessions |

## `hive batch`

Creates multiple sessions from a JSON specification.

| Flag     | Alias | Description                                          |
| -------- | ----- | ---------------------------------------------------- |
| `--file` | `-f`  | Path to JSON file (reads from stdin if not provided) |

```bash
echo '{"sessions":[{"name":"task1","prompt":"Fix auth bug"}]}' | hive batch
```

## `hive doctor`

Runs diagnostic checks on configuration and environment.

| Flag       | Description                      |
| ---------- | -------------------------------- |
| `--format` | Output format (`text` or `json`) |

## `hive ctx`

Manages context directories for sharing files between sessions.

### `hive ctx init`

Creates a symlink to the repository's context directory.

```bash
hive ctx init  # Creates .hive -> ~/.local/share/hive/context/{owner}/{repo}/
```

### `hive ctx prune`

Deletes files older than the specified duration.

| Flag           | Description                  |
| -------------- | ---------------------------- |
| `--older-than` | Duration (e.g., `7d`, `24h`) |

## `hive msg`

Publish and subscribe to messages for inter-agent communication. See [Messaging](getting-started/messaging.md) for usage patterns.

### `hive msg pub`

| Flag       | Alias | Description                    |
| ---------- | ----- | ------------------------------ |
| `--topic`  | `-t`  | Topic to publish to (required) |
| `--file`   | `-f`  | Read message from file         |
| `--sender` | `-s`  | Override sender ID             |

```bash
hive msg pub -t build.status "Build completed"
```

### `hive msg sub`

| Flag        | Alias | Description                        |
| ----------- | ----- | ---------------------------------- |
| `--topic`   | `-t`  | Topic pattern (supports wildcards) |
| `--last`    | `-n`  | Return only last N messages        |
| `--listen`  | `-l`  | Poll for new messages continuously |
| `--wait`    | `-w`  | Wait for a single message and exit |
| `--new`     | -     | Only unread messages               |
| `--timeout` | -     | Timeout for listen/wait mode       |

```bash
hive msg sub -t "agent.*" --last 10
hive msg sub --wait --timeout 5m
```

### `hive msg list`

Lists all topics with message counts.

### `hive msg topic`

Generates a unique topic ID.

| Flag       | Alias | Description  |
| ---------- | ----- | ------------ |
| `--prefix` | `-p`  | Topic prefix |

## `hive session info`

Displays information about the current session.

| Flag     | Description    |
| -------- | -------------- |
| `--json` | Output as JSON |

## `hive review`

Review and annotate markdown documents stored in context directories. See [Context & Review](getting-started/context.md) for full details.

| Flag       | Alias | Description                          |
| ---------- | ----- | ------------------------------------ |
| `--file`   | `-f`  | Path to specific document to review  |
| `--latest` | `-l`  | Open most recently modified document |

```bash
hive review                          # Interactive picker
hive review -f .hive/plans/auth.md   # Review specific file
hive review --latest                 # Review most recent document
```

## `hive doc`

Access documentation and guides.

### `hive doc migrate`

Shows configuration migration information.

| Flag    | Description         |
| ------- | ------------------- |
| `--all` | Show all migrations |

### `hive doc messaging`

Outputs messaging conventions documentation for LLMs.
