---
name: docs-audit
description: Check the work on this branch (or a PR) for anything a user would need to read about and is not documented, then add or update the page on hivedesktop.com, for Hive Desktop or the hive CLI. Use when asked to audit the docs, to decide whether a change needs a docs page, or as a pre-PR pass over a feature that added a setting, a node, an action type, a CLI command or flag, a TUI keybinding, a surface, or a failure mode.
---

# Audit the docs

The docs site is the user's only map of the app, and its failure mode is
silent: a setting ships, a node type lands, a surface is renamed, and the page
that described it keeps describing the old one. Nothing fails a build over it.
This audit is the check.

It covers both products: `docs/docs/desktop/` and `docs/docs/cli/`.
`docs/architecture.md`, ADRs, and the shipped agent skills are other surfaces
with their own skills; step 3 says which change belongs where.

## 1. Scope the change

```bash
git diff main...HEAD --stat
git diff main...HEAD -- cmd/desktop/internal/app cmd/desktop/frontend/src   # desktop
git diff main...HEAD -- cmd/hive internal/core/config                         # CLI
```

For a PR that is not checked out, `gh pr diff <number>`.

## 2. Inventory what a user can now see, do, or hit

Read the diff for these and write them down before deciding anything:

- **a setting**: a field on `settings.Settings`, a new mock mode, a new
  environment variable read anywhere;
- **a node or action type**: a `connector.Descriptor`, a flow node, an entry
  in `actions.Doc`, a field on any of their configs, a change to a payload
  contract;
- **a surface**: a new view, area, settings section, drawer, dialog, or a
  rename of one the docs refer to by name (`Settings ▸ Integrations`,
  `Chats`, `Code`);
- **a verb or a key**: a command in `keybindings/catalog.ts`, a default combo,
  a launcher rule;
- **an agent-facing thing**: a shipped skill, an MCP catalogue entry, a
  workspace manifest field;
- **a CLI or TUI change**: a new `hive` command or flag, a config field in
  `internal/core/config`, a default in `defaultViewsConfig` or
  `defaultUserCommands`, a template variable, a status indicator;
- **a failure a user meets**: a new error message, a new precondition (a
  binary on PATH, a permission, a version floor), a removed fallback.

A refactor, a test, an internal rename, and a change to how something is
drawn all inventory to nothing. Say so and stop. An audit that finds nothing
is a valid outcome and is reported as one.

## 3. Decide where each one belongs

**A user-facing fact goes on the docs site**, on the page that already owns
its subject. The site has one tab per product. The Desktop tab mirrors the
app: its sidebar groups are the app's areas (Inbox, Code, Chats), each with
its own directory under `docs/docs/desktop/`, and Configuration is the
reference. The CLI tab under `docs/docs/cli/` has Getting Started,
Configuration, and Recipes.

| Subject | Page |
| --- | --- |
| a setting, a path, an environment variable | `docs/docs/desktop/configuration/settings.md` |
| the updater, a release channel | `docs/docs/desktop/configuration/settings.md`, the `## updates` section |
| a command id or a default combo | `docs/docs/desktop/configuration/keybindings.md` |
| items, feeds, and notifications as one model | `docs/docs/desktop/inbox/how-it-works.md` |
| a source node, what an account needs, the webhook contract | `docs/docs/desktop/inbox/sources.md` |
| a flow node type, wiring, the editor | `docs/docs/desktop/inbox/flows.md` |
| an action type, inputs, targets, launchers | `docs/docs/desktop/inbox/actions.md` |
| a workspace manifest field, a skill package, an MCP entry | `docs/docs/desktop/chats/agent-workspaces.md` |
| Code: attach, windows, launchers, typography | `docs/docs/desktop/code/terminal-mode.md` |
| a failure and its way out | `docs/docs/desktop/getting-started/troubleshooting.md` |
| the problem reporter and what a report contains | `docs/docs/desktop/getting-started/troubleshooting.md`, the `## Report a problem` section |
| installing a release | `docs/docs/desktop/getting-started/index.md`, the `## Install` section |
| sign-in, notification permission, the starter feeds | `docs/docs/desktop/getting-started/sign-in.md`, `notifications.md`, `first-feed.md` |
| building the app | `docs/docs/desktop/getting-started/build-from-source.md` |
| CLI: installing, the first session | `docs/docs/cli/getting-started/index.md` |
| CLI: sessions, clone strategies, lifecycle, status indicators | `docs/docs/cli/getting-started/sessions.md` |
| CLI: messaging, task tracking (`hc`), todos, context and review | `docs/docs/cli/getting-started/<topic>.md` |
| CLI: a config field, an environment override | `docs/docs/cli/configuration/index.md` |
| CLI: a TUI keybinding or palette command | `docs/docs/cli/configuration/keybindings.md` |
| CLI: user commands, built-in actions | `docs/docs/cli/configuration/commands.md` |
| CLI: rules, templates, template functions | `docs/docs/cli/configuration/rules.md` |
| CLI: plugins, sources, themes | `docs/docs/cli/configuration/<topic>.md` |
| CLI: the Claude Code plugin | `docs/docs/cli/getting-started/claude-plugin.md` |

