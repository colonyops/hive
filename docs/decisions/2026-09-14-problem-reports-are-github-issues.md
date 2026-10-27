# Problem reports are GitHub issues

- **Status:** accepted
- **Date:** 2026-09-14

## Context

The repository is public with an issue tracker, and its bug template asks
for the facts a diagnostic bundle carries. A bundle's redaction removes
credentials, not identity: a log tail names the user's home directory,
repositories and branches, and flows name the orgs and hosts they poll. That
content must never land in a world-readable issue.

## Decision

1. **Reporting a bug and building a bundle are two actions, not one.**
   "Report a problem" opens `issues/new?template=bug-desktop.yml` with only the
   version and environment fields filled (`report.IssueURL`). "Save a
   diagnostic bundle" writes a file and opens no browser. They must not be
   recombined: any flow that ends with "now attach this to the issue" walks
   the user into publishing it.

2. **The bundle is written to disk, never transmitted.** `ReportService.Save`
   writes a redacted, gzipped `hive-report-<id>.json.gz` under the reports
   directory. Redaction happens in Go (`report.Redact`) and defaults to
   removal: secret-named keys are dropped, every value under a carrier map
   (`env`, `headers`, `secrets`) is dropped, and token-shaped strings are
   scrubbed everywhere, including the log tail. The user sends it to a maintainer through a private channel
   the maintainer names when they ask. The app ships no upload address.

3. **Every surface beyond build info is opt-in and defaults off.** Logs,
   settings, flows and actions each have their own switch, labelled with what
   it exposes.

4. **`ErrorDialog` opens the issue and prefills no error text.** An error
   string names flows, nodes and repositories, so it reaches an issue only by
   the user's own paste.

5. **The bug template asks for no bundle.**

## Consequences

- Source builds and release builds behave the same; there is no report
  token or upload service.
- The bundle cap is GitHub's 25 MB attachment ceiling
  (`report.MaxBundleBytes`).
- The connected-account list is not in the bundle: it was the most
  identifying field and the least diagnostic.
- A maintainer gets no bundle by default and must ask for one. That is the
  intended trade.
