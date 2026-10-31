# Applets are web pages with full access to the machine

- **Status:** accepted
- **Date:** 2026-10-31

## Context

A user should be able to ask an agent for a UI, from a Docker viewer to something with
no place in Hive's design, such as a game, and open it from the app. These applets are
written by or for the person whose machine they run on, so restricting what they can do
protects nobody and costs most of the value. A schema of tables and pickers was tried
first and removed: it could not express the applets people wanted, and a page does what
it did in a few lines.

## Decision

An applet is a directory, `<ConfigDir>/applets/<id>/`, holding an `applet.yaml` that
names it and the page that is the applet. The page is served from the directory through
the app's asset handler, with `window.hive` injected before its scripts. `hive` runs any
command and reads or writes any file. There is no allowlist and no permission prompt.

The frame is sandboxed with an opaque origin. This does not limit what an applet can
do, since the bridge can run anything. It keeps a page's mistakes off the app: a stray
`localStorage.clear()` or a global stylesheet cannot damage Hive. `localStorage` is
shimmed and persisted per applet, because generated pages expect it. Hive's theme crosses
into the frame as `--hv-*` variables on the page's root, so a page can follow it.

Hive ships one applet, the Applet Builder, in the binary. It asks what the applet should
look like, and hands the user a prompt for an agent. The words of that prompt stay
in Go (`prompts/templates/applets.tmpl`), and the page sends only the answers. A directory
of the same name replaces a built-in.

Unrestricted applies to the applet the user opened, not to whatever else can reach the
bridge. Three rules hold that line:

- **A page proves itself with a token.** A frame keeps its window when it navigates, so
  `event.source` alone would answer a remote page reached through a link. The token is
  injected only into pages `Asset` serves, and those responses carry no CORS header, so
  a page in the frame cannot fetch one to read it.
- **The native build serves pages through the asset handler**, not the loopback server,
  so nothing outside the webview can read an applet's files or reach the bridge.
- **The headless server build (`-tags server`) has no applets.** There the asset handler
  and the Wails runtime are an unauthenticated TCP listener, and any local process or
  website could use them. The one exception is the e2e harness, identified by its per-run
  marker, which runs in a Docker container with no network, or with only the test
  fixture's network in the remote lane.

Nothing runs when a file loads. Commands start when the user opens an applet and its page
runs them.

## Consequences

- An applet has the user's full access, and so does any remote script its page loads.
  Opening an applet from someone else is running their code.
- A page cannot `fetch` another HTML file of its own, because that response has no CORS
  header. It can navigate to one.
- Cookies and IndexedDB do not work in an opaque origin.
- Keys in a frame do not reach the app's keymap, so the injected script forwards
  Escape, and a page keeps it by calling `preventDefault`.
- A page falls back to its own directory on a server session: commands run on this
  machine, as a pop-up terminal's do.
- A built-in does not update on disk, because it is not on disk. Its saved `localStorage`
  is, in the directory a copy would use.
