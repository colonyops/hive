---
icon: lucide/square-terminal
description: Install the hive command from Hive Desktop, put it on your PATH, and use it next to a Homebrew or go install copy.
---

# The hive command

Hive Desktop can install the `hive` command for you. It is a link at `~/.local/bin/hive` that points at the Hive Desktop app, so the command is always the same version as the app and updates with it. Hive Desktop and the command share one database, and two different versions can disagree about its layout.

First run asks whether to install it, with the box checked. **Settings ▸ Hive CLI** has the same switch afterwards. Turning it off removes the link.

If you updated from a version that did not have this, the switch starts off and nothing is installed until you turn it on.

## Check that it works

Open a new terminal and run:

```bash
hive --version
```

The version matches the one in **Settings ▸ About**. **Settings ▸ Hive CLI** shows where the link is, the app's version, and the version your terminal runs. It warns you when they differ.

## Add ~/.local/bin to your PATH

If `~/.local/bin` is not on the `PATH` your login shell sets, the terminal cannot find `hive`. Settings shows the line to add:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Put it in your shell profile, such as `~/.zshrc` or `~/.bashrc`, and open a new terminal. Hive Desktop never edits your shell files.

## If you already have a hive

Hive Desktop only removes or replaces a link it made. If `~/.local/bin/hive` is a file or a link you made some other way, Hive leaves it alone, and first run says so.

A copy earlier on your `PATH` hides the link. Settings names that copy. To use the app's command, remove the other one:

- Homebrew: `brew uninstall --cask hive`
- `go install`: delete the `hive` file in your Go `bin` directory, usually `~/go/bin/hive`

Keeping both works too. Update the other copy whenever Hive Desktop updates.

## When the switch is off

- **Development builds** never install the command.
- **macOS can run Hive from a temporary location** after you open it straight from a download. Move Hive to your Applications folder and open it from there, then turn the switch on.
- **A folder that cannot be written** shows the error under the switch. Fix the permissions on `~/.local/bin` and turn the switch on again.

## Setting

The switch is `hive_cli.install_command` in `settings.yaml`. Settings writes it. A missing key means you were never asked.

```yaml
hive_cli:
  install_command: true
```

See [Settings](settings.md#hive-cli-compatibility-config) for the shared configuration file, and the hive CLI [documentation](../../cli/getting-started/index.md) for what the command does.
