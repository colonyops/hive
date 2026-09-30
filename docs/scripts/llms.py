"""Publish the docs for LLMs. Runs after `zensical build` (see mise.toml).

Three things land in the built site, all derived from the nav in
zensical.toml so a new page needs no registration:

- /llms.txt: one link per page in nav order (https://llmstxt.org).
- /llms-full.txt: every page's Markdown inlined, in the same order.
- <page url>.md: one page as Markdown beside its HTML (/desktop/inbox/flows/
  has /desktop/inbox/flows.md; a section index such as /cli/recipes/ has
  /cli/recipes.md).

The Markdown is what the author wrote, with the front matter removed and
relative links made absolute so it reads correctly outside the site. The
landing page is HTML dressed as Markdown and is left out.
"""

import posixpath
import re
import sys
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
FRONT_MATTER = re.compile(r"\A---\n(.*?)\n---\n", re.S)
LINK = re.compile(r"(\]\()([^)\s]+)(\))")
SCHEME = re.compile(r"^[a-z][a-z0-9+.-]*:")
TITLE = re.compile(r"^# (.+)$", re.M)


def main() -> int:
    config = tomllib.loads((ROOT / "zensical.toml").read_text(encoding="utf-8"))["project"]
    docs = ROOT / config.get("docs_dir", "docs")
    site = ROOT / config.get("site_dir", "site")
    site_url = config["site_url"].rstrip("/") + "/"
    if not site.is_dir():
        print(f"llms: {site} does not exist; run the build first", file=sys.stderr)
        return 1

    pages = [(section, src) for section, src in walk(config["nav"]) if src != "index.md"]

    index = [
        f"# {config['site_name']}",
        "",
        f"> {config['site_description']}",
        "",
        "Every page below is also served as plain Markdown at the linked URL;",
        "the HTML page is the same path without the `.md` suffix.",
        "",
    ]
    full = [
        f"# {config['site_name']} documentation",
        "",
        f"> {config['site_description']}",
        "",
        f"Every page of {site_url}, in navigation order. The index with one link per page is at {site_url}llms.txt.",
        "",
    ]

    current = None
    for section, src in pages:
        text = (docs / src).read_text(encoding="utf-8")
        meta, body = split_front_matter(text)
        title = first_title(body, src)
        description = meta.get("description", "")
        body = absolutize(body, src, site_url).strip()
        url = page_url(src)
        twin = twin_path(url)
        (site / twin).parent.mkdir(parents=True, exist_ok=True)
        (site / twin).write_text(body + "\n", encoding="utf-8")
        if section != current:
            if current is not None:
                index.append("")
            current = section
            index += [f"## {section}", ""]
        index.append(f"- [{title}]({site_url}{twin}): {description}")
        full += ["---", "", f"<!-- {site_url}{url} · {section} -->", "", body, ""]

    index += [
        "",
        "## Optional",
        "",
        f"- [Full documentation in one file]({site_url}llms-full.txt): every page above, concatenated",
        f"- [Source repository]({config['repo_url']}): the app, the docs source, and the issue tracker",
        "",
    ]
    (site / "llms.txt").write_text("\n".join(index), encoding="utf-8")
    (site / "llms-full.txt").write_text("\n".join(full), encoding="utf-8")
    print(f"llms: wrote llms.txt, llms-full.txt and {len(pages)} Markdown twins")
    return 0


def walk(nav, trail=()):
    """Yield (section title, source path) for every page in nav order.

    The section is the tab and the sidebar group joined by a separator
    ("Desktop · Inbox"), so the index reads the way the site's nav does.
    """
    for entry in nav:
        if isinstance(entry, str):
            yield " · ".join(trail[:2]) or first_title_of(entry), entry
            continue
        for title, value in entry.items():
            if isinstance(value, str):
                yield " · ".join((trail + (title,))[:2]), value
            else:
                yield from walk(value, trail + (title,))


def first_title_of(src: str) -> str:
    _, body = split_front_matter((ROOT / "docs" / src).read_text(encoding="utf-8"))
    return first_title(body, src)


def split_front_matter(text: str):
    match = FRONT_MATTER.match(text)
    if not match:
        return {}, text
    meta = {}
    for line in match.group(1).splitlines():
        key, sep, value = line.partition(":")
        if sep and not line.startswith((" ", "-")):
            meta[key.strip()] = value.strip().strip("'\"")
    return meta, text[match.end():]


def first_title(body: str, src: str) -> str:
    match = TITLE.search(body)
    return match.group(1).strip() if match else Path(src).stem


def page_url(src: str) -> str:
    if src == "index.md":
        return ""
    if src.endswith("/index.md"):
        return src[: -len("index.md")]
    return src[: -len(".md")] + "/"


def twin_path(url: str) -> str:
    return "index.md" if url == "" else url.rstrip("/") + ".md"


def absolutize(markdown: str, src: str, site_url: str) -> str:
    base = posixpath.dirname(src)

    def repl(match):
        target = match.group(2)
        if SCHEME.match(target) or target.startswith("#"):
            return match.group(0)
        if target.startswith("/"):
            return f"{match.group(1)}{site_url.rstrip('/')}{target}{match.group(3)}"
        path, _, anchor = target.partition("#")
        resolved = posixpath.normpath(posixpath.join(base, path)) if path else src
        url = page_url(resolved) if resolved.endswith(".md") else resolved
        suffix = f"#{anchor}" if anchor else ""
        return f"{match.group(1)}{site_url}{url}{suffix}{match.group(3)}"

    return LINK.sub(repl, markdown)


if __name__ == "__main__":
    sys.exit(main())
