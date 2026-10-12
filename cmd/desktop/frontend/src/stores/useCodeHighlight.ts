import { shallowRef } from 'vue'
import { createHighlighterCore, type HighlighterCore } from 'shiki/core'
import { createOnigurumaEngine } from 'shiki/engine/oniguruma'
import bash from 'shiki/langs/bash.mjs'
import css from 'shiki/langs/css.mjs'
import diff from 'shiki/langs/diff.mjs'
import dockerfile from 'shiki/langs/dockerfile.mjs'
import go from 'shiki/langs/go.mjs'
import html from 'shiki/langs/html.mjs'
import ini from 'shiki/langs/ini.mjs'
import javascript from 'shiki/langs/javascript.mjs'
import json from 'shiki/langs/json.mjs'
import jsx from 'shiki/langs/jsx.mjs'
import markdown from 'shiki/langs/markdown.mjs'
import python from 'shiki/langs/python.mjs'
import rust from 'shiki/langs/rust.mjs'
import sql from 'shiki/langs/sql.mjs'
import toml from 'shiki/langs/toml.mjs'
import tsx from 'shiki/langs/tsx.mjs'
import typescript from 'shiki/langs/typescript.mjs'
import vue from 'shiki/langs/vue.mjs'
import xml from 'shiki/langs/xml.mjs'
import yaml from 'shiki/langs/yaml.mjs'
import githubDark from 'shiki/themes/github-dark.mjs'
import githubLight from 'shiki/themes/github-light.mjs'
import type { Highlight } from '../lib/githubMarkdown'
import { createLogger } from '../lib/log'
import { defineStore } from './defineStore'

const log = createLogger('code-highlight')

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

const languages = [
  bash,
  go,
  typescript,
  tsx,
  javascript,
  jsx,
  json,
  yaml,
  toml,
  markdown,
  html,
  css,
  vue,
  sql,
  python,
  rust,
  diff,
  dockerfile,
  ini,
  xml,
]

const cacheLimit = 200
const sliceMs = 12
const bumpEveryMs = 150

// One slice per frame, inside the frame's callback: a timer would do, but
// WebKit throttles timers in a page the user is not touching to one tick a
// second, which turned half a second of highlighting into four.
const nextFrame: (run: () => void) => void =
  typeof requestAnimationFrame === 'function' ? (run) => requestAnimationFrame(run) : (run) => setTimeout(run, 0)

/**
 * Syntax highlighting for fenced code, shared by every markdown renderer.
 *
 * Oniguruma, not shiki's JavaScript regex engine: the JavaScript engine
 * emulates the atomic groups these grammars rely on with lookahead and
 * backtracks catastrophically on some input (a one-line Go struct with a
 * block comment took over ten seconds per render and froze the window). The
 * WASM is a dynamic import, so nothing pays for it until a fence renders.
 *
 * Highlighting never runs inside a render. A fence the cache does not hold
 * renders escaped and joins the queue; the queue drains in slices between
 * frames, and `version` re-runs every computed that asked, which then finds
 * its fences in the cache. A document with dozens of fences still costs
 * hundreds of milliseconds to highlight, and paid inside one render that was
 * a frozen window.
 */
export const useCodeHighlight = defineStore('codeHighlight', () => {
  let highlighter: HighlighterCore | null = null
  let loading: Promise<void> | null = null
  const loaded = new Set<string>()
  const ready = shallowRef(false)

  const cache = new Map<string, string | null>()
  const queue = new Map<string, { code: string; lang: string }>()
  const version = shallowRef(0)
  let scheduled = false
  let lastBump = 0

  /** Loads the engine once. Resolves either way: a failed load leaves every fence unhighlighted. */
  function load(): Promise<void> {
    loading ??= createHighlighterCore({
      themes: [githubLight, githubDark],
      langs: languages,
      engine: createOnigurumaEngine(import('shiki/wasm')),
    }).then(
      (core) => {
        highlighter = core
        for (const lang of core.getLoadedLanguages()) loaded.add(lang)
        ready.value = true
      },
      (err: unknown) => {
        log.warn('syntax highlighting is unavailable', err)
      },
    )
    return loading
  }

  function cacheKey(code: string, lang: string): string {
    return `${lang}\0${code}`
  }

  function remember(key: string, html: string | null): void {
    if (cache.size >= cacheLimit) cache.delete(cache.keys().next().value as string)
    cache.set(key, html)
  }

  function compute(code: string, lang: string): string | null {
    try {
      // markdown-it fence content ends in a newline that shiki would render
      // as a blank last line.
      return highlighter!.codeToHtml(code.replace(/\n$/, ''), {
        lang,
        themes: { light: 'github-light', dark: 'github-dark' },
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
      remember(key, compute(job.code, job.lang))
      if (performance.now() - started >= budgetMs) break
    }
    const now = performance.now()
    if (queue.size === 0 || now - lastBump >= bumpEveryMs) {
      lastBump = now
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
    while (queue.size) drain(Number.POSITIVE_INFINITY)
  }

  /**
   * Shiki's HTML (a `pre.shiki` carrying `--shiki-light` and `--shiki-dark`
   * variables per token) for a known language the cache holds, or null so
   * the renderer keeps its own escaped output. Shiki escapes the code
   * itself. A miss queues the fence; a computed that called this re-runs
   * once the engine is ready and again as queued fences land.
   */
  const highlight: Highlight = (code, lang) => {
    if (!ready.value || !highlighter) {
      void load()
      return null
    }
    void version.value
    const alias = lang.trim().toLowerCase()
    const resolved = extraAliases[alias] ?? alias
    if (!loaded.has(resolved)) return null
    const key = cacheKey(code, resolved)
    const hit = cache.get(key)
    if (hit !== undefined) {
      cache.delete(key)
      cache.set(key, hit)
      return hit
    }
    if (!queue.has(key)) queue.set(key, { code, lang: resolved })
    schedule()
    return null
  }

  return { highlight, load, settle }
})
