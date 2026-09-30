"""Write redirect stubs for URLs the site used to serve. Runs after
`zensical build` (see mise.toml).

GitHub Pages cannot answer a request with a 301, so every old URL gets a
page that sends the browser on: a meta refresh, a canonical link for
crawlers, and a script that keeps the fragment. Old app builds link the
desktop docs at their pre-merge paths (/getting-started/, /configuration/
settings/#updates) and at the paths before the site became a Zensical
site (/docs/...); the CLI's README and the GitHub Pages redirect from
colonyops.github.io/hive/ bring visitors to the CLI's pre-merge paths.

The table is frozen at the merge. A page that moves later gets a new row;
a page that is new does not.
"""

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SITE = ROOT / "site"

DESKTOP = [
    "getting-started/",
    "getting-started/features/",
    "getting-started/agent-and-repos/",
    "getting-started/sign-in/",
    "getting-started/notifications/",
    "getting-started/first-feed/",
    "getting-started/build-from-source/",
    "getting-started/troubleshooting/",
    "inbox/how-it-works/",
    "inbox/flows/",
    "inbox/sources/",
    "inbox/actions/",
    "inbox/menu-bar/",
    "code/terminal-mode/",
    "chats/agent-workspaces/",
    "configuration/settings/",
    "configuration/keybindings/",
]

# Where /getting-started/ and /configuration/keybindings/ collide, the desktop
# page wins: the installed app links them, and this is its domain.
CLI = [
    "getting-started/sessions/",
    "getting-started/context/",
    "getting-started/messaging/",
    "getting-started/task-tracking/",
    "getting-started/todos/",
    "getting-started/session-picker/",
    "getting-started/claude-plugin/",
    "configuration/",
    "configuration/rules/",
    "configuration/commands/",
    "configuration/todos/",
    "configuration/plugins/",
    "configuration/sources/",
    "configuration/themes/",
    "recipes/",
    "recipes/sequential-chain-review/",
    "recipes/parallel-code-review/",
    "recipes/inter-agent-code-review/",
    "recipes/ralph-loop/",
    "recipes/git-backed-context/",
    "faq/",
]

LEGACY = {
    "docs/": "desktop/getting-started/",
    "install/": "desktop/getting-started/#install",
    "compare/": "",
    "docs/concepts/how-it-works/": "desktop/inbox/how-it-works/",
    "docs/concepts/flows/": "desktop/inbox/flows/",
    "docs/concepts/sources/": "desktop/inbox/sources/",
    "docs/concepts/actions/": "desktop/inbox/actions/",
    "docs/concepts/terminal-mode/": "desktop/code/terminal-mode/",
    "docs/concepts/agent-workspaces/": "desktop/chats/agent-workspaces/",
    "docs/help/troubleshooting/": "desktop/getting-started/troubleshooting/",
    "docs/help/reporting-a-problem/": "desktop/getting-started/troubleshooting/#report-a-problem",
    "docs/help/updates/": "desktop/configuration/settings/#updates",
}

STUB = """<!doctype html>
<html lang="en">
<meta charset="utf-8">
<title>Redirecting to {target}</title>
<link rel="canonical" href="{target}">
<meta http-equiv="refresh" content="0; url={target}">
<meta name="robots" content="noindex">
<script>
  var target = "{target}";
  location.replace(target.indexOf("#") === -1 ? target + location.hash : target);
</script>
<p>This page moved to <a href="{target}">{target}</a>.</p>
"""


def table() -> dict[str, str]:
    rows = {old: "desktop/" + old for old in DESKTOP}
    rows.update({"docs/" + old: "desktop/" + old for old in DESKTOP})
    rows.update({old: "cli/" + old for old in CLI})
    rows.update(LEGACY)
    return rows


def main() -> int:
    if not SITE.is_dir():
        print(f"redirects: {SITE} does not exist; run the build first", file=sys.stderr)
        return 1
    for old, new in table().items():
        stub = SITE / old / "index.html"
        if stub.exists():
            print(f"redirects: {old} is a real page; drop it from the table", file=sys.stderr)
            return 1
        stub.parent.mkdir(parents=True, exist_ok=True)
        stub.write_text(STUB.format(target="/" + new), encoding="utf-8")
    print(f"redirects: wrote {len(table())} stubs")
    return 0


if __name__ == "__main__":
    sys.exit(main())
