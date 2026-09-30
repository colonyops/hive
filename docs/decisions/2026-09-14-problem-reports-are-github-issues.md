# Problem reports are GitHub issues

- **Status:** accepted
- **Date:** 2026-09-14

Supersedes ADR [in-app-problem-reporting](2026-07-27-in-app-problem-reporting.md).

## Context

The app uploaded a redacted diagnostic bundle to a private R2 bucket behind a
shared bearer token stamped into release builds (ADR in-app-problem-reporting).
That existed because the repository was private and had no issue tracker. It is
public now, and `.github/ISSUE_TEMPLATE/bug.yml` asks for the same facts the
bundle carries.

The upload path also cost a whole branch of the release flow: a worker route, an
R2 bucket, a secret in two places, and a preflight in `cmd/release` that verified
the release token before shipping. Source builds had no token and failed closed,
so the feature behaved differently in development and in a release.

## Decision

1. **Reporting a bug and building a bundle are two actions, not one.**
   "Report a problem" opens `issues/new?template=bug.yml` carrying the version
   and the OS/arch/commit line, and nothing else. "Save a diagnostic bundle" is
   a separate command that writes a file and opens no browser. They must not be
   recombined.

   The split is the whole decision. Redaction removes credentials, not identity:
   a log tail names the user's home directory, repositories and branches, and
   flows name the orgs and hosts they poll. That content is acceptable in a
   bucket only the maintainer reads. It is not acceptable in a world-readable
   issue. Any flow that ends with "now attach this to the issue" walks the user
   into publishing it.

2. **The bundle is written to disk, never transmitted.** `ReportService.Save`
   assembles the redacted bundle, gzips it, and writes
   `<DataDir>/reports/hive-report-<id>.json.gz`. `report.Uploader`, the stamped
   token, the `/api/report` route, and the `hive-desktop-reports` bucket are
   removed. No replacement transport is added: the user sends the file to a
   maintainer through a private channel the maintainer names when they ask for
   it. The app ships no address, so nothing is scraped out of a public binary.

3. **Every surface beyond build info is opt-in and defaults off.** Logs,
   settings, flows and actions each have their own switch, labelled with what it
   exposes rather than with what it is. The previous "basic info" group bundled
   safe build facts with the log tail, which cannot survive a default-off rule.

4. **`ErrorDialog` opens the issue and prefills no error text.** It filed a
   report with every surface attached in one click, which is the one thing a
   public tracker forbids. An error string names flows, nodes and repositories,
   so it reaches an issue by the user's own paste, never by a prefill.

5. **The bug template asks for no bundle.** A field inviting one is a field
   inviting a user to publish their paths and repository names.

## Consequences

- The release pipeline has no report-token preflight. `verifyWeb` proves the
  worker is live with `GET /api/latest?channel=__probe__`, which stops at the
  worker's own channel validation (400) without reading the manifest bucket.
- Source builds and release builds no longer differ here.
- The bundle cap is GitHub's 25 MB attachment ceiling rather than the worker's
  5 MB body limit.
- The connected-account list left the bundle. It was the most identifying field
  in it (self-hosted hostnames and usernames) and the least diagnostic.
- The maintainer receives no bundle by default, and must ask for one and name a
  private channel. That is slower per bug and is the intended trade.
- #88 (list and pull down problem reports) has no subject.
