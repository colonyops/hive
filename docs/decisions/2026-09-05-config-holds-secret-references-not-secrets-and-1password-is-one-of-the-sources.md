# Config holds secret references, not secrets, and 1Password is one of the sources

- **Status:** accepted
- **Date:** 2026-09-05

## Context

Credentials must never sit in config, which is often committed with
dotfiles. Connectors keep their values in the keychain behind a
`credentials.Ref` and a connect flow, but settings such as the telemetry
token have no connect flow. An environment variable does not work for a
shipped build either: macOS starts an `.app` with a minimal environment and
the app reads no env files.

## Decision

1. **A secret-bearing config field holds a reference, and a literal is
   rejected.** `internal/platform/secrets` wraps `appkit/secret` with
   `Resolve(ref)` and `HasKnownPrefix(ref)`. `appkit/secret` passes an
   unprefixed value through as a literal, so validation calls
   `HasKnownPrefix` on every field that must not hold one. The sources are
   `env:NAME`, `file:/path`, and `op://<vault>/<item>/<field>`.

   **A field beside a secret may take a reference without requiring one.**
   `telemetry.endpoint` and `telemetry.instance_id` name a destination, not
   a credential, but accepting a reference lets one 1Password item hold a
   whole destination. Every such field is resolved; only the secret fields
   are checked for a prefix. A shape check such as the endpoint's https rule
   applies to a written-out value at load and to a reference after it
   resolves, because resolving inside `Validate` would shell out to a secret
   manager on every settings save.

2. **`op://` is a registered source.** The value in config is exactly the
   string 1Password's "Copy Secret Reference" produces, handed to
   `op read --no-newline`.

3. **`op` is located through `execenv.SearchDirs()` after PATH**, for the
   same reason as `tmuxbin.Locate` (ADR tmux-discovery): the Dock gives an
   `.app` a PATH with no package manager's bin directory.

4. **`file:` is what makes a shipped build work**, since it does not depend
   on how the app was launched. `env:` suits headless and CI.

5. **References resolve once, at load.** A rotated secret needs a relaunch,
   which is the trade for a source that can raise a Touch ID prompt.

6. **Registration is package-variable initialization, not `init()`**
   (`gochecknoinits` is on). Importing `secrets` is what makes `op`
   available.

7. **`Settings` stores the reference as a plain `string`, not an
   `appkit/secret.Secret`.** `Secret` marshals as `[redacted]`, and saving
   settings marshals the whole struct, so a `Secret` field would overwrite
   the user's reference on the first save.

## Consequences

- A 1Password read can block on a person, so it is bounded at 30s, runs
  once at startup, and is skipped when the section that names it is
  disabled. A failed resolve disables that telemetry destination with the
  reason logged rather than failing startup.
- `op` is not a dependency. A config that names no `op://` reference never
  looks for it.
- A new source is one `Register` call and one entry in `KnownPrefixes`.
- Users today are the desktop `telemetry` block and the drive server's
  `smtp.password`. Existing `credentials.Ref` fields stay in the keychain.
