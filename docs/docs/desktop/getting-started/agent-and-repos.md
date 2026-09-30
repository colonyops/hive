---
icon: lucide/folder-git-2
description: Choose a coding agent and point Hive at the folders holding your repositories.
---

# Set up your agent and code

Hive starts coding sessions in your repositories. The first screen of first run asks for the two things it needs to do that: which coding agent to run, and where your code is.

Hive writes both to the hive CLI configuration file, so the `hive` command picks up the same setup. Nothing else in that file is touched.

## Choose an agent

Pick one or more from the list. Hive marks the ones it found on your `PATH`, and you can pick one you have not installed yet — it will work once the command is there.

Selecting more than one adds a **Start new sessions with** choice. That is the agent a new session uses when you do not name another.

!!! note "Skip-permission flags"
    **Skip the agent's permission prompts** adds the flag that lets the agent act without asking, where the agent has one — `--dangerously-skip-permissions` for Claude Code, `--full-auto` for Codex. It is off by default.

## Point at your repositories

Select **Choose a folder** and pick the folder that *contains* your repositories. For a layout like this:

```
~/code/
  hive-desktop/
  my-app/
  notes/
```

choose `~/code`, not `~/code/my-app`. Hive rejects a folder that is itself a git repository and tells you so.

Every repository under the folders you add shows up in the new session picker. Add as many folders as you like — work in one, side projects in another.

The row under each folder shows how many repositories Hive found there. If it says none, you probably picked a level too deep or too shallow.

## Already use the hive CLI?

If your config already names an agent and at least one folder, Hive says so and uses it as it is. Nothing is rewritten.

## Skipping

You can skip this step. The Inbox half of Hive — feeds, sources, flows, notifications — works without it. The new session picker stays empty until you set it.

## Changing it later

This screen is the only one that writes the file. To change either setting afterwards, edit the file yourself and restart Hive Desktop — **Settings ▸ Hive CLI** shows you where it is, opens it, and links to the hive CLI [configuration reference](../../cli/configuration/index.md).

The file is shared with the `hive` command, so anything else in it — rules, tmux settings, keybindings, your own agent profiles — is kept exactly as written.

!!! tip "`HIVE_DEFAULT_AGENT`"
    If that variable is set in your shell, it overrides the agent you choose here. This screen says so when it is set.
