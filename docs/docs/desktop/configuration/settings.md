---
icon: lucide/settings
description: Change Hive Desktop preferences in the app or through settings.yaml.
---

# Settings

Open Settings with <kbd>⌘,</kbd> or from the command palette.

## In the app

Settings shows or changes:

- general behavior such as polling and the default editor;
- the theme, interface fonts, and terminal typography;
- terminal spacing, visible windows, the status bar, and session age badges;
- keyboard shortcuts;
- notification delivery and sound;
- the feeds pinned to the menu bar;
- connected source accounts and the local webhook listener;
- actions and quick terminals;
- the agent workspace folder location;
- the addresses of the MCP servers Hive hosts, and how to add one to your own agent;
- the optional Hive CLI compatibility config;
- [local usage analytics](analytics.md), diagnostics, updates, and release channels.

The app validates changes before saving them.

## Configuration files

Hive Desktop and the hive CLI use separate configuration paths. They share Code sessions and tasks when both use the same Hive data root, which is the default.

| Scope | Common default | Configures |
| --- | --- | --- |
| Hive CLI and Code session engine | `~/.config/hive/config.yaml` | Repositories, agent profiles, clone and recycle rules, setup commands, starting tmux windows, and shared session behavior |
| Hive Desktop | `~/.config/hive/desktop/` | Inbox, Code presentation, Chats, notifications, integrations, shortcuts, and app behavior |

The hive CLI config honors `HIVE_CONFIG` and `XDG_CONFIG_HOME`. The Desktop config honors `HIVE_DESKTOP_CONFIG_DIR` and `XDG_CONFIG_HOME`. Hive Desktop reads the hive CLI configuration at startup, so restart it after editing that file.

Session and task sharing also depends on the data root. The common default is `~/.local/share/hive/`. If you move it, set `HIVE_DATA_DIR` in your shell profile. The hive CLI uses it, and Hive Desktop reads it from your login shell, so it applies even when you open the app from the Dock. Hive Desktop picks the data root in this order:

1. `HIVE_DESKTOP_HIVE_DATA_DIR`, for pointing only Hive Desktop somewhere else
2. `HIVE_DATA_DIR`
3. the Hive Desktop data root (`HIVE_DESKTOP_DATA_DIR`, or `~/.local/share/hive/`)

Hive Desktop logs the data root it picked at startup. If your login shell fails or takes more than a few seconds to start, the app cannot read `HIVE_DATA_DIR` for that run. It falls back to its own data root, so your sessions and tasks look missing until the next launch, and it logs a warning that names the directory it used.

See the hive CLI [configuration reference](../../cli/configuration/index.md) and [repository rules](../../cli/configuration/rules.md).

Hive Desktop keeps these files under its config directory by default:

| Content | Path |
| --- | --- |
| Settings | `settings.yaml` |
| Flows | `flows/` |
| Actions | `actions.yml` |
| Agent workspaces | `workspaces/` |
| Desktop application data | `~/.local/share/hive/desktop/` |
| Shared CLI and Desktop log | `<Hive data root>/hive.log` |
| Account tokens | OS keychain |

Set `HIVE_DESKTOP_CONFIG_DIR` or `HIVE_DESKTOP_DATA_DIR` to move the config or Desktop data root. You can also move the flow, action, and workspace paths separately. The shared log follows the Hive data root described above, not a separate Desktop data-root override when `HIVE_DATA_DIR` or `HIVE_DESKTOP_HIVE_DATA_DIR` selects another location. Settings ▸ Hive CLI and Settings ▸ System show the effective paths.

A `settings.yaml` key this version does not know, for example one a newer version wrote into a synced config directory, does not stop the app. Startup logs a warning naming the key and ignores it, so the in-app updater stays available. Saving settings from that version drops the key.

!!! tip "Ask the Hive workspace"
    The **Hive** workspace in **Chats** can update settings with the `hive-settings` skill. For example, ask it to change the polling interval or terminal font.

## Hive CLI compatibility config

Hive Desktop includes the Hive runtime it needs. A separately installed [Hive CLI](../../cli/getting-started/index.md) is not required.

If you use the Hive CLI, Desktop also reads its optional configuration file at startup. It checks `HIVE_CONFIG` first, then looks for `config.yaml`, `config.yml`, `hive.yaml`, or `hive.yml` under `$XDG_CONFIG_HOME/hive/` with `~/.config/hive/` as the fallback. No file is required. Hive uses built-in defaults when none exists.

First run asks for the two settings Hive Desktop needs from that file — the agents a session can start with, and the parent folders holding your repositories — and writes them; see [Set up your agent and code](../getting-started/agent-and-repos.md). Everything else in the file is left as written: rules, tmux settings, keybindings, user commands, and any agent profile with a command of its own.

