---
icon: lucide/terminal
description: Install the hive command from Hive Desktop so the CLI in your terminal is always the app's version.
---

# Install the hive command

Hive Desktop includes the [hive CLI](../../cli/getting-started/index.md). The second screen of first run offers to install it as the `hive` command, and the box is checked by default.

Hive installs the command as a link at `~/.local/bin/hive` that points at the app. The command is always the same version as the app, and every app update moves it too. Both share one database, so keeping them on one version is what keeps them compatible.

Turn it on or off later under **Settings ▸ Hive CLI**. Turning it off removes only the link Hive created.

## If `~/.local/bin` is not on your PATH

Settings shows a hint when your shell cannot find the link. Add the directory to your shell profile, for example in `~/.zshrc`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

## If you already have hive

Hive never replaces or removes a `hive` it did not install, such as one from Homebrew or `go install`.

- When that copy is at `~/.local/bin/hive`, Hive leaves it and Settings says so. Remove it to let Hive manage the command.
- When it is elsewhere and comes first on your `PATH`, your shell keeps running it. Settings names the copy, shows its version beside the app's, and suggests removing it, for example with `brew uninstall --cask hive`.

Development builds and an app that macOS runs from a temporary location (one opened straight from `~/Downloads`) do not install the command. Move Hive to Applications first.
