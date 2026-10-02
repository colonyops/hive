---
icon: lucide/rocket
description: Install Hive Desktop and create your first feed.
---

# Getting Started

Hive Desktop collects work from supported sources and routes it into local feeds. Flows decide which items enter a feed, show a notification, or run an action.

## Install

<div class="hive-downloads" data-hive-downloads markdown="1">

Direct downloads come from the release manifest. Use the install script below if they do not appear.

</div>

The macOS download is a `.dmg`: open it and drag **Hive** to Applications. The Linux downloads are tarballs: extract one and put the `hive-desktop` binary on your `PATH`.

### Install from the terminal

The install script picks the right build, verifies its checksum, and installs the app. It leaves the separate [`hive` CLI](../../cli/getting-started/index.md) unchanged.

```sh
curl -fsSL https://hivedesktop.com/install.sh | bash
```

=== "macOS"

    Installs `/Applications/Hive.app`, or `~/Applications` when the first is not writable.

=== "Linux"

    Hive requires the GTK 4 and WebKitGTK 6.0 runtime libraries. [Install the packages for your distribution](troubleshooting.md#hive-does-not-start-on-linux) before you start Hive.

    The installer puts the binary under `~/.local/share/hive` and adds Hive to your application menu. Set `HIVE_HOME` to choose another binary directory. A manually extracted tarball does not add the application-menu entry.

To inspect the installer first, omit `| bash`.

Hive updates itself. You can change the release channel under **Settings ▸ About**. See [Settings](../configuration/settings.md#updates).

## First run

1. Choose a coding agent and the folders holding your repositories.
2. Connect GitHub, or skip it and connect another [source](../inbox/sources.md).
3. Allow notifications if you want system banners.
4. Start a chat with your agent, or open the feed.

Hive creates a profile named **Default** for you; connecting GitHub fills it
with the starter feeds. The last step opens the built-in **Hive** workspace,
where your agent asks how you work and offers to set up your profiles and
feeds. Select **Not now** to go straight to the feed instead.

These pages cover each step:

- [Set up your agent and code](agent-and-repos.md)
- [Sign in to GitHub](sign-in.md)
- [Turn on notifications](notifications.md)
- [See your first items](first-feed.md)

## Configure Hive

Use the flow editor and Settings screens for normal configuration. You can also edit the YAML files under `~/.config/hive/desktop/`.

The built-in **Hive** workspace in **Chats** can create flows, connect generic sources, define actions, and change settings. It requires a supported coding agent CLI on your `PATH`.

Start with:

- [Feature overview](features.md)
- [Sources](../inbox/sources.md)
- [Flows](../inbox/flows.md)
- [Actions](../inbox/actions.md)
- [Keyboard shortcuts](../configuration/keybindings.md)
