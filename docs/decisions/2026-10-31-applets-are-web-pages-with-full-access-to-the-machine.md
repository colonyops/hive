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

Nothing runs when a file loads. Commands start when the user opens an applet, or puts it
in the title bar, and its page runs them.

A page is not limited to the panel. An applet with `titlebar:` offers a second page, a
frame in a slot of the title bar with its own width, and that page can open a dropdown
frame under the bar. The surfaces share one bridge and differ only in what hosts them, so
the next place an applet can appear is another host of `AppletFrame` and a key in
`applet.yaml`.

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
reason as `applet.<id>`. It goes to one frame, the title bar page if it is running, because
that frame is the one that keeps running and two frames acting on it would run it twice.
With no page to take it, the panel opens, and the bridge holds the command until the page
listens.

**Sharing is where the first rule stops holding.** This ADR's case for no restrictions is
that an applet is written by or for the person who runs it. An import breaks that, so it
gets the one warning in this design, and it still adds no permission, since an applet that
someone trusts has the access any applet has. A `.hiveapplet` is a zip holding
`hive-applet.json`, with a format number and the id, and the files under `files/`. The file
is hostile until the user confirms: it is checked whole in memory and installed by renaming
a finished directory in, so nothing lands early or partly. Import never replaces an applet,
because a replace is how someone else's update overwrites what a user trusted, and it clears
any title bar place or chord the id still has in `settings.yaml`, so a file cannot inherit
what the user gave another applet of that name.

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
