# Hive acts as the macOS TCC responsible process for terminal children

- **Status:** proposed
- **Date:** 2026-08-07

## Context

On macOS, Hive starts tmux, shells, agents, and their tools. Transparency,
Consent, and Control (TCC) therefore treats the signed app as the responsible
code when one of those children accesses a protected folder, another app, or a
local-network address. This is why the same command can work in Ghostty but fail
inside Hive: each host has its own privacy grants and its own declarations.

The Developer ID signature gives Hive a stable identity, but it does not grant
access. A responsible app must describe the protected resources its children
may request, and hardened runtime separately requires the Apple Events
entitlement before a child can trigger an Automation prompt. The shipping
bundle declared neither local-network use nor child-process file access, and
tccd rejects Apple Events because the entitlement is absent.

App-container protection is different. Access to another app's container is a
process-lifetime privilege, so a tool that reaches into Docker or another app's
data can prompt again after Hive restarts. There is no usage-description key or
public entitlement that turns that into a persistent grant. Full Disk Access
suppresses it, but only the user or a managed-device policy may grant that.

## Decision

Hive remains the responsible code for processes it launches. We do not use the
private responsibility-disclaim spawn API or move terminal processes into a
launchd agent to evade attribution.

Both production and development bundles declare why child commands may use
Apple Events, Desktop, Documents, Downloads, local-network devices, network
volumes, removable volumes, and system administration. Every Darwin signing
path applies the same entitlements, including
`com.apple.security.automation.apple-events`; ad-hoc builds get the capability
for functional parity even though only the Developer ID signature gives TCC a
stable release identity.

The app does not request these privileges at startup. macOS asks when a command
first performs the corresponding operation, which preserves the context for
the user's decision. We do not add App Sandbox file or network entitlements:
Hive is not sandboxed, and those entitlements do not grant TCC privileges.

Full Disk Access remains optional. Users whose tools routinely read protected
app containers may grant it to `/Applications/Hive.app`; Hive will not require
it for ordinary repositories or attempt to grant it on their behalf.

## Consequences

Local addresses such as development servers on RFC 1918 networks can trigger a
proper Hive Local Network prompt and then use the persisted per-user decision.
Folder and Automation prompts name the child command's purpose rather than
failing because the responsible bundle is incomplete.

The first use of each protected resource still prompts. A denied decision stays
denied until the user changes it in System Settings, and app-container access
may prompt again for each Hive process unless the user grants Full Disk Access.
Managed fleets can deploy persistent file privileges with a PPPC profile, but
macOS does not allow MDM to preapprove Local Network access.
