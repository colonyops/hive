# Allow nested Codex sandbox namespaces only in the remote development fixture

- **Status:** accepted
- **Date:** 2026-10-19

## Context

Docker's default seccomp policy prevents Codex's bubblewrap sandbox from
creating user and mount namespaces. Model responses work, but shell tools
fail. Disabling the agent sandbox would invalidate the fixture's acceptance
coverage.

## Decision

The remote development fixture uses a pinned Moby default seccomp profile
with allowances for `clone`, `unshare`, `mount`, `umount2`, and `pivot_root`.
Keep its upstream attribution and license beside the profile. Apply it only
to the Compose remote service. Add no capabilities, privileged mode, or
unconfined policy. Keep Codex on workspace-write with on-request approvals.

## Consequences

Processes in that container can create nested namespaces; the kernel still
checks namespace ownership and capabilities. The rest of Moby's syscall
filter remains in force. This is a development fixture policy, not a change
to production server installation or the smoke client. Host AppArmor and
user-namespace restrictions can still reject sandbox creation.
