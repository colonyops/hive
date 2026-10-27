# Hive Desktop is addressed by hive:// URLs, not by its internal routes

- **Status:** accepted
- **Date:** 2026-10-10

## Context

Other programs need to send the user to a place in Hive Desktop: a tool that
starts tmux sessions and shows a "go to" button, or an agent that wants the
user to look at its session or its canvas. The frontend's hash routes are
internal and change with the UI.

## Decision

**A place in the app has one public address, a `hive://` URL.** The app
registers the scheme with the operating system. The schema lives in
`internal/domain/deeplink`, so the CLI and the desktop accept the same URLs:

```
hive://code/<slug>[?window=<id>][&canvas=<name>]
hive://chats/<workspace>[/<chat>][?canvas=<name>]
hive://inbox/<profile>[/<item>][?feed=<id>]
hive://canvas/<name>?owner=<workspace | owner/repo | @global>
hive://drive/<space-key>[/<path>]
```

The areas use the names users see, not the route names. `code` takes a tmux
session name, so a link also reaches a session hive did not create. Unknown
query parameters are ignored, so a newer link still opens in an older app.

**A link only navigates.** It raises the window and routes. It never creates
a session, runs a command, or changes data, because any web page can open a
`hive://` URL.

**The adapter holds the link until the frontend takes it.** A link that
launches the app arrives before the webview listens. `LinkService` keeps the
latest link, and `link:open` is only a wake-up signal.

**On Linux, a second launch hands its link to the running app.** The OS runs
the binary again for each link, so the app uses the Wails single-instance
lock, keyed by the data directory. Wails fails startup when it cannot take the
lock, so the app skips the lock when no session bus is present.

**Hive ships the schema, not link builders.** Links come from static
templates or from an agent that reads the schema in the docs or in
`hive desktop --help`. `hive desktop open` checks a link with `Parse` and
passes it to `open` or `xdg-open`. It does not talk to the app.

## Consequences

- The URL schema is a contract. A change to a route does not change a link,
  and a change to a link needs a compatible parse.
- Only the packaged macOS app and installed Linux builds receive links.
  `wails dev` and the `.dev.app` bundle do not register the scheme.
- `hive desktop open` cannot report that a slug or an item does not exist.
