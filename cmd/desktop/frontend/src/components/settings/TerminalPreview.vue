<script setup lang="ts">
import { markRaw, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { Terminal, type IDisposable } from '@xterm/xterm'
import { useTerminalFont } from '../../composables/useTerminalFont'
import { useTheme } from '../../composables/useTheme'
import { loadTerminalFaces, terminalFontStack } from '../../lib/terminalFaces'
import { claimAtlasRenderer } from '../../lib/terminalRenderer'
import { xtermTheme } from '../../lib/terminalTheme'
import '@xterm/xterm/css/xterm.css'

// A real Terminal on a real atlas renderer, not a styled block of DOM. The
// settings above it are only legible through the thing they configure: Canvas2D
// does not inherit the `-webkit-font-smoothing: antialiased` the rest of the app
// draws with, so a face renders heavier here than the same face does in the DOM
// (ADR terminal-typography-is-configurable), and box drawing is stroked to cell bounds rather than taken from
// the font (ADR terminal-atlas-renderer). A CSS mock would be wrong in exactly the two places these
// settings are for.

const {
  px: fontSizePx,
  family: fontFamily,
  weight: fontWeight,
  weightBold: fontWeightBold,
  lineHeight,
  letterSpacing,
} = useTerminalFont()
const { theme } = useTheme()

// The line-height check ADR terminal-line-height-and-letter-spacing leaves to the eye: a box whose verticals must
// meet across rows, block glyphs that must tile, and a run of prose long enough
// to judge tracking against. The powerline segment is what the bundled symbol
// face answers, so a family picked here that lacks one shows the fallback.
const SAMPLE = [
  '\x1b[44;30m  main \x1b[0m\x1b[34m\x1b[0m \x1b[32m❯\x1b[0m hive preview',
  'AaBbCc 0123456789 il1I O0 {}[]()<> &@#%',
  '\x1b[2mdim\x1b[0m \x1b[1mbold\x1b[0m \x1b[3mitalic\x1b[0m \x1b[4munderline\x1b[0m',
  '',
  '┌────────────┬────────────┐',
  '│ box drawing│ joins rows │',
  '├────────────┼────────────┤',
  '│ ░▒▓█ shades│ ▁▃▅▇ blocks│',
  '└────────────┴────────────┘',
  '',
  '\x1b[90m12:00:01\x1b[0m \x1b[36mINFO\x1b[0m  build started',
  '\x1b[90m12:00:02\x1b[0m \x1b[33mWARN\x1b[0m  retrying in 250ms',
  '\x1b[90m12:00:03\x1b[0m \x1b[32mOK\x1b[0m    847 passed, 0 failed',
]

// Fixed rather than fitted: the sample is written to be read at one width, and a
// grid that reflowed with the settings pane would change what is being compared
// every time the pane moved. Wider than the column at the largest presets, which
// is what the host's overflow is for.
const COLS = 46
const ROWS = SAMPLE.length

const host = ref<HTMLElement | null>(null)
const term = shallowRef<Terminal | null>(null)
const disposers: IDisposable[] = []
let disposed = false

onMounted(async () => {
  // Before the Terminal is constructed: xterm measures its cell on open() and
  // never re-measures, and the atlas caches whatever was resident (ADR terminal-atlas-renderer).
  await loadTerminalFaces(fontFamily.value, fontSizePx.value, fontWeight.value, fontWeightBold.value)
  if (disposed || !host.value) return

  const created = markRaw(new Terminal({
    fontFamily: terminalFontStack(fontFamily.value),
    fontSize: fontSizePx.value,
    fontWeight: fontWeight.value,
    fontWeightBold: fontWeightBold.value,
    lineHeight: lineHeight.value,
    letterSpacing: letterSpacing.value,
    theme: xtermTheme(),
    disableStdin: true,
    cursorInactiveStyle: 'none',
    // Nothing scrolls: the sample is exactly the grid it is written for.
    scrollback: 0,
  }))
  created.resize(COLS, ROWS)
  created.open(host.value)
  // Nothing retries a preview — it has no reveal path to retry on — and a claim
  // that failed here failed for the real panes too, so what it shows is still
  // what the user is going to get.
  claimAtlasRenderer(created, (addon) => disposers.push(addon), () => {})
  created.write(SAMPLE.join('\r\n'))

  // xterm's input textarea is focusable even with stdin disabled, and a preview
  // is not a stop on the way through the settings pane.
  host.value.querySelector('textarea')?.setAttribute('tabindex', '-1')
  term.value = created
})

watch(theme, () => { if (term.value) term.value.options.theme = xtermTheme() })

watch(
  [fontSizePx, fontFamily, fontWeight, fontWeightBold, lineHeight, letterSpacing],
  async ([px, family, weight, weightBold, height, spacing]) => {
    await loadTerminalFaces(family, px, weight, weightBold)
    if (!term.value) return
    term.value.options.fontFamily = terminalFontStack(family)
    term.value.options.fontSize = px
    term.value.options.fontWeight = weight
    term.value.options.fontWeightBold = weightBold
    term.value.options.lineHeight = height
    term.value.options.letterSpacing = spacing
  },
)

onBeforeUnmount(() => {
  disposed = true
  // Addons ahead of the Terminal: xterm disposes its core before its addons,
  // and a renderer addon restores a renderer on the way out.
  for (const disposer of disposers.splice(0)) disposer.dispose()
  term.value?.dispose()
  term.value = null
})
</script>

<template>
  <div data-testid="settings-terminal-preview">
    <p class="mb-1.5 text-[12.5px] text-text-2">Preview</p>
    <div class="hive-scroll overflow-x-auto rounded-lg border border-border bg-app p-3">
      <div ref="host" class="w-fit" data-testid="settings-terminal-preview-pane" />
    </div>
  </div>
</template>
