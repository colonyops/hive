# A canvas html block is restricted by what it can reach, not by how it looks

- **Status:** accepted
- **Date:** 2026-09-11

## Context

An agent's html block must be able to draw (a diagram needs `<svg>`), but
the block renders in the app's own webview, which carries the Wails bindings
and no CSP. Script there can call `ActionsService.CreateAction` with a shell
command and run it, so XSS in this pane is command execution. The realistic
route is an indirect prompt injection in something the agent read, turned
into markup that re-renders every time the canvas opens. Nothing in that
threat is about colour, class names, or coordinates.

## Decision

**The allowlists in `cmd/desktop/internal/app/canvas/html.go` bound reach,
and nothing else.** An element or attribute is refused when it executes,
navigates, loads a document, or confuses the second parse: `script`, the
`style` element, `iframe`, `object`, `embed`, `form` and its controls, every
`on*` handler, `srcdoc`, and the svg integration points `desc`, `title` and
`foreignObject`, whose content is dropped along with `math` and `template`.
`id` is refused because an id in this page can clobber a global the app's
code reads.

**Everything inside the allowlists is the agent's**: any class, any `style`
attribute, any value. A value rule that cannot name a concrete exploit is a
preference, not a sanitizer.

**`contain: layout` on `.hv-html` is what makes that safe.** Free classes
let a block reach global utilities and position itself, and a full-window
overlay in a page with bindings is a phishing target. Layout containment
makes the block the containing block for fixed descendants, so nothing
paints outside the pane. `container-type` alone does not do this. The rule
lives in `web/packages/ui/src/styles/canvas-html.css`.

**`img` and the `style` attribute are allowed, and they reach the network.**
Opening a canvas can fetch from a host the agent named. A hostile agent has
its own network already, and filtering `url()` out of CSS text is defeated by
escapes and comments, so a filter would look like a boundary without being
one.

**The `hv-` classes are defaults, not a vocabulary.** The stylesheet styles
them and the agent docs teach them, but a class outside the list reaches the
DOM.

**`RejectedHTML` asks the policy instead of restating it.** It sanitizes a
one-element probe and reports whether the attribute survived, so it cannot
drift from what the render drops.

## Consequences

- Colour correctness is advice. An agent can write a value unreadable in one
  theme; the docs lead with the `hv-` names and say the user may be in either
  theme.
- Removing `contain: layout` from `.hv-html` re-opens the overlay.
- Opening a canvas is a network event. If that becomes unwanted, the fix is a
  CSP on the webview, which covers the rest of the app too, not a narrower
  allowlist here.
