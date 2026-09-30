# A canvas html block is restricted by what it can reach, not by how it looks

- **Status:** accepted
- **Date:** 2026-09-11

## Context

[ADR canvas-html-blocks-are-sanitized-in-go-and-styled-by-an-app-owned-class-vocabulary](2026-08-29-canvas-html-blocks-are-sanitized-in-go-and-styled-by-an-app-owned-class-vocabulary.md)
shipped one allowlist that did two unrelated jobs. It kept markup from
reaching anything dangerous, and it kept an agent from picking its own
colours. Both were enforced the same way, which made the second look like
security.

#429 is what the second job costs. Asked for a diagram, an agent was refused
`<svg>` and `<style>`, and fell back to `hv-card` boxes with `→` characters
between them. The relations between the nodes are the content, and none of
them survived.

Separating the two jobs needs the actual threat named. The pane renders in the
app's own webview, which carries the Wails bindings and no CSP. Script there
can call `ActionsService.CreateAction` with a shell command and then invoke
it, so XSS in this pane is command execution, not defacement. The realistic
route is not a hostile local process — one of those can already write the
actions file directly — but an indirect prompt injection in something the
agent read, turned into markup and then into persistence, since a canvas
re-renders every time it is opened.

Nothing in that threat is about colour, class names, or coordinate units.

## Decision

**The allowlists bound reach, and nothing else.** An element or attribute is
refused when it executes, navigates, loads a document, or confuses the second
parse: `script`, `style` the element, `iframe`, `object`, `embed`, `form` and
its controls, every `on*` handler, `srcdoc`, and the svg integration points
`desc`, `title` and `foreignObject`, whose content is dropped with `math` and
`template`. `id` is refused too, because an id in this page can clobber a
global the app's own code reads.

**Everything inside the allowlists is the agent's.** Any class, any `style`
attribute, any value. The class vocabulary, the coordinate patterns, the
required `viewBox` and the ban on `fill`/`stroke` are all gone. A value rule
that cannot name a concrete exploit is a preference wearing a sanitizer's
coat, and it costs an agent work it came to do.

**`contain: layout` on `.hv-html` is what makes that safe.** Free classes mean
a block can reach the global utilities and position itself, and a
full-window overlay in a page with bindings is a phishing target. Measured in
the pane's real structure: without it, `class="fixed inset-0"` covers the
whole window; with it, a fixed child with a maximal z-index cannot paint
outside the pane, and the sidebar still takes the click. `container-type`
alone does not do this, despite layout containment implying it.

**`img` and the `style` attribute are allowed, and they do reach the
network.** Opening a canvas can fetch from a host the agent named, from the
user's machine. This is a deliberate trade, taken with the owner: a hostile
agent has its own network already, and blocking only `img` while `style`
carries `url()` is theatre. Filtering `url()` out of CSS text is not offered
as an alternative — escapes and comments defeat substring matching, and a
filter that looks like a boundary and is not is worse than none.

**The `hv-` vocabulary stays, as defaults.** The stylesheet still styles
every name, the doc page still teaches them, and the bijection test still
holds all three together. What changed is that a name outside the list now
reaches the DOM instead of failing the write.

**`RejectedHTML` asks the policy about URLs instead of restating its rule.**
It sanitizes a one-element probe and reports whether the attribute survived.
The old copy of the scheme list was a second declaration that could drift into
accepting what the render drops.

## Consequences

- Colour correctness is now advice. An agent can write a hex value that is
  unreadable in one of the two themes, and nothing stops it. The doc page
  leads with the `hv-` names and says the user may be in either theme.
- The sanitizer is smaller than before it grew an svg subset. Deleting the
  class regex, the value patterns and the per-element attribute map removed
  more code than drawing added.
- `contain: layout` is load-bearing, not cosmetic. Removing it from
  `.hv-html` re-opens the overlay, and the CSS says so where it is set.
- Opening a canvas is now a network event. If that becomes unwanted, the
  place to fix it is a CSP on the webview, which would also cover the rest of
  the app, rather than a narrower allowlist here.
