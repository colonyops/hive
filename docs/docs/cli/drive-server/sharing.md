---
icon: lucide/link
description: Share a Hive Drive document as a public link with an expiry, and what a shared HTML page can do.
---

# Share Links

A share link gives anyone who has it read access to one document. They do not need an account. The link is `<base_url>/s/<slug>`.

```bash
hive drive share plans/ship.md --expires 7d
```

- The document must be on the server. [Push it](cli.md#push) first.
- Sharing a document again returns the same link. Passing `--expires` again changes that link's expiry.
- `--expires` takes `1d`, `7d`, or `30d`. Without it, a new link does not expire.
- `--expires none` removes the expiry from an existing link.
- The server deletes an expired link. Deleting a document deletes its link.
- Link pages ask search engines not to index them. Treat the link as a secret anyway.

## HTML documents

A shared HTML document runs in a sandbox with these limits:

- Inline `<script>` and `<style>` run.
- Images and fonts work only as `data:` URIs.
- Network requests do not work: no `fetch`, no external scripts or stylesheets, no CDN libraries, no remote images or fonts, no forms, and no frames.
- The page cannot read the server's cookies or storage.

A page that loads a charting library from a CDN renders without it. Put the script and the data inline in the file before you push.
