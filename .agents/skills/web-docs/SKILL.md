---
name: web-docs
description: Add, edit, or restructure a page on the public documentation site at hivedesktop.com, the Zensical site under docs/ whose pages live in docs/docs/ and whose nav lives in docs/zensical.toml. Use only for user-facing product docs; docs/architecture.md, ADRs, and in-app copy are not this site.
compatibility: Requires mise. The root mise.toml pins Python and uv; run `mise run docs:install` once in a fresh worktree (docs/.venv is gitignored). PR CI builds the site in the `site` job, so a broken link fails the PR.
---

# Update the docs site

The site is Zensical, the successor of Material for MkDocs by the same team,
configured in `docs/zensical.toml`. `docs/AGENTS.md` describes the site as a
whole: the two product tabs, the landing page, the derived files, the
redirect stubs, and the deploy. This skill is the mechanics of one page.

To decide *whether* a change needs a page, and which one, use the
`docs-audit` skill.

## Where a page lives

`docs/docs/<product>/<directory>/<slug>.md`. The file's path is its URL:
`desktop/inbox/flows.md` is `/desktop/inbox/flows/`, and a directory's
`index.md` is its root (`desktop/getting-started/index.md` is
`/desktop/getting-started/`).

The nav has two tabs beside Home (`index.md`, the landing page), one per
product, each with sidebar groups:

- **Desktop** (`docs/docs/desktop/`): **Getting Started**
  (`getting-started/index.md` with the `## Install` section, `features.md`,
  then `agent-and-repos.md`, `sign-in.md`, `notifications.md`,
  `first-feed.md`), **Inbox** (`inbox/`), **Code** (`code/`), **Chats**
  (`chats/`), **Configuration** (`configuration/settings.md`,
  `keybindings.md`), and **Resources** (`getting-started/build-from-source.md`,
  `troubleshooting.md`).
- **CLI** (`docs/docs/cli/`): **Getting Started**, **Configuration**,
  **Recipes**, and `faq.md`.

A page about something the user does in Inbox goes in `desktop/inbox/` and
the Inbox group. Build and support pages go in Resources. A setting or key
goes on its Configuration page. A page that describes something both
products share, such as sessions or the task tree, lives with the product
that owns the feature and is linked from the other.

## The nav is hand-maintained

`docs/zensical.toml` holds the nav, and the nav is the site's structure. Its
order is the tab order and the sidebar order, a nested table is a sidebar
group, and a nav entry is a bare path, so the page names itself with its
`# Title`. Adding a page is two edits: the file and its line in the nav.

A page that is not in the nav still builds and is reachable by URL, and the
strict build does not warn about it. It has no tab or sidebar entry, no line
in `llms.txt`, and no Markdown twin, so nobody finds it. Check the nav
whenever you add a file.

## Frontmatter and body

```yaml
---
icon: lucide/settings   # shown beside the title in the nav
description: Change Hive Desktop preferences in the app or through settings.yaml.
---

# Settings

Open Settings with <kbd>⌘,</kbd> or from the command palette.

## Configuration files
```

- `icon` is a `lucide/<name>` icon.
- The body starts with `# Title`, followed by a short task-oriented lede when
  the title needs context. Do not repeat the frontmatter description.
- `description` becomes the page's line in `llms.txt`
  (`docs/scripts/llms.py` reads the frontmatter).

`docs/docs/desktop/getting-started/index.md` and
`docs/docs/desktop/inbox/sources.md` are useful models.

## Writing conventions

- **Keep it short.** Include what a user needs to complete a task, avoid data
  loss, meet a requirement, or recover from a failure. Remove implementation
  detail and explanations of ordinary controls.
- **State facts directly.** Avoid em and en dashes, rhetorical contrasts such
  as "not X, but Y", "Why it matters" headings, staged reveals, and marketing
  filler.
- **Do not duplicate an owner page.** Link to the page that owns a subject.
  Sources owns provider support; Settings owns configuration locations; the
  shortcut dialog owns the complete live shortcut list.
- **Summarize visible options.** Use a sentence or bullets for groups such as
  fonts, terminal spacing, notification delivery, and update channels. Do not
  document every visible setting or explain why someone might change it.
- **Use exact configuration only when needed.** A short YAML example is useful
  for manual-only settings and file formats. Do not reproduce a whole schema
  already available in the app or a shipped skill.
- **Callouts are admonitions.** `!!! tip "Title"` on its own line, body
  indented four spaces. The pages use `tip`, `note`, and `info`; `???` in
  place of `!!!` makes one collapsible (`pymdownx.details`). Use one for a
  precondition a user would otherwise discover by failing, and for the Hive
  workspace pointer on a config page.
- **Content tabs** are `=== "macOS"` / `=== "Linux"` blocks, body indented
  four spaces (`pymdownx.tabbed`). The `## Install` section of
  `desktop/getting-started/index.md` is the example.
- **Point config pages at the Hive workspace.** The app seeds a Chats
  workspace named `Hive` carrying every shipped `hive-*` skill. A page about
  a file the agent can edit carries a short `!!! tip "Ask the Hive workspace"`
  naming that skill (`hive-settings`, `hive-flows`, `hive-actions`, ...).
- **Name things what the app names them.** `Settings ▸ Integrations`, the
  Inbox / Code / Chats areas, workspace, feed. Check
  `cmd/desktop/frontend/src/components/settings/sectionMeta.ts` and
  `cmd/desktop/frontend/src/keybindings/catalog.ts` before writing a label.
- **Keys are `<kbd>` elements**: press `<kbd>g</kbd>` then `<kbd>a</kbd>`.
- **Internal links are relative Markdown-file links**, with an anchor when
  one is needed: `../configuration/settings.md#updates`, `sign-in.md`, and
  `../../cli/configuration/index.md` for the other product. The strict build
  validates them, and `docs/scripts/llms.py` rewrites them to absolute URLs
  in the Markdown twins. Never link the other product by URL.
- Mermaid fences, `attr_list`, `md_in_html`, and emoji shortcodes are
  enabled in `docs/zensical.toml`.

## Validate

```bash
mise run docs:install   # first time in a fresh worktree
mise run docs:build     # zensical build --clean --strict, then llms.py and redirects.py
mise run docs:serve     # browser live reload on http://127.0.0.1:8000
```

`--strict` aborts the build on any warning. A link to a page that does not
exist, a link to an anchor that is not a heading on its target, and a nav
entry whose file is missing all report `page does not exist` with a
file:line:column. Strict does not catch a page absent from the nav (see
above) or a fact that is wrong, so re-read the diff beside the page.

The download buttons and the `## Install` panel read the release manifest
from `dl.hivedesktop.com`; on a local serve they sit in their fallback state.

PR CI runs the same build (the `site` job in `.github/workflows/ci.yml`), so
a broken link fails the PR rather than the deploy.

## Guardrails

- **Never deploy.** The site deploys to GitHub Pages from the publish
  workflow after a CLI release (`.github/workflows/deploy-site.yml`).
- A page that moves gets a row in `docs/scripts/redirects.py`, so its old URL
  still lands somewhere.
- `docs/site/` is build output; `docs/.venv/` and `docs/.cache/` are local
  state. All are gitignored. Never edit a file under `docs/site/` and never
  commit any of them.
- These pages are product documentation for users. Internal shape belongs in
  `docs/architecture.md`, decisions in `docs/decisions/`, and in-app copy in
  the app.
