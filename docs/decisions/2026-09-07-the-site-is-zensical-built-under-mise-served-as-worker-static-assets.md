# The site is Zensical, built under mise, served as Worker static assets

- **Status:** accepted; amends [ADR web-workers-static-assets](2026-07-23-web-workers-static-assets.md), whose hosting decision still holds
- **Date:** 2026-09-07

## Context

hivedesktop.com was an Astro build: a hand-written marketing site with a
custom docs layout on top of a content collection. Growing the docs into
product documentation (#377) exposed that the layout was a second,
home-grown docs system to maintain, and that it looked nothing like the
docs of the hive CLI (colonyops/hive), which the desktop app extends.

The hive CLI's docs are a Zensical site: the successor of Material for
MkDocs by the same team, configured in `zensical.toml`, with the `modern`
theme variant, admonitions, content tabs, navigation tabs and sections, and
per-page lucide icons in the nav.

## Decision

`web/` is a Zensical project. `web/zensical.toml` mirrors the hive CLI's
configuration (theme variant, features, palette, markdown extensions) and
holds the nav: a Getting Started tab whose sidebar groups follow the app's
areas (First run, Inbox, Code, Chats), and a Configuration tab that is the
reference. Pages are Markdown under `web/docs/`; the landing page is
`web/docs/index.md` written as HTML sections styled by
`web/docs/stylesheets/extra.css`, the same way the hive CLI's landing page
is. Astro, Pagefind, and the custom docs layout are gone.

`web/mise.toml` pins Python, uv, and Node and owns the site's tasks, run
from inside `web/`: `mise run install`, `mise run build`, `mise run dev`,
`mise run deploy`. It is a plain nested config, not a mise monorepo root:
the monorepo form was tried and rejected because it names every task
`//web:<task>`, and a separate `web/mise.lock` was accepted in exchange for
plain names. Zensical is a pinned dependency in `web/pyproject.toml`,
resolved into `web/uv.lock`, so the build is reproducible in CI and on a
contributor's machine. The hive CLI installs Zensical unpinned through pipx;
pinning is deliberate here because the release-age policy the repository
follows applies to Python packages too.

The build is `zensical build --strict` followed by `web/scripts/llms.py`,
which derives `llms.txt`, `llms-full.txt`, and a Markdown twin of every
page from the nav. Zensical has no hook or plugin surface, so this is a
script rather than a plugin.

Hosting is unchanged: the build output in `web/site/` is served as Cloudflare
Worker static assets by the Worker in `web/worker/`, which also answers the
two `/api/*` routes and redirects the pre-Zensical URLs: `/docs/*`, which the
installed app's About pane still links, `/install`, and `/compare/*`.

## Consequences

- One docs system across the two Hive products. A page written for one reads
  the same way in the other, and the `hive` skill package can point an agent
  at either.
- Adding a page is two edits: the file and its entry in `zensical.toml`'s
  nav. A page left out of the nav still builds and is reachable by URL, but
  has no tab, no sidebar entry, and no line in the llms outputs, which follow
  the nav with no further edit.
- The landing page is HTML in Markdown. It has no component model; a change
  is an edit to `index.md` and `extra.css`.
- The site builds on every PR in CI (`web-build`) and deploys from main, both
  through the same mise tasks a contributor runs.
- Python joins the repository's toolchain, scoped to `web/`.
