# Agent Instructions — hivedesktop.com

Scope: `docs/`, the public site at `hivedesktop.com`: the landing page and
the product docs for the hive CLI and Hive Desktop. The repository root
`AGENTS.md` still applies.

## What the site is

A Zensical site, the successor of Material for MkDocs by the same team.
`zensical.toml` holds the theme, the markdown extensions, and the nav. Pages
are Markdown under `docs/docs/`. The build writes `docs/site/`, which GitHub
Pages serves at the custom domain (`docs/docs/CNAME`).

`tasks.toml` owns the site's tasks; the root `mise.toml` includes it and pins
Python and uv. Zensical is pinned in `pyproject.toml` and resolved into
`uv.lock`. Every task runs inside `docs/`, from anywhere in the repo:

| Task | What it runs |
| --- | --- |
| `mise run docs:install` | `uv sync --frozen`; once per fresh worktree |
| `mise run docs:build` | `zensical build --clean --strict`, then `scripts/llms.py` and `scripts/redirects.py` |
| `mise run docs:serve` | `zensical serve` with browser live reload on http://127.0.0.1:8000 |
| `mise run docs:lock` | `uv lock`, after editing `pyproject.toml` |

The root `check` task does not build the site, so a contributor without
Python can run it. CI's `site` job and the root `ci` task do.

## Pages

A page is `docs/docs/<product>/<directory>/<slug>.md` plus a line in the nav
in `zensical.toml`. The nav is the site's structure: its order is the tab and
sidebar order, a nested table is a sidebar group, and a page that is not
listed builds without a warning but has no nav entry, no `llms.txt` line, and
no Markdown twin.

Two tabs beside Home, one per product, with the same sidebar shape:

- **Desktop** (`docs/docs/desktop/`): **Getting Started** (`getting-started/index.md`
  with the `## Install` section, `features.md`, then the first-run pages),
  **Inbox** (`inbox/`), **Code** (`code/`), **Chats** (`chats/`),
  **Configuration** (`configuration/settings.md`, `keybindings.md`), and
  **Resources** (`getting-started/build-from-source.md`, `troubleshooting.md`).
- **CLI** (`docs/docs/cli/`): **Getting Started** (`getting-started/`),
  **Configuration** (`configuration/`), **Recipes** (`recipes/`), and `faq.md`.

A page goes in the group for its product and area. A page that describes
something both products share, such as sessions or the task tree, lives with
the product that owns the feature and is linked from the other.

Frontmatter is `icon: lucide/<name>` and `description:`. The body starts with
`# Title`; do not repeat the frontmatter description below it. Callouts are
admonitions (`!!! tip "Title"`, body indented four spaces), content tabs are
`=== "macOS"`, keys are `<kbd>`, and internal links are relative
Markdown-file links that the strict build validates. A link from one product
to the other is the same kind of link (`../../cli/configuration/index.md`);
never link the other product by URL.

Keep pages short and task-focused. State facts directly. Avoid em and en
dashes, rhetorical contrasts, "Why it matters" headings, staged reveals, and
marketing filler. Do not explain every visible control. Keep each fact on the
page that owns it and link there instead of repeating it.

The landing page is `docs/docs/index.md`, HTML sections styled by
`docs/docs/stylesheets/extra.css`: a hero with a download button for the desktop
app and an install link for the CLI, a strip linking to the four showcase
sections (CLI, Feeds, Code, Chats), each pairing copy with a demo, and a CTA.
The CLI demo is an animated terminal in CSS; the desktop demos are videos
under `assets/demos/` (`feeds.mp4`, `code.mp4`, `chats.mp4`), swapped for a
"coming soon" placeholder by `javascripts/demos.js` when the file is missing.

Files under `docs/docs/` that are not Markdown are copied to the site root
unchanged: `CNAME`, `install.sh` (the desktop installer, fetched by
`curl https://hivedesktop.com/install.sh`), `robots.txt`,
`assets/favicon.svg`, `assets/hive-desktop.png` (the Linux app-menu icon the
installer downloads), and `javascripts/download.js`, which fills the hero and
CTA download buttons and the `## Install` section's download panel from the
release manifest at `dl.hivedesktop.com`. That bucket must allow cross-origin
reads from `https://hivedesktop.com`; without the rule the buttons keep their
fallback links to the install section.

After the build, `scripts/llms.py` derives `site/llms.txt` (one link per
page), `site/llms-full.txt` (every page inlined), and a Markdown twin of every
nav page at its URL plus `.md`. `overrides/main.html` adds a
`<link rel="alternate" type="text/markdown">` pointing at `/llms.txt` to
every page.

## Old URLs

GitHub Pages cannot redirect, so `scripts/redirects.py` writes a stub page for
every URL the site used to serve: the desktop docs before they moved under
`/desktop/` (installed app builds link `/getting-started/` and
`/configuration/settings/#updates` from the About pane), the pre-Zensical
`/docs/...` paths, and the CLI docs before they moved under `/cli/` (the
`colonyops.github.io/hive/` address redirects to this domain with the path
kept). The table is frozen at the merge: a page that moves later gets a new
row there, and the script fails when a row would overwrite a real page.

## CI and deploy

`.github/workflows/ci.yml` runs `mise run docs:build` on every PR that
touches `docs/` (the `site` job), so a broken link or a missing nav file
fails the PR. `.github/workflows/deploy-site.yml` runs the same build on a
push to `main` that touches `docs/**` and deploys `docs/site/` to GitHub
Pages.

## Guardrails

- `docs/site/` is build output; `.venv/` and `.cache/` are local state. All
  are gitignored. Never edit a file under `site/` and never commit any of them.
- This is user-facing product documentation only. Contributor docs, ADRs,
  and in-app copy are not this site.
