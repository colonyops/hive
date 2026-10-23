import { shallowRef } from 'vue'
import { createHighlighterCore, type HighlighterCore } from 'shiki/core'
import { createOnigurumaEngine } from 'shiki/engine/oniguruma'
import { useHiveUiPlatform, type HiveUiPlatform } from '../platform'
import { codeThemes, type CodeTheme } from './codeThemes'
import type { Highlight } from './githubMarkdown'

// Aliases the grammars do not declare themselves. Shiki already covers sh,
// shell, zsh, ts, js, yml, md, py, rs and docker.
const extraAliases: Record<string, string> = {
  golang: 'go',
  jsonc: 'json',
  json5: 'json',
  patch: 'diff',
  htm: 'html',
  svg: 'xml',
  conf: 'ini',
  console: 'bash',
}

// Dynamic imports keep about a megabyte of grammar source out of the startup
// chunk: nothing pays for them until the first fence renders.
function loadLanguages() {
  return Promise.all([
    import('shiki/langs/bash.mjs'),
    import('shiki/langs/go.mjs'),
    import('shiki/langs/typescript.mjs'),
    import('shiki/langs/tsx.mjs'),
    import('shiki/langs/javascript.mjs'),
    import('shiki/langs/jsx.mjs'),
    import('shiki/langs/json.mjs'),
    import('shiki/langs/yaml.mjs'),
    import('shiki/langs/toml.mjs'),
    import('shiki/langs/markdown.mjs'),
    import('shiki/langs/html.mjs'),
    import('shiki/langs/css.mjs'),
    import('shiki/langs/vue.mjs'),
    import('shiki/langs/sql.mjs'),
    import('shiki/langs/python.mjs'),
    import('shiki/langs/rust.mjs'),
    import('shiki/langs/diff.mjs'),
    import('shiki/langs/dockerfile.mjs'),
    import('shiki/langs/ini.mjs'),
    import('shiki/langs/xml.mjs'),
  ]).then((modules) => modules.map((m) => m.default))
}

export const cacheLimit = 200
const sliceMs = 12
const bumpEveryMs = 150

// One slice per frame, inside the frame's callback: a timer would do, but
// WebKit throttles timers in a page the user is not touching to one tick a
// second, which turned half a second of highlighting into four.
const nextFrame: (run: () => void) => void =
  typeof requestAnimationFrame === 'function' ? (run) => requestAnimationFrame(run) : (run) => setTimeout(run, 0)

/**
 * Syntax highlighting for fenced code, shared by every markdown renderer of
 * one app.
 *
 * Oniguruma, not shiki's JavaScript regex engine: the JavaScript engine
 * emulates the atomic groups these grammars rely on with lookahead and
 * backtracks catastrophically on some input (a one-line Go struct with a
 * block comment took over ten seconds per render and froze the window). The
 * WASM and the grammars are dynamic imports, so nothing pays for them until
 * a fence renders.
 *
 * A fence is highlighted in the code theme of the app theme in view
 * (lib/codeThemes.ts). The engine starts with that one theme and loads
 * another the first time the user switches to it; the cache is keyed by
 * theme, so switching back costs nothing.
 *
 * Highlighting never runs inside a render. A fence the cache does not hold
 * renders escaped and joins the queue; the queue drains in slices between
 * frames, and `version` re-runs every computed that asked, which then finds
 * its fences in the cache. A document with dozens of fences still costs
 * hundreds of milliseconds to highlight, and paid inside one render that was
 * a frozen window.
 *
 * The cache keeps the most recent `cacheLimit` fences but never evicts one a
 * render asked for since the last `version` bump, so it grows past the limit
 * while more fences than that are on screen. Evicting those would drop the
 * hits the next render needs, and the queue would never empty.
 */
