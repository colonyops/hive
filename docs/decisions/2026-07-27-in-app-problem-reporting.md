# In-app problem reporting to a private R2 bucket

- **Status:** superseded by ADR [problem-reports-are-github-issues](2026-09-14-problem-reports-are-github-issues.md)
- **Date:** 2026-07-27

The repository is public and has an issue tracker. The bundle builder and the
redactor survive; the upload path, the worker route, the shared token and the
bucket are gone.

## Context

Users had no way to report a bug from inside the app, and asking them to find
and attach the log file by hand loses the build info and config that make a
report actionable. Those artifacts also carry secrets — the webhook source's
`secret`, action `env` maps, and anything token-shaped in a log line — so a
naive "attach everything" is a credential leak.

The existing publishing stack is Cloudflare R2 behind the `hive-desktop-web`
worker (ADR r2-manifest-distribution, 0005). The original ask said S3; there is no reason reports
must live in AWS, and R2 gives a native worker binding with no credentials in
the worker and no egress cost.

## Decision

1. **Assemble and redact Go-side, in `internal/app/report`.** A `ReportService`
   on `App` builds the bundle (build/system info, a bounded 256 KB log tail,
   and a redacted config snapshot), gzips it, and POSTs it. The webview never
   sees raw config — redaction and size caps live in one place.

2. **Redaction defaults to removal.** Config is decoded to generic maps and
   scrubbed three ways: secret-named keys (`token`, `secret`, `api_key`, …) are
   dropped; arbitrary-key carrier maps (`env`, `headers`, `secrets`) have every
   value dropped; and token-shaped strings (GitHub PATs, bearer tokens) are
   scrubbed everywhere, including the log tail and the user's own description.
   A test plants secrets in every surface and asserts none survive into the
   compressed payload. The SQLite pipeline DB is deliberately excluded — larger
   and higher redaction risk than it is worth for a first cut.

3. **Ingest is a route on the existing worker, not a new one.** `POST
   /api/report` sits beside `/api/latest` and `/api/subscribe`. A separate
   worker would isolate a report-endpoint bug from the landing page, but doubles
   the wrangler project, deploy workflow, and domain for a private-beta tool;
   the route is wrapped so a failure returns 5xx rather than affecting assets.

4. **The endpoint bar: shared token + gzip + size cap, IP rate-limit at the
   edge.** The worker requires `Authorization: Bearer <REPORT_TOKEN>`,
   `Content-Encoding: gzip` (verified by magic bytes), and a 5 MB ceiling, and
   builds the object key server-side from its own clock
   (`reports/YYYY/MM/DD/<id>.json.gz`) so a client cannot choose where its
   report lands. The token is baked into the client (`-X …wailsui.reportToken`)
   and is therefore extractable — it stops opportunistic abuse, not a determined
   attacker, so IP rate-limiting is a Cloudflare WAF rule (see
   `distribution.md`), not worker code. With no token stamped in, the client
   disables reporting and the worker answers 503 — reporting fails closed.

5. **Private bucket, no public path.** `hive-desktop-reports` is a new bucket
   with no custom domain and no public access; the worker binding is the only
   writer. Reports are read with `wrangler r2 object get`. A lifecycle rule
   expires objects after 90 days.

## Consequences

- Enabling reporting is an infra step, not a code change: create the bucket, set
  the `REPORT_TOKEN` worker secret, and stamp the matching `-X` into the release
  build. Until then the feature is present but inert (503 / unavailable).
- `report.Uploader` is a driven port filled by the adapter, like `Notifier` and
  `Gate`; the endpoint domain is baked into the client per decision 0003's
  philosophy (a stable domain, never a bucket URL).
- The redactor is denylist-shaped for open-ended user YAML because a true
  allowlist over connector-specific config would strip most debug value. The
  carrier-map rule is what catches a secret stored under an arbitrary key.
- No automated worker test ships: `web/` has no test runner, and adding one for
  a single route was judged not worth a new toolchain. The request-rejection
  paths are verified by hand with `wrangler dev` (documented in the worker).
