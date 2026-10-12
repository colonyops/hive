import { createHighlighterCoreSync } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'
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

// The JavaScript regex engine keeps this synchronous (no WASM load), which is
// what markdown-it's highlight hook requires.
const highlighter = createHighlighterCoreSync({
  themes: [githubLight, githubDark],
  langs: [
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
  ],
  engine: createJavaScriptRegexEngine(),
})

const loaded = new Set(highlighter.getLoadedLanguages())

/**
 * Returns shiki's HTML (a `pre.shiki` carrying `--shiki-light` and
 * `--shiki-dark` variables per token) for a known language, or null so the
 * caller keeps its own escaped output. Shiki escapes the code itself.
 */
export function highlightCode(code: string, lang: string): string | null {
  const key = lang.trim().toLowerCase()
  const resolved = extraAliases[key] ?? key
  if (!loaded.has(resolved)) return null
  try {
    // markdown-it fence content ends in a newline that shiki would render as a
    // blank last line.
    return highlighter.codeToHtml(code.replace(/\n$/, ''), {
      lang: resolved,
      themes: { light: 'github-light', dark: 'github-dark' },
      defaultColor: false,
    })
  } catch {
    // A pathological input must not take the whole rendered body down.
    return null
  }
}
