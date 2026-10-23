# Hive acts as the macOS TCC responsible process for terminal children

- **Status:** accepted
- **Date:** 2026-08-07

## Context

On macOS, Hive starts tmux, shells, agents, and their tools, so TCC treats the
signed app as the responsible code when a child touches a protected folder,
another app, or a local-network address. A command that works in another
terminal can fail inside Hive, because each host has its own grants and
declarations. A responsible app must declare the resources its children may
request, and hardened runtime also needs the Apple Events entitlement before a
child can raise an Automation prompt.

Access to another app's container is a per-process privilege with no
usage-description key or entitlement that makes it persistent. Only Full Disk
Access suppresses the prompt, and only the user or an MDM policy can grant it.

## Decision

Hive stays the responsible code for what it launches. It does not use the
private responsibility-disclaim spawn API or move terminals into a launchd agent
to evade attribution.

`cmd/desktop/build/darwin/Info.plist` and `Info.dev.plist` declare usage
descriptions for Apple Events, Desktop, Documents, Downloads, local network,
network volumes, removable volumes, and system administration.
`entitlements.plist` carries `com.apple.security.automation.apple-events`, and
every Darwin signing path (the release publisher and the ad-hoc and Developer ID
tasks in `build/darwin/Taskfile.yml`) signs with it. `macos_privacy_test.go`
in `cmd/tools/release` guards the declarations.

The app requests nothing at startup; macOS asks when a command first does the
operation. No App Sandbox entitlements are added: Hive is not sandboxed, and
they grant no TCC privilege. Full Disk Access stays optional and is never
requested on the user's behalf.

## Consequences

- The first use of each protected resource still prompts, and a denial stays
  until the user changes it in System Settings.
- App-container access may prompt again for each Hive process unless the user
  grants Full Disk Access. MDM can deploy file privileges with a PPPC profile but
  cannot preapprove Local Network access.
