---
icon: lucide/git-branch
---

# Git-backed Context

Store hive's planning and research documents in a git repository so they are version-controlled, searchable, and shareable across machines.

By default, context documents live inside hive's data directory (`~/.local/share/hive/context/`), which is not tracked by git. Redirecting them into a dedicated repository gives you full history, diffs, and automatic sync across machines.

## Setup

**1. Create (or pick) a git repository for your context documents:**

```bash
mkdir ~/notes/hive-context
cd ~/notes/hive-context
git init
git remote add origin git@github.com:you/hive-context.git
```

**2. Point hive at it via `context.base_dir`:**

```yaml
# ~/.config/hive/config.yaml
context:
  base_dir: ~/notes/hive-context
```

**3. Validate the config:**

```bash
hive doctor
```

**4. Initialize the `.hive` symlink in each repository you work on:**

```bash
cd ~/projects/my-repo
hive ctx init   # creates .hive → ~/notes/hive-context/{owner}/{repo}/
```

Documents written to `.hive/plans/` and `.hive/research/` now land inside `~/notes/hive-context/`.

## Automatic Syncing

Set up a periodic job to commit, pull, and push the context repository automatically. This handles documents written at any point during a session without requiring manual intervention.

=== "macOS (launchd)"

    Save to `~/Library/LaunchAgents/hive.context-sync.plist`:

    ```xml
    <?xml version="1.0" encoding="UTF-8"?>
    <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
      "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
    <plist version="1.0">
    <dict>
      <key>Label</key>
      <string>hive.context-sync</string>
      <key>ProgramArguments</key>
      <array>
        <string>/bin/sh</string>
        <string>-c</string>
        <string>cd ~/notes/hive-context &amp;&amp; git add -A &amp;&amp; git diff --cached --quiet || git commit -m "auto: sync context" &amp;&amp; git pull --rebase --autostash &amp;&amp; git push</string>
      </array>
      <key>StartInterval</key>
      <integer>300</integer>
      <key>RunAtLoad</key>
      <true/>
    </dict>
    </plist>
    ```

    Load it:

    ```bash
    launchctl load ~/Library/LaunchAgents/hive.context-sync.plist
    ```

=== "Linux (cron)"

    ```bash
    # crontab -e
    */5 * * * * cd ~/notes/hive-context && git add -A && git diff --cached --quiet || git commit -m "auto: sync context" && git pull --rebase --autostash && git push
    ```

This runs every 5 minutes: commits any new or changed documents, pulls remote changes, and pushes.

## Directory Structure

With `base_dir: ~/notes/hive-context`, the layout is:

```
~/notes/hive-context/
├── my-org/
│   ├── my-repo/
│   │   ├── plans/
│   │   │   └── 2026-01-15-auth-refactor.md
│   │   └── research/
│   │       └── api-analysis.md
│   └── other-repo/
│       └── plans/
└── shared/
    └── architecture-decisions.md
```

Each project's `.hive` symlink points to its own subdirectory. All documents live in one repository that you can push, pull, and browse with standard git tools.

## Tips

- **Use a private repository** if your plans contain sensitive design details.
- **Add a `.gitignore`** to the context repo to exclude any generated files or large binaries you don't want tracked.
- **Use Obsidian as a GUI** — open `~/notes/hive-context` as an Obsidian vault to get a rich editor, graph view, and search across all your plans and research documents.
- **Add a `CLAUDE.md`** at the root of the context repository and use the repository as a command center for dispatching sessions and managing work.
