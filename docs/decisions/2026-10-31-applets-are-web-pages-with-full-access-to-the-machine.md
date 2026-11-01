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
bridge. A frame can reach a remote page through one link, so these rules hold that line:

- **A page proves itself with a token.** A frame keeps its window when it navigates, so
  `event.source` alone would answer a remote page reached through a link. The token is
  injected only into pages `Asset` serves, and those responses carry no CORS header, so
  a page in the frame cannot fetch one to read it.
- **A page sends its token only to Hive.** A remote page could otherwise frame an applet's
  own page and read the token off its first message. The bridge posts only when its parent
  is the top window, which a page cannot fake, so `window.hive` rejects every call in a page
  nested any deeper, including an applet's own.
- **The Wails runtime takes calls only from the app's own page.** Wails runs a bound method
  on any request to `/wails/runtime`, including a `GET` an image makes, and that path has no
  token. WKWebView sends no preflight to a custom scheme, so a custom header alone proves
  nothing. A call must be a `POST` with `x-wails-client-id`, and with no `Origin` or the
  app's own.
- **The native build serves pages through the asset handler**, not the loopback server,
  so nothing outside the webview can read an applet's files or reach the bridge.
- **The headless server build (`-tags server`) has no applets.** There the asset handler
  and the Wails runtime are an unauthenticated TCP listener, and any local process or
  website could use them. The one exception is the e2e harness, identified by its per-run
  marker, which runs in a Docker container with no network, or with only the test
  fixture's network in the remote lane.

Nothing runs when a file loads. Commands start when the user opens an applet, or puts it
in the title bar, and its page runs them.

A page is not limited to the panel. An applet with `titlebar:` offers a second page, a
frame in a slot of the title bar with its own width, and that page can open a dropdown
frame under the bar. The surfaces share one bridge and differ only in what hosts them and in
one row of a table of rules, so the next place an applet can appear is that row, another host
of `AppletFrame`, and a key in `applet.yaml`. Settings, the import dialog and import cleanup
read the list of surfaces, so a new one needs no change there.

**The user turns a title bar page on, in Settings or as `applets.titlebar` in
`settings.yaml`.** It loads with the app and runs until it is turned off, so a file on disk
must not be enough to start it, for the rule above and because a page in the bar takes
room the user did not offer. This is the same reason an applet gets no default chord. It
is one switch and not a permission: once on, the page has what any applet has.

**Whatever a frame starts, that frame stops.** One applet can now have several frames, so
closing its panel must not end a process its title bar page runs. Frames of one applet
reach each other through `hive.broadcast`, and share `localStorage` by passing each change
along, because an opaque origin has no `BroadcastChannel` or `storage` event of its own and
each frame's save writes the whole store.

**An applet adds to the palette in two ways, and only one can be bound.** A row in
`commands:` is a command: it exists from the file, and a user can write its id in
`settings.yaml`. A row from `hive.setCommands` is data a running page knows about, such as a
list of stations, and its life is the page's. This is the split the rest of the palette has
between commands and object rows. A declared command carries no default chord, for the same
reason as `applet.<id>`. It goes to one frame, the running persistent surface, because
that frame is the one that keeps running and two frames acting on it would run it twice.
Today that is the title bar page. With no page to take it, the panel opens, and the bridge holds the command until the page
listens.

**Sharing is where the first rule stops holding.** This ADR's case for no restrictions is
that an applet is written by or for the person who runs it. An import breaks that, so it
gets the one warning in this design, and it still adds no permission, since an applet that
someone trusts has the access any applet has. A `.hiveapplet` is a zip holding
`hive-applet.json`, with a format number, the id and the exporting Hive version, and the files
under `files/`. The file
is hostile until the user confirms: it is checked whole in memory and installed by renaming
a finished directory in, so nothing lands early or partly. Import never replaces an applet,
because a replace is how someone else's update overwrites what a user trusted, and once
installed it clears any place, off switch or chord the id still has in `settings.yaml`, in Go,
so a file cannot inherit what the user gave another applet of that name.

**Shared files have to keep working, so both formats read forward.** A file outlives the Hive
that wrote it, and hosts in the wild keep whatever parser they shipped with. `hive-applet.json`
ignores fields it does not know, and `format` rises only for a layout an older Hive would
misread. In `applet.yaml`, `version` is the format and never the applet's own release. A key this
Hive does not know, at any depth, or an unknown icon or palette scope, is a warning and is
skipped. A wrong shape under a known key stays an error. A later `version` or an unmet `hive:`
refuses with "update Hive". Warnings quote a name only if it is plain ASCII, and skipped values
reach nothing, so a hostile file cannot use one to reach a path, a command or the page.

**Ignoring a key must not fail open on a limit.** If a later Hive adds a key that restricts an
applet, an older Hive that ignores it would run the applet with more power than its author
meant. So a key that limits what an applet can do must ship with `hive:` set to the release that
understands it, or with a new `version`, and an older Hive then refuses the file rather than
running it without the limit. `hive:` itself fails closed: a value this Hive cannot read refuses
the file. An applet with no such key has full access, as every applet has today. The import
dialog lists every skipped key and says a limit among them would not apply.

**Hive's command ids and routes are part of the applet API.** A shared page names them through
`hive.command`, `hive.commands` and `hive.navigate`, so a rename keeps the old name as an alias. A
snapshot test of the set makes a rename a visible diff. The other option was a curated list that
refuses the rest: it would cost less to keep, and take away most of what those calls are for.

**Verbs follow one rule.** `close` and `resize` act on the frame that calls them, and mean what
fits its surface. `open` and `dropdown` act on another surface.

**A user can turn an applet off,** in Settings or as `applets.disabled`. This is a choice about
the applet and not a limit on it: the applet is off or has its full access. Its bridge stops
answering and its pages stop being served, in Go, so the rule holds whatever the frontend does.

## Consequences

- An applet has the user's full access, and so does any remote script its page loads.
  Opening an applet from someone else is running their code. The warning says so, and no
  check of the file can make it safe: the checks protect the disk from the import, not the
  user from the applet.
- An export cannot tell every data file a page wrote from one its author wrote, so it leaves
  out hidden files and logs and shows the user the list.
- A page cannot `fetch` another HTML file of its own, because that response has no CORS
  header. It can navigate to one.
- Cookies and IndexedDB do not work in an opaque origin.
- `window.hive` rejects every call in a page nested inside another, including an applet's own.
- A page a frame navigates to gets no reply or event, because those cross a `MessagePort` that
  stays with the page that opened it.
- Keys in a frame do not reach the app's keymap, so the injected script forwards Escape
  and any command, control or option chord, and a page keeps one by calling `preventDefault`.
  One click into a page would otherwise leave every shortcut dead, and the chord that opened
  a panel could not close it. A panel stays modal: the keymap already refuses all but the
  palette and that applet's own commands while one is open. A page that uses a chord and
  does not call `preventDefault` gets both.
- Bare keys stay in the page, so typing never runs a shortcut, except the rest of a chord
  sequence the app reports waiting.
- A dropdown loads again each time it opens. A page that must keep running, such as a music
  player, lives in the title bar frame and talks to its dropdown.
- A page falls back to its own directory on a server session: commands run on this
  machine, as a pop-up terminal's do.
- A built-in does not update on disk, because it is not on disk. Its saved `localStorage`
  is, in the directory a copy would use.