export function createCodeHighlight(platform: HiveUiPlatform) {
  const log = platform.log('code-highlight')
  const appTheme = platform.theme
  let highlighter: HighlighterCore | null = null
  let loading: Promise<void> | null = null
  const loaded = new Set<string>()
  const loadedThemes = new Set<string>()
  const themeLoads = new Map<string, Promise<void>>()
  const ready = shallowRef(false)

  const cache = new Map<string, string | null>()
  const queue = new Map<string, { code: string; lang: string; theme: string }>()
  const version = shallowRef(0)
  let touched = new Set<string>()
  let scheduled = false
  let lastBump = 0

  function codeTheme(): CodeTheme {
    return codeThemes[appTheme.value]
  }

  /** Loads the engine once. Resolves either way: a failed load leaves every fence unhighlighted. */
  function load(): Promise<void> {
    loading ??= (async () => {
      const first = codeTheme()
      const [registration, langs] = await Promise.all([first.load(), loadLanguages()])
      const core = await createHighlighterCore({
        themes: [registration],
        langs,
        engine: createOnigurumaEngine(import('shiki/wasm')),
      })
      highlighter = core
      for (const lang of core.getLoadedLanguages()) loaded.add(lang)
      loadedThemes.add(first.name)
      ready.value = true
    })().catch((err: unknown) => {
      log.warn('syntax highlighting is unavailable', { err })
    })
    return loading
  }

  /** Loads a theme the engine does not hold yet, then re-runs the renders that asked. */
  function ensureTheme(theme: CodeTheme): Promise<void> {
    if (loadedThemes.has(theme.name)) return Promise.resolve()
    let pending = themeLoads.get(theme.name)
    if (!pending) {
      pending = theme
        .load()
        .then((registration) => highlighter!.loadTheme(registration))
        .then(
          () => {
            loadedThemes.add(theme.name)
            version.value++
          },
          (err: unknown) => {
            log.warn('code theme is unavailable', { theme: theme.name, err })
            themeLoads.delete(theme.name)
          },
        )
      themeLoads.set(theme.name, pending)
    }
    return pending
  }

  function cacheKey(theme: string, code: string, lang: string): string {
    return `${theme}\0${lang}\0${code}`
  }

  function remember(key: string, html: string | null): void {
    for (const old of cache.keys()) {
      if (cache.size < cacheLimit) break
      if (!touched.has(old)) cache.delete(old)
    }
    cache.set(key, html)
  }

  function compute(code: string, lang: string, theme: string): string | null {
    try {
      // markdown-it fence content ends in a newline that shiki would render
      // as a blank last line. One theme under the `app` key: every token
      // carries a --shiki-app variable and the block keeps the app's own
      // background.
      return highlighter!.codeToHtml(code.replace(/\n$/, ''), {
        lang,
        themes: { app: theme },
        defaultColor: false,
      })
    } catch {
      // A pathological input must not take the whole rendered body down.
      return null
    }
  }

  function drain(budgetMs: number): void {
    const started = performance.now()
    for (const [key, job] of queue) {
      queue.delete(key)
      remember(key, compute(job.code, job.lang, job.theme))
      if (performance.now() - started >= budgetMs) break
    }
    const now = performance.now()
    if (queue.size === 0 || now - lastBump >= bumpEveryMs) {
      lastBump = now
      touched = new Set()
      version.value++
    }
  }

  function schedule(): void {
    if (scheduled) return
    scheduled = true
    nextFrame(() => {
      scheduled = false
      drain(sliceMs)
      if (queue.size) schedule()
    })
  }

  /** Highlights everything queued so far, for a test that reads the result back at once. */
  async function settle(): Promise<void> {
    await load()
    if (highlighter) await ensureTheme(codeTheme())
    while (queue.size) drain(Number.POSITIVE_INFINITY)
  }

  /**
   * Shiki's HTML (a `pre.shiki` carrying a `--shiki-app` variable per token)
   * for a known language the cache holds under the app theme in view, or
   * null so the renderer keeps its own escaped output. Shiki escapes the code
   * itself. A miss queues the fence; a computed that called this re-runs
   * once the engine or the theme is ready and again as queued fences land.
   */
  const highlight: Highlight = (code, lang) => {
    // Read before any early return: a computed that asked while a theme was
    // loading must depend on the bump that says it has.
    void version.value
    const theme = codeTheme()
    if (!ready.value || !highlighter) {
      void load()
      return null
    }
    if (!loadedThemes.has(theme.name)) {
      void ensureTheme(theme)
      return null
    }
    const alias = lang.trim().toLowerCase()
    const resolved = extraAliases[alias] ?? alias
    if (!loaded.has(resolved)) return null
    const key = cacheKey(theme.name, code, resolved)
    touched.add(key)
    const hit = cache.get(key)
    if (hit !== undefined) {
      cache.delete(key)
      cache.set(key, hit)
      return hit
    }
    if (!queue.has(key)) queue.set(key, { code, lang: resolved, theme: theme.name })
    schedule()
    return null
  }

  return { highlight, load, settle }
}

export type CodeHighlight = ReturnType<typeof createCodeHighlight>

// One highlighter per app: the engine and the cache are expensive, and every
// renderer in the app asks the same one. Keyed by platform so a second app
// (or a test) gets its own.
const instances = new WeakMap<HiveUiPlatform, CodeHighlight>()

export function useCodeHighlight(): CodeHighlight {
  const platform = useHiveUiPlatform()
  let instance = instances.get(platform)
  if (!instance) {
    instance = createCodeHighlight(platform)
    instances.set(platform, instance)
  }
  return instance
}
