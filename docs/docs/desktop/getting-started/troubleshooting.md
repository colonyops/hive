---
icon: lucide/life-buoy
description: Fix common setup, source, notification, and terminal problems.
---

# Troubleshooting

Check **Activity** for flow, source, and action errors. Open **Settings ▸ System ▸ Diagnostics** for the application log.

## Hive does not start on Linux

Hive requires GTK 4 and WebKitGTK 6.0. Install their runtime packages, then start Hive again:

=== "Debian or Ubuntu"

    ```sh
    sudo apt install libgtk-4-1 libwebkitgtk-6.0-4
    ```

=== "Fedora"

    ```sh
    sudo dnf install gtk4 webkitgtk6.0
    ```

=== "Arch Linux"

    ```sh
    sudo pacman -S gtk4 webkitgtk-6.0
    ```

Hive supports Ubuntu 24.04, Debian 13, Fedora 40, and newer releases. See the [Wails Linux documentation](https://v3.wails.io/guides/build/linux/) for details about the GTK and WebKitGTK stack.

The terminal installer also adds Hive to your desktop application menu. If you installed an older version of the script or extracted the tarball manually, run the current [terminal installer](index.md#install-from-the-terminal) to create the entry.

## tmux is not installed

Code, Chats, and quick terminals require tmux 3.2 or newer.

```sh
brew install tmux
```

Return to Code after installation. Hive checks again without a restart. If tmux is installed in an unusual location, set `paths.tmux` in [Settings](../configuration/settings.md#advanced-configuration).

## A coding agent is not found

Hive currently supports the `claude` and `codex` commands. Check the selected workspace agent, then confirm the command is available:

```sh
which claude
which codex
```

Hive reads your login shell environment at launch. Fix the shell's `PATH`, then restart Hive.

## Notifications never appear as banners

Open **Settings ▸ Notifications** and check the master switch, delivery mode, and system permission.

On macOS, select **Allow notifications** if permission has not been requested. If permission is denied, enable Hive under **System Settings ▸ Notifications**.

## The GitHub code expired

Start **Connect GitHub** again and approve the new code. You can also select **Use a token instead** and provide a classic token with `repo` and `notifications` scopes.

## The feed is empty after skipping GitHub

Connecting GitHub after first run does not add the starter feeds automatically. Open the flow editor or ask the **Hive** workspace in Chats to create them.

See [Flows](../inbox/flows.md) for a minimal source-to-feed example.

## A config change did nothing

Hive keeps the last valid flow or action file active when an edit fails validation. Open Activity, fix the reported error, and save again.

Common errors include unknown fields, invalid durations, duplicate node IDs, and unsupported action targets.

## A webhook does not arrive

Check the sender's result:

- `401` means the `X-Hive-Secret` header is missing or incorrect.
- Connection refused means the local listener is disabled or the port changed.
- `202` with no feed item usually means the path, node state, flow state, or wiring is wrong.

**Settings ▸ Integrations ▸ Webhooks** shows the current URL and lets you set a fixed port. The webhook node editor shows the most recent accepted delivery.

## A feed source reports an error

Open Activity and read the error. The usual cause is a URL that points at a page instead of at its feed document. Look for a `<link rel="alternate" type="application/rss+xml">` tag in the page source, or try `/feed`, `/rss`, or `/atom.xml`.

Hive also fails the fetch when the feed asks for credentials, answers a non-2xx status, or is larger than 8 MiB. A failed fetch changes nothing, so the entries already in the feed stay where they are.

## A session was not created

The New Session form closes as soon as Hive accepts it, because the clone runs in the background. If that clone or a session hook fails, Hive raises an error toast that stays until you dismiss it, and keeps the form.

Select **Retry** on the toast, or open the form again with ⌘N. It comes back with the repository, name, prompt, and agent you submitted, and shows the step creation stopped on together with the last lines it printed.

The failure is also recorded in **Activity**, with its own **Retry** button. That row is stored, so it survives a restart: fix the cause first, then come back and retry the same form.

A clone that fails after the checkout is complete leaves the directory behind, and no session list shows it. The failure names the path, so you can delete it:

```sh
rm -rf ~/.local/share/hive/repos/<repo>-<id>
```

A global git hook is a common cause. `git clone` returns the exit code of its `post-checkout` hook, so a hook that fails turns a finished clone into a failed session. Run the same clone in a terminal to see the hook's own output.

## The terminal does not fill its pane

Another tmux client may be setting the shared window size. Detach or resize that client, or add this to `tmux.conf`:

```text
set -g window-size largest
```

See [Terminal mode](../code/terminal-mode.md#shared-tmux-sizing).

## Report a problem

Open **Settings ▸ System** and select **Report a problem**. Hive opens a new issue at [github.com/hay-kot/hive-desktop/issues](https://github.com/hay-kot/hive-desktop/issues) with your version and platform filled in. Nothing from your machine is attached. Describe the problem and paste the log lines that show it.

The default log path is:

```text
~/.local/share/hive/desktop/desktop.log
```

## Save a diagnostic bundle

Use this only when a maintainer asks for one. Open **Settings ▸ System** and select **Save a diagnostic bundle**.

The bundle always holds build and system details. Logs, settings, flows, and actions are separate switches, and all four start off. Switch on what the maintainer asks for.

Tokens, secrets, and API keys are removed from everything the bundle holds. Names are not. A log tail names your home directory, repositories, and branches, and flows name the orgs and hosts they poll.

**Do not attach a bundle to a GitHub issue.** Issues are public. Send the file through the private channel the maintainer gives you.

Bundles are written to:

```text
~/.local/share/hive/desktop/reports/
```

Read one with `gunzip -c hive-report-<id>.json.gz | jq .`.
