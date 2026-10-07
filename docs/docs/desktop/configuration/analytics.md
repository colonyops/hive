---
icon: lucide/chart-no-axes-column
description: View and clear local usage counts shared by Hive Desktop and the hive CLI.
---

# Analytics

Open **Settings ▸ Analytics**, or search for **Analytics** in the command palette.

The page counts three event types over the last 90 days:

- completed CLI commands, including failures;
- successful Hive session creations from either program;
- new Desktop terminal starts, including scratch terminals. Attaching to an already-running terminal does not count.

Click **Refresh counts** after an operation. Writes run in the background and can take up to 10 seconds. A CLI invocation flushes its events before it exits. Counts use UTC capture times.

## Collection and privacy

Local collection is on by default. Nothing is exported. Events contain bounded command identifiers, outcomes, durations, clone strategies, checkout reuse, or terminal kinds. They do not contain repository names, paths, branches, prompts, raw arguments, account details, or terminal output.

History lives in `usage-analytics.db` beside the shared `hive.db`. Both programs must use the same [Hive data root](settings.md#configuration-files) to share counts. The analytics database has its own random installation identifier, independent of operational telemetry. The page shows this shared UUID and lets you copy it. If no local history exists, no identifier is created just to display one.

The collection switch edits only analytics settings in the [external Hive CLI configuration](settings.md#hive-cli-compatibility-config). Restart Desktop after changing it. A CLI process uses the setting from its next invocation; already-running processes keep their startup setting.

You can also edit that file directly:

```yaml
analytics:
  enabled: false
  local:
    enabled: true
```

Both keys default to `true`. Either `false` disables the collector without opening an analytics writer. Set `HIVE_ANALYTICS_ENABLED=false` to disable collection for one process. Desktop reads this override through the login shell and labels it on the page. The override never forces disabled configuration on.

## Retention and clear

Collection removes rows older than 90 days at startup and daily. Disabling it preserves history and stops retention maintenance. The page can still read and clear existing history.

**Clear history** asks for confirmation. It deletes shared history and rotates the installation identifier. Events captured before the clear cannot return later from another process's pending batch. Collection settings do not change.

Analytics is best-effort: a crash, a full queue, or an unavailable database can lose events without failing the operation you performed.
