# Run the headless server and its tmux daemon as separate user services

- **Status:** accepted
- **Date:** 2026-10-15

## Context

A headless installation must start at boot under the agent account. A backend
upgrade must not kill the tmux panes it serves. Systemd normally stops all
processes in a service's control group.

## Decision

Ship a Linux bundle with a per-user installer and two systemd user services.
The tmux service owns a foreground daemon in a private runtime socket directory.
The Hive service depends on it and uses that same directory. Restarting Hive
leaves tmux running. Each service retains systemd's normal process cleanup.
The operator enables lingering for boot startup and operation after logout.

## Consequences

The server installation has separate data and configuration roots and uses
SSH to reach its loopback TLS API. It needs no GUI or desktop keychain when
connector credentials use environment overrides. Agent CLI credentials belong
to the service account. Restarting tmux or rebooting loses live processes;
persisted sessions and schedule catch-up policy remain available.
