# Config holds secret references, not secrets, and 1Password is one of the sources

- **Status:** accepted
- **Date:** 2026-09-05

## Context

"Never put a token in config" has been a standing rule, and until now it was
enforced by having no field to put one in: a connector names a
`credentials.Ref` and the value lives in the keychain, reached through a connect
flow in the UI.

Telemetry has no connect flow, so its token was read from a fixed environment
variable instead. That does not survive contact with a shipped build. macOS
starts an `.app` with a minimal environment, and **the app reads no env files**
— `launch.env` and `overrides.env` are loaded by mise, and only for the `dev`
task. The variable was therefore set under `mise run dev` and essentially never
set for a real install.

`appkit/secret` already solves the general shape. A `Secret` resolves a
prefixed value at unmarshal time — `env:NAME`, `file:/path` — and `Register`
adds sources.

## Decision

1. **A secret-bearing config field holds a reference, and a literal is
   rejected.** `internal/app/secrets` wraps `appkit/secret` with
   `Resolve(ref)` and `HasKnownPrefix(ref)`. Validation calls the second, so
   pasting a credential into `settings.yaml` is a config error rather than
   something that quietly works. `appkit/secret` passes an unprefixed value
   through as a literal by design — that is right for a library and wrong for
   this config, so the check lives here.

   **A field beside a secret may take a reference without requiring one.**
   `telemetry.endpoint` and `telemetry.instance_id` name a destination rather
   than a credential, and are ordinarily written out; accepting a reference
   there is what lets one 1Password item hold a whole destination instead of
   splitting it across the file and the vault. Requiring one would be wrong —
   an endpoint is not a secret. So the rule is per field: `Resolve` on every
   one, `HasKnownPrefix` only on the ones that must not hold a literal.

   The cost is that a shape check must be conditional. `telemetry.endpoint`'s
   https rule cannot apply to a reference, whose target is unknown until
   launch, and resolving inside `Validate` would shell out to a secret manager
   on every settings save. A written-out endpoint is therefore checked at load
   and a reference at construction, where the resolved value is checked
   either way.

2. **`op://<vault>/<item>/<field>` is a registered source.** The prefix cut
   leaves `//vault/item/field`, which the resolver rebuilds into the canonical
   reference and hands to `op read --no-newline`. The value in config is
   therefore exactly the string 1Password's own "Copy Secret Reference" puts on
   the clipboard, with nothing to translate.

3. **`op` is located through `execenv.SearchDirs()`, not PATH alone.** Same
   reason as `tmuxbin.Locate` (ADR tmux-discovery): the Dock gives an `.app`
   `/usr/bin:/bin:/usr/sbin:/sbin`, and a Homebrew `op` is not on it. PATH is
   still tried first so an explicitly installed one wins.

4. **`file:` is what makes a shipped build work.** It reads a path, not the
   environment, so it is the source that does not care how the app was
   launched. `env:` remains right for headless and CI.

5. **References resolve once, at load.** A rotated secret needs a relaunch.
   That is the deliberate trade for a source that can raise a Touch ID prompt:
   resolving per request would prompt per request. It also keeps the OTLP
   exporters on a static `WithHeaders` map rather than needing a RoundTripper.

6. **Registration is package-variable initialization, not `init()`.**
   `gochecknoinits` is on, and `appkit/secret.Register` must run before the
   first resolve. `var _ = register()` in `secrets`, imported by `settings`, is
   what orders those two.

7. **The `Secret` type itself is not stored in `Settings`.** `Secret` marshals
   as `[redacted]`, and `saveSettingsAt` marshals the whole struct back to
   `settings.yaml` — so a `Secret` field would overwrite the user's reference
   with the literal string `[redacted]` on the first save. The field stays a
   plain `string` reference and resolution happens at the point of use.

## Consequences

- `settings.yaml` stays safe to commit: it names where a credential lives, and
  the check makes that structural rather than advisory.
- The reference vocabulary is the extension point. A keychain-backed source is
  a `Register` call and one entry in `KnownPrefixes`, not a new mechanism, and
  it is how `credentials.Store` would eventually be reachable from config.
- A 1Password read can block on a person approving a prompt, so it is bounded
  at 30s, runs once at startup, and is skipped entirely when the section that
  names it is disabled — a turned-off `telemetry` block must never raise a
  prompt. An unattended launch with a locked vault
  resolves to an error, which disables telemetry with the reason logged rather
  than failing startup.
- `op` is not a dependency. A config that never names an `op://` reference
  never looks for it.
- The `telemetry` block is the only user today. Nothing forces an existing
  `credentials.Ref` field to migrate, and none should until there is a reason.
