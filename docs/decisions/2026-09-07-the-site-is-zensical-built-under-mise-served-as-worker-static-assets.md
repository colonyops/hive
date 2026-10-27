# The site is Zensical, built under mise, served from GitHub Pages

- **Status:** accepted
- **Date:** 2026-09-07

## Context

hivedesktop.com documents both the hive CLI and Hive Desktop. A home-grown
docs layout is a second docs system to maintain, and the CLI's docs were
already a Zensical site (the successor of Material for MkDocs).

## Decision

The site is a Zensical project under `docs/`. `docs/zensical.toml` holds the
theme, the markdown extensions, and the nav. Pages are Markdown under
`docs/docs/`. The landing page, `docs/docs/index.md`, is HTML sections
styled by `docs/docs/stylesheets/extra.css`.

Zensical is pinned in `docs/pyproject.toml` and resolved into
`docs/uv.lock`, so the build is reproducible and the repository's
release-age policy applies to it. The tasks are `docs:install`,
`docs:build`, `docs:serve` and `docs:lock` in `docs/tasks.toml`.

`docs:build` runs `zensical build --strict`, then `scripts/llms.py`, which
derives `llms.txt`, `llms-full.txt` and a Markdown twin of every page from
the nav, then `scripts/redirects.py`, which writes redirect stubs for old
URLs. Zensical has no plugin surface, so these are scripts.

The built site deploys to GitHub Pages through
`.github/workflows/deploy-site.yml`, which the release's publish workflow
calls and which can also be dispatched by hand. GitHub Pages cannot answer
with a 301, so an old URL gets a stub page that redirects the browser.

## Consequences

- Adding a page is two edits: the file and its nav entry. A page left out of
  the nav still builds but has no sidebar entry and no line in the llms
  outputs.
- The landing page has no component model; a change is an edit to
  `index.md` and `extra.css`.
- The site builds in CI (the `site` job) and goes live with the next release.
- The redirect table is frozen per move: a page that moves gets a new row in
  `redirects.py`.