A new page is rare. It is right when a subject has no owner in that table and
would not fit as a section of one, not when a feature is big. It goes in the
directory and sidebar group of the area it belongs to; a new area is a new
directory and a new group. Adding a page is the `docs-page` skill's
procedure: the file, its frontmatter, and its line in the nav in
`docs/zensical.toml`. A page left out of the nav builds without a warning and
is reachable by URL only, which is the same as not existing.

**These are not docs-site changes**, and saying so is half the audit:

- **How the app is built** goes in `docs/architecture.md`; **why a choice was
  made** goes in an ADR. A user page says what a thing does and what it needs,
  never how it is implemented.
- **A change to a config schema also changes what the agent is told.** The
  shipped skills under `cmd/desktop/internal/app/prompts/templates/` and the per-type docs
  they splice in (`flow/docs/*.md`, `actions/docs/*.md`, `mcpcatalog/docs/*.md`)
  are the agent's reference. A field documented on the site but not there
  means the Hive workspace writes it wrong. That is the `desktop-shipped-skills`
  skill's procedure; run it beside this one.
- **A user-visible change owes a release-notes fragment** in the changelog
  of the program it changes. That is the `release-notes` skill.
- **Developer-only facts** (mock modes, `launch.env`, the devserver) belong
  on the build page only to the extent a person building the app must know
  them. The rest is `docs/development.md` and `cmd/desktop/AGENTS.md`.
- **A CLI change can also owe the Claude plugin skills** under
  `claude-plugin/hive/skills/` (`hc`, `config`, the messaging skills) when it
  changes a command those skills teach. Update them in the same PR.

## 4. Write it

The `docs-page` skill covers the mechanics: frontmatter, the nav, the build.
The conventions that keep the pages consistent:

- **Write the minimum useful page.** Keep instructions, requirements, safety
  constraints, and recovery steps. Remove implementation details and reasons
  for ordinary controls.
- **Summarize settings and shortcuts.** Group visible controls in a sentence
  or short list. The app owns the complete live list. Show YAML only when a
  user needs to edit a file manually.
- **Avoid generated-sounding prose.** Do not use em or en dashes, rhetorical
  contrast formulas, "Why it matters" headings, staged reveals, or marketing
  filler. State the fact or instruction directly.
- **Keep one owner for each fact.** Link to the owner page instead of copying
  provider lists, action types, settings, or key tables onto another page.
- **Name things what the app names them.** Settings sections are
  `Settings ▸ Integrations`; the areas are Inbox, Code, and Chats; a
  workspace is a flow. Read `sectionMeta.ts` and the catalog's `group` values
  rather than guessing.
- **Say what a thing needs before what it does.** tmux 3.2, an agent on
  PATH, a scope on a token, a permission: the precondition goes first, in an
  admonition if a user would otherwise discover it by failing.
- **Point at the Hive workspace where an agent could do the work.** Every
  config page carries a short `!!! tip "Ask the Hive workspace"` naming the
  skill the seeded `Hive` workspace has for that file. Add one to a page that
  gains a config surface; do not add one to a page about a thing an agent
  cannot drive.
- **A callout is an admonition**: `!!! tip "Title"` on its own line, body
  indented four spaces. `tip`, `note`, and `info` are the types in use.
- **The body starts with `# Title`.** Add a short task-oriented lede only when
  the title needs context. The frontmatter is `icon:` and `description:` and
  its description is not repeated in the body.
- **Internal links are relative Markdown-file links**
  (`../configuration/settings.md#updates`), which the strict build validates.
- **Do not hand-maintain what is derived.** The tabs, the sidebar and its
  groups, the prev/next footer, search, the sitemap, `llms.txt`,
  `llms-full.txt`, and each page's `.md` twin all come from the nav and the
  pages. A page in the nav appears everywhere with no further edit.

A fact you cannot verify in the code does not go on the page. Read the
struct, the descriptor, or the component; the docs are reviewed as a spec.

## 5. Verify

```bash
mise run docs:build
```

(`mise run docs:install` first in a fresh worktree; both run from anywhere in
the repo.) The build runs `zensical build --strict` and aborts on any warning: a link to
a page that does not exist, an anchor that is not a heading on its target, or
a nav entry whose file is missing. Then check the two things the build
cannot:

1. **Every new page is in the nav** in `docs/zensical.toml`. The build does
   not warn about a page that is not.
2. **The page says what the code does.** Re-read the diff beside the page.
   A default, a range, a scope name, and a key are the four things most often
   wrong.

PR CI runs the same build (the `site` job), so a broken link fails the PR. It
cannot tell a wrong default from a right one; only this step can.

## Report

Say what you added or changed, page by page, and say what you decided
**not** to document and why. "This is internal shape and belongs in
architecture.md", "the shipped skill needs the same field", and "no user can
see this" are all conclusions worth writing down, because the next audit
re-asks the same questions.
