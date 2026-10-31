# The webview renders at the display refresh rate through a WebKit private preference

- **Status:** accepted
- **Date:** 2026-10-30

## Context

WKWebView holds `requestAnimationFrame` and page rendering near 60fps on a
120Hz panel, through the `PreferPageRenderingUpdatesNear60FPSEnabled` WebKit
preference. There is no public API for it, and no user default reaches it. The
preference only works when set before the webview's first load. Setting it
later has no effect, even across a new navigation. Wails creates the webview
and starts that load in one main-thread call, with no hook in between.

## Decision

At startup on macOS, `wailsui` swizzles `-[WKWebView initWithFrame:configuration:]`
and turns the preference off through the private `WKPreferences` feature SPI,
for every webview the process creates. If WebKit no longer has the SPI or the
preference, the app logs that and keeps WebKit's default rate.

## Consequences

- Frame rate follows the display, up to 120fps on a ProMotion panel. Nothing
  here can exceed what WebKit allows on the display in use.
- Code on a `requestAnimationFrame` loop runs up to twice as often, so it costs
  more while it runs.
- A private API rules out a Mac App Store build. Hive ships Developer ID signed
  and notarized, so this does not affect it today.
- A macOS update can change this without notice. Settings ▸ Observability shows
  the frame rate to check it against.
- A Wails option that sets the preference at creation would replace the swizzle.