Change session configuration later by editing the file yourself, then restarting Hive Desktop. **Settings ▸ Analytics** can separately edit the shared collection switches; see [Analytics](analytics.md). The hive CLI [configuration reference](../../cli/configuration/index.md) describes every key.

**Settings ▸ Hive CLI** shows the exact path selected at startup, and lets you copy it, open or reveal an existing file, or create a missing one and open it. It also tells you when the file no longer parses, which is worth checking after a hand edit.

`HIVE_DEFAULT_AGENT` takes precedence over the default agent you choose, when it names a configured profile. The screen says so when the variable is set.

## Advanced configuration

Most users can use the Settings screens. Edit `settings.yaml` for values that are not exposed there or when you manage configuration with dotfiles.

```yaml
polling:
  interval: 5m
http:
  enabled: true
  host: 127.0.0.1
  port: 0
paths:
  tmux: ""
editor:
  command: ""
agent_workspaces:
  dir: ""
appearance:
  terminal_show_session_age: true
  terminal_session_age_threshold_days: 5
retention:
  action_runs: 100
```

- `polling.interval` has a minimum of 60 seconds.
- The local HTTP server provides webhooks, MCP, and terminal connections. It only binds to loopback. Set a fixed port if another local tool needs a stable webhook URL.
- `paths.tmux` accepts an absolute path when Hive cannot find tmux.
- `editor.command` accepts an executable name or absolute path without arguments.
- `agent_workspaces.dir` changes where Chats workspaces are stored.
- `retention.action_runs` is how many finished action runs **Action runs** keeps, each with its full log. It defaults to 100 and accepts 1 through 10,000. A change takes effect the next time Hive starts.
- `appearance.terminal_show_session_age` shows a whole-day age badge in the Code sidebar after `terminal_session_age_threshold_days` days. It defaults to on with a five-day threshold. The threshold accepts 1 through 365.

Every scalar setting can be overridden for one launch with an environment variable based on its YAML path. For example, `polling.interval` becomes `HIVE_DESKTOP_POLLING_INTERVAL`.

## Updates

**Settings ▸ About** shows the current channel and controls automatic update checks.

To follow a different channel, edit `settings.yaml`:

```yaml
updates:
  channel: beta
```

Available channels are Stable, Beta, and Dev. Every release is published to all three, so each channel currently carries the same version. An omitted channel follows the channel used for the current build.

## Telemetry

**Settings ▸ Observability** shows the app's current CPU, memory, process tree, Go runtime, and UI frame behavior. Process data updates while the page is open. UI frame sampling also runs only while this page is open.

The same page reports the status of two independent Grafana Cloud exports:

- **Metrics, logs, and traces** use an OTLP endpoint.
- **Continuous profiles** use a Pyroscope-compatible Grafana Cloud Profiles endpoint.

Each card shows whether the destination is enabled and configured, plus whether it is **Exporting**, **Ready**, waiting for a restart, or failed to start. Exporting means Hive started the local exporter. Check Grafana Cloud or the Hive log for later delivery failures. A change in `settings.yaml` requires a restart.

Grafana Cloud provides different URLs and basic-auth users for OTLP and Profiles. A Cloud Access Policy token can serve both when it includes `profiles:write`, but configure each destination separately in `settings.yaml`:

```yaml
telemetry:
  enabled: true
  endpoint: https://otlp-gateway-prod-us-central-0.grafana.net/otlp
  instance_id: "123456"
  host_id: "fdbf79e8af94cb7f9e8df36789187052"
  token: op://Private/Grafana Cloud/otlp-token
  profiles:
    enabled: true
    endpoint: https://profiles-prod-us-central-0.grafana.net
    user: "123456"
    token: op://Private/Grafana Cloud/profiles-token
```

`telemetry.host_id` is optional. Set it to the machine id to add the OpenTelemetry `host.id` resource attribute to metrics, logs, and traces. Profiles use the same value as their `host_id` label. Hive does not read a machine id automatically. Each app launch gets a random OpenTelemetry `service.instance.id`. `telemetry.instance_id` has a different purpose: it is the OTLP endpoint's basic-auth username.

Profile export collects CPU, allocation, and in-use heap profiles. It does not collect goroutine, mutex, or block profiles. OTLP and profile export can run independently.

Tokens must use `env:`, `file:`, or `op://` references. Hive rejects literal telemetry tokens in the settings file. Endpoints and users may also use these references.

A ready-made Grafana dashboard for these signals is in the repository's [`cmd/desktop/grafana/`](https://github.com/colonyops/hive/tree/main/cmd/desktop/grafana) directory. Push it to your stack with [gcx](https://github.com/grafana/gcx). Its README maps each metric, span, and profile to the query that reads it.
