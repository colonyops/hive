# NAME

hive - Manage multiple AI agent sessions

# SYNOPSIS

hive

```
[--config|-c]=[value]
[--data-dir]=[value]
[--log-file]=[value]
[--log-level]=[value]
[--profiler-port]=[value]
```

# DESCRIPTION

Hive creates isolated git environments for running multiple AI agents in parallel.

Instead of managing worktrees manually, hive handles cloning, recycling, and
spawning terminal sessions with your preferred AI tool.

Run 'hive' with no arguments to open the interactive session manager.
Run 'hive new' to create a new session from the current repository.

**Usage**:

```
hive [global options] command [command options]
```

# GLOBAL OPTIONS

**--config, -c**="": path to config file (default: "/Users/hayden/.config/hive/config.yaml")

**--data-dir**="": path to data directory (default: "/Users/hayden/.local/share/hive")

**--log-file**="": path to log file (defaults to <data-dir>/hive.log)

**--log-level**="": log level (debug, info, warn, error, fatal, panic) (default: "info")

**--profiler-port**="": enable pprof HTTP endpoint on specified port (e.g., 6060) (default: 0)


# COMMANDS

## new

Create a new agent session

>hive new <name...>

**--remote, -r**="": git remote URL (defaults to current directory's origin)

**--source, -s**="": source directory for file copying (defaults to current directory)

## ls

List all sessions

>hive ls [--json]

**--json**: output as JSON lines with inbox info

## prune

Remove recycled sessions exceeding max_recycled limit

>hive prune [--all]

**--all, -a**: Delete all recycled sessions (ignore max_recycled limit)

## doctor

Run health checks on your hive setup

>hive doctor [options]

**--autofix**: automatically fix issues (e.g., delete orphaned worktrees)

**--format**="": output format (text, json) (default: "text")

## batch

Create multiple sessions from JSON input

    hive batch [options]
    
    Read from stdin:
      echo '{"sessions":[{"name":"task1","prompt":"Do something"}]}' | hive batch
    
    Read from file:
      hive batch -f sessions.json

**--file, -f**="": path to JSON file (reads from stdin if not provided)

## ctx

Manage context directories for inter-agent communication

**--repo, -r**="": target a specific repository context (owner/repo)

**--shared, -s**: use the shared context directory

### init

Create symlink to context directory

### ls

List context directory contents as a tree

### prune

Delete old files from context directory

**--older-than**="": delete files older than this duration (e.g., 7d, 24h)

## msg

Publish and subscribe to inter-agent messages

### pub

Publish a message to topic(s)

>hive msg pub --topic <topic> [--topic <topic2>] [message]

**--file, -f**="": read message from file

**--sender, -s**="": override sender ID (default: auto-detect from session)

**--topic, -t**="": topic(s) to publish to (supports wildcards, repeatable)

### sub

Read messages from a topic

>hive msg sub [--topic <pattern>] [--last N] [--listen]

**--last, -n**="": return only last N messages (default: 0)

**--listen, -l**: poll for new messages instead of returning immediately

**--peek**: read without acknowledging messages

**--timeout**="": timeout for --listen/--wait mode (e.g., 30s, 5m, 24h) (default: "30s")

**--topic, -t**="": topic pattern to subscribe to (supports wildcards like agent.*)

**--wait, -w**: wait for a single message and exit (for inter-agent handoff)

### inbox

Read messages from your session's inbox

**--all**: show all messages, not just unread

**--peek**: don't mark messages as read

### list

List all topics

>hive msg list

### topic

Generate a random topic ID

>hive msg topic [--prefix <prefix>]

**--new, -n**: generate a new random topic ID (default behavior)

**--prefix, -p**="": topic prefix (overrides config, use empty string for no prefix)

## doc

Documentation and migration guides

### migrate

Show configuration migration guide

**--all**: show all migrations, not just those needed for your config

### messaging

Show inter-agent messaging conventions for LLMs

## session

Session management commands

### info

Show current session information

**--json**: output as JSON (recommended for LLMs)

## review

Review and annotate markdown documents

**--file, -f**="": path to markdown file (absolute or relative to cwd)

**--latest**: open most recently modified document (requires context dir)
