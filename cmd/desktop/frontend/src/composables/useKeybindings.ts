import { computed, ref } from 'vue'
import {
  KeybindingSettings as GetKeybindingSettings,
  SetKeybindingSettings,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { commands, defaultCombosFor } from '../keybindings/catalog'

// The frontend keybinding layer. Pure normalization (comboFromEvent /
// formatCombo) is separate from the effective keymap so both are unit-testable
// without mounting a component. Only *overrides* are persisted (to
// settings.yaml's `keybindings:` section via SettingsService): an id absent
// from the store falls back to its catalog default, an id mapped to `[]` is
// explicitly unbound. Bindings target the stable command ids in
// keybindings/catalog.ts.
//
// A binding is one combo or a space-separated sequence of combos (`g i`).
// `canonicalizeCombo` stays the per-step helper; `canonicalizeBinding` is the
// entry point everywhere a whole binding string is parsed.

type Overrides = Record<string, string[]>

const knownIDs = computed(() => new Set(commands.value.map((c) => c.id)))
const defaultCombos = computed<Record<string, string[]>>(() => {
  const mac = detectMac()
  return Object.fromEntries(commands.value.map((c) => [c.id, defaultCombosFor(c, mac)]))
})

const MODIFIER_ORDER = ['mod', 'ctrl', 'alt', 'shift'] as const

// event.key values that are modifiers or otherwise not real bindable keys.
const IGNORED_KEYS = new Set([
  'Shift',
  'Control',
  'Alt',
  'Meta',
  'CapsLock',
  'NumLock',
  'ScrollLock',
  'OS',
  'AltGraph',
  'Fn',
  'FnLock',
  'Hyper',
  'Super',
  'Symbol',
  'SymbolLock',
  'Dead',
  'Unidentified',
])

const KEY_SYMBOLS: Record<string, string> = {
  arrowdown: '↓',
  arrowup: '↑',
  arrowleft: '←',
  arrowright: '→',
  enter: '↵',
  space: 'Space',
  escape: 'Esc',
  backspace: '⌫',
  tab: 'Tab',
  delete: 'Del',
  plus: '+',
}

// The punctuation event.code names, mapped to the character the same physical
// key produces unmodified — the spelling the rest of this module uses.
const PUNCTUATION_CODES: Record<string, string> = {
  Backquote: '`',
  Minus: '-',
  Equal: '=',
  BracketLeft: '[',
  BracketRight: ']',
  Backslash: '\\',
  Semicolon: ';',
  Quote: "'",
  Comma: ',',
  Period: '.',
  Slash: '/',
}

const NAMED_CODES = /^(Arrow(Up|Down|Left|Right)|Enter|Escape|Tab|Backspace|Delete|Space|Home|End|PageUp|PageDown)$/

// ─── Pure helpers (exported for tests) ─────────────────────────────────────────

/** Meta/Ctrl → `mod`, Alt/Option → `alt`; anything else is not a modifier. */
function normalizeModifier(token: string): string | null {
  switch (token) {
    case 'mod':
    case 'meta':
    case 'cmd':
    case 'command':
    case 'ctrl':
    case 'control':
      return 'mod'
    case 'alt':
    case 'option':
    case 'opt':
      return 'alt'
    case 'shift':
      return 'shift'
    default:
      return null
  }
}

function normalizeKey(key: string): string {
  if (key === ' ' || key === 'Spacebar') return 'space'
  if (key === '+') return 'plus'
  return key.toLowerCase()
}

// Add an explicit `shift` token only when Shift didn't already change the
// produced character: single latin letters (`g` → `shift+g`) and named keys
// (`arrowdown` → `shift+arrowdown`). For symbols/digits the character itself
// already encodes Shift (Shift+/ yields `?`), so `shift` is not doubled on.
function shouldRecordShift(base: string): boolean {
  return /^[a-z]$/.test(base) || base.length > 1
}

/**
 * Canonicalize a combo string to its single spelling: modifiers collapsed
 * (Meta/Ctrl → `mod`), lowercased, ordered `mod, ctrl, alt, shift`, then the
 * base key. Returns '' for a combo with no base key, or whose key contains
 * whitespace — that spelling belongs to a sequence, not a single step.
 */
export function canonicalizeCombo(combo: string): string {
  const parts = combo
    .split('+')
    .map((p) => p.trim().toLowerCase())
    .filter(Boolean)
  const mods = new Set<string>()
  let key = ''
  for (const part of parts) {
    const mod = normalizeModifier(part)
    if (mod) mods.add(mod)
    else key = normalizeKey(part)
  }
  if (!key || /\s/.test(key)) return ''
  const ordered = MODIFIER_ORDER.filter((m) => mods.has(m))
  return [...ordered, key].join('+')
}

/**
 * Canonical spelling for a binding: one combo, or space-separated combos
 * ("g i"). '' when any step is invalid.
 */
export function canonicalizeBinding(binding: string): string {
  const steps = binding.trim().split(/\s+/).filter(Boolean)
  if (steps.length === 0) return ''
  const canonSteps: string[] = []
  for (const step of steps) {
    const canon = canonicalizeCombo(step)
    if (!canon) return ''
    canonSteps.push(canon)
  }
  return canonSteps.join(' ')
}

/** One more step a pending sequence could take, and the command it leads to. */
export interface SequenceContinuation {
  step: string
  commandId: string
}

/** Steps accepted so far, and what can extend them next — the hint pill's data. */
export interface PendingSequence {
  steps: string[]
  continuations: SequenceContinuation[]
}

export type SequenceTransition =
  /** The steps + combo complete a binding: dispatch commandId. */
  | { kind: 'run'; commandId: string }
  /**
   * The combo extends (or starts) a pending sequence. deferredCommandId is
   * non-null when the accumulated steps are ALSO a full binding (Zed's prefix
   * rule): the caller arms SEQUENCE_TIMEOUT_MS and dispatches it if no
   * continuation arrives. The timer decision is made here; the timer itself
   * belongs to the caller.
   */
  | { kind: 'extend'; pending: PendingSequence; deferredCommandId: string | null }
  /** Pending existed and the combo matched nothing bare: clear + consume. */
  | { kind: 'swallow' }
  /**
   * No sequence involvement: dispatch normally. Also returned when pending
   * existed but the combo carries the primary modifier (see stepSequence) —
   * the caller clears pending and proceeds through normal dispatch.
   */
  | { kind: 'pass' }

/** Zed's prefix rule: how long a bound-key-that-is-also-a-prefix waits. */
export const SEQUENCE_TIMEOUT_MS = 1000

/** The base key an event.key names, or null when it is not a bindable key. */
function keyBase(e: KeyboardEvent): string | null {
  const rawKey = e.key
  if (!rawKey || IGNORED_KEYS.has(rawKey)) return null
  return normalizeKey(rawKey) || null
}

/**
 * The base key an event.code names, or null for a code this module has no
 * spelling for. Only used for alt combos — see comboFromEvent.
 */
function baseFromCode(code: string): string | null {
  if (/^Key[A-Z]$/.test(code)) return code.slice(3).toLowerCase()
  if (/^Digit[0-9]$/.test(code)) return code.slice(5)
  if (NAMED_CODES.test(code)) return code.toLowerCase()
  return PUNCTUATION_CODES[code] ?? null
}

/**
 * Canonical combo for a keydown, or null when the event is not a bindable
 * keystroke (lone modifier, IME composition, unidentified key).
 */
export function comboFromEvent(e: KeyboardEvent): string | null {
  if (e.isComposing || e.keyCode === 229) return null
  // An alt combo takes its base from the physical key, not the character. On
  // macOS Option composes: Option+G emits `©`, so reading event.key would make
  // the combo `alt+©` and no binding a user would think to write could match
  // it — and Option+E emits `Dead`, which keyBase drops outright. The cost is
  // that alt binds by position, so a non-QWERTY layout gets the key where `g`
  // sits on QWERTY; the alternative is alt bindings that cannot be spelled at
  // all. Every other modifier still reads the character, which is what makes
  // `?` (Shift+/) a combo rather than `shift+/`.
  //
  // A modifier held alone still falls through: its code (`AltLeft`) is not one
  // baseFromCode spells, so keyBase answers and IGNORED_KEYS rejects it.
  const base = (e.altKey && e.code ? baseFromCode(e.code) : null) ?? keyBase(e)
  if (!base) return null

  const mods = new Set<string>()
  if (e.metaKey || e.ctrlKey) mods.add('mod')
  if (e.altKey) mods.add('alt')
  if (e.shiftKey && shouldRecordShift(base)) mods.add('shift')

  const ordered = MODIFIER_ORDER.filter((m) => mods.has(m))
  return [...ordered, base].join('+')
}

/**
 * The combo to resolve when a terminal pane has focus, or null when the pane
 * should keep the keystroke.
 *
 * A pane owns every key it can use, so only modifiers a terminal never wants
 * qualify: Cmd on macOS, Ctrl+Shift elsewhere. comboFromEvent cannot make that
 * call on its own — it collapses Meta and Ctrl into one `mod` token, so `mod+k`
 * cannot tell ⌘K from Ctrl+K, and Ctrl+K is readline's kill-to-end-of-line.
 * Same split useTerminalWindows' isSearchCombo makes for ⌘F, for the same
 * reason.
 *
 * The Shift is dropped from the Ctrl form before resolving: on a platform
 * without Cmd, Ctrl+Shift is how a terminal emulator spells an app chord
 * (Ctrl+Shift+C is ⌘C), so it stands in for Cmd rather than being part of the
 * combo — which is what lets one configured `mod+k` match on both platforms.
 * It also means a `mod+shift+<key>` binding can never be reached from a pane
 * there, which is why the catalog carries `ctrlDefaultCombos` for the escaping
 * commands whose macOS chord is shifted.
 */
export function terminalEscapeCombo(e: KeyboardEvent): string | null {
  const escapes = e.ctrlKey ? e.shiftKey && !e.metaKey : e.metaKey && !e.altKey
  if (!escapes) return null
  const combo = comboFromEvent(e)
  if (!combo) return null
  if (!e.ctrlKey) return combo
  return canonicalizeCombo(
    combo
      .split('+')
      .filter((token) => token !== 'shift')
      .join('+'),
  )
}

function detectMac(): boolean {
  return typeof navigator !== 'undefined' && /Mac/i.test(navigator.userAgent)
}

function formatModifier(mod: string, isMac: boolean): string {
  switch (mod) {
    case 'mod':
      return isMac ? '⌘' : 'Ctrl'
    case 'alt':
      return isMac ? '⌥' : 'Alt'
    case 'shift':
      return isMac ? '⇧' : 'Shift'
    case 'ctrl':
      return 'Ctrl'
    default:
      return mod
  }
}

/**
 * Human-readable label for a binding — `⌘K` on macOS, `Ctrl+K` elsewhere. A
 * sequence renders each step in order, space-joined: `G I`, `⌘K ⌘S`.
 */
export function formatCombo(binding: string, isMac: boolean = detectMac()): string {
  const canon = canonicalizeBinding(binding)
  if (!canon) return ''
  return canon
    .split(' ')
    .map((step) => formatStep(step, isMac))
    .join(' ')
}

function formatStep(combo: string, isMac: boolean): string {
  const parts = combo.split('+')
  const key = parts[parts.length - 1]
  const mods = parts.slice(0, -1).map((m) => formatModifier(m, isMac))
  const keyLabel = KEY_SYMBOLS[key] ?? (key.length === 1 ? key.toUpperCase() : capitalize(key))
  return isMac ? [...mods, keyLabel].join('') : [...mods, keyLabel].join('+')
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1)
}

// ─── Persisted overrides (module singleton) ────────────────────────────────────

// An unknown id is kept, not dropped. Launcher commands come from actions.yml
// and are not known until the catalog has loaded, which is after this runs —
// dropping them here would erase a user's launcher bindings from settings.yaml
// on their next rebind. An id that stays unknown simply never resolves.
function sanitizeOverrides(value: unknown): Overrides {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  const out: Overrides = {}
  for (const [id, combos] of Object.entries(value as Record<string, unknown>)) {
    if (!Array.isArray(combos)) continue
    const clean: string[] = []
    for (const combo of combos) {
      if (typeof combo !== 'string') continue
      const canon = canonicalizeBinding(combo)
      if (canon && !clean.includes(canon)) clean.push(canon)
    }
    out[id] = clean // may be [] to mean "explicitly unbound"
  }
  return out
}

// The durable record is settings.yaml's `keybindings` section, so shortcuts
// live alongside the rest of the user's config and can be managed from a
// dotfiles repo or handed to an agent (see the "Keyboard shortcuts" prompt in
// cmd/desktop/internal/app/prompts). Webview localStorage is not a candidate: it is
// partitioned per bundle id and per dev-server port and macOS may purge it
// outright, so a rebind could silently vanish.
//
// Only overrides are stored. An id absent here falls back to its catalog
// default; an id mapped to [] is explicitly unbound.
const overrides = ref<Overrides>({})

// Incremented by every mutation so an in-flight hydrate can tell its result is
// already stale and must not clobber a rebind the user just made (the same
// versioning idea as useTheme).
let overridesVersion = 0

// Saves are chained rather than fired in parallel so two quick rebinds cannot
// land out of order and leave settings.yaml disagreeing with the screen. The
// chain absorbs its own failures so it never settles rejected.
let persistChain: Promise<void> = Promise.resolve()

function persistOverrides(next: Overrides): void {
  persistChain = persistChain
    .then(async () => {
      await SetKeybindingSettings({ overrides: next })
    })
    .catch((error: unknown) => {
      // The rebind is already live in this session; losing the durable write is
      // degraded-but-working, not a reason to revert what the user sees.
      console.warn('Unable to persist keybindings to settings.yaml', error)
    })
}

/** Replaces the override map and writes it through to settings.yaml. */
function applyOverrides(next: Overrides): void {
  overridesVersion++
  overrides.value = next
  persistOverrides(next)
}

// Reads the durable overrides once at startup. A failure keeps the catalog
// defaults rather than blanking the keymap: an unavailable binding must not
// leave the app with no shortcuts.
async function hydrateFromSettings(): Promise<void> {
  const version = overridesVersion
  try {
    const settings = await GetKeybindingSettings()
    if (overridesVersion !== version) return
    overrides.value = sanitizeOverrides(settings.overrides)
  } catch (error) {
    console.warn('Unable to load keybindings from settings.yaml', error)
  }
}

/** Called once from main.ts, before the app can dispatch a shortcut. */
export function initializeKeybindings(): void {
  void hydrateFromSettings()
}

// True while the settings editor is capturing a keystroke; the global
// dispatcher checks this so a combo being recorded never also fires a command.
const recording = ref(false)

// The sequence steps accepted so far, module state (like recording) so the
// global dispatcher writes it and the hint pill renders it. stepSequence is
// pure over its arguments and never touches this itself — the caller does.
const pendingSequence = ref<PendingSequence | null>(null)

function clearPendingSequence(): void {
  pendingSequence.value = null
}

const effectiveBindings = computed<Record<string, string[]>>(() => {
  const result: Record<string, string[]> = {}
  for (const command of commands.value) {
    const override = overrides.value[command.id]
    result[command.id] = override !== undefined ? override : defaultCombos.value[command.id]
  }
  return result
})

// binding → command id. Catalog order makes resolution deterministic when two
// commands share a binding (the conflict is surfaced in the settings UI). A
// multi-step binding is keyed by its full space-joined string, so this alone
// cannot match a sequence's first step — that is what keeps `resolve` from
// firing early on `g` while `g i` is still pending.
const exactBindings = computed<Map<string, string>>(() => {
  const map = new Map<string, string>()
  for (const command of commands.value) {
    for (const binding of effectiveBindings.value[command.id]) {
      if (!map.has(binding)) map.set(binding, command.id)
    }
  }
  return map
})

// canonical prefix (steps taken so far, space-joined) → next step → the first
// command a binding through that step claims. Lets stepSequence answer "what
// can extend this pending sequence" without rescanning every command.
const sequencePrefixes = computed<Map<string, Map<string, string>>>(() => {
  const index = new Map<string, Map<string, string>>()
  for (const command of commands.value) {
    for (const binding of effectiveBindings.value[command.id]) {
      const steps = binding.split(' ')
      for (let i = 0; i < steps.length - 1; i++) {
        const prefix = steps.slice(0, i + 1).join(' ')
        const nextStep = steps[i + 1]!
        let continuations = index.get(prefix)
        if (!continuations) {
          continuations = new Map()
          index.set(prefix, continuations)
        }
        if (!continuations.has(nextStep)) continuations.set(nextStep, command.id)
      }
    }
  }
  return index
})

function combosFor(id: string): string[] {
  return effectiveBindings.value[id] ?? []
}

/** Resolves a single-step combo only — a sequence's first step never matches. */
function resolve(combo: string): string | null {
  return exactBindings.value.get(combo) ?? null
}

function setCombos(id: string, combos: string[]): void {
  applyOverrides({ ...overrides.value, [id]: combos })
}

function addBinding(id: string, binding: string): void {
  const canon = canonicalizeBinding(binding)
  if (!canon || !knownIDs.value.has(id)) return
  const current = combosFor(id)
  if (current.includes(canon)) return
  setCombos(id, [...current, canon])
}

function removeBinding(id: string, binding: string): void {
  const canon = canonicalizeBinding(binding)
  setCombos(
    id,
    combosFor(id).filter((c) => c !== canon),
  ) // [] = explicitly unbound
}

function resetToDefault(id: string): void {
  const next = { ...overrides.value }
  delete next[id]
  applyOverrides(next)
}

function clearAll(): void {
  applyOverrides({})
}

function isOverridden(id: string): boolean {
  return overrides.value[id] !== undefined
}

/**
 * Command ids (other than excludeId) that also bind `binding`. A binding that
 * only prefixes another (`g` vs. `g i`) is not a conflict — the Zed rule makes
 * both functional — so this checks exact-string equality only.
 */
function conflicts(binding: string, excludeId?: string): string[] {
  const canon = canonicalizeBinding(binding)
  if (!canon) return []
  const ids: string[] = []
  for (const command of commands.value) {
    if (command.id === excludeId) continue
    if (effectiveBindings.value[command.id].includes(canon)) ids.push(command.id)
  }
  return ids
}

/** True when `combo` carries the platform's primary modifier (Cmd/Ctrl). */
function hasPrimaryModifier(combo: string): boolean {
  return combo.split('+').includes('mod')
}

/**
 * The pure sequence transition: given the steps accepted so far (null when no
 * sequence is pending) and the newly typed combo, decides whether to dispatch,
 * extend the pending sequence, swallow the keystroke, or pass it through to
 * normal single-combo dispatch. Reads the live keymap (exactBindings /
 * sequencePrefixes) but never mutates pendingSequence — the caller does.
 */
export function stepSequence(pending: PendingSequence | null, combo: string): SequenceTransition {
  const steps = pending ? [...pending.steps, combo] : [combo]
  const joined = steps.join(' ')

  const nextSteps = sequencePrefixes.value.get(joined)
  if (nextSteps && nextSteps.size > 0) {
    const continuations: SequenceContinuation[] = [...nextSteps].map(([step, commandId]) => ({ step, commandId }))
    return {
      kind: 'extend',
      pending: { steps, continuations },
      deferredCommandId: exactBindings.value.get(joined) ?? null,
    }
  }

  const commandId = exactBindings.value.get(joined)
  if (commandId) return pending ? { kind: 'run', commandId } : { kind: 'pass' }

  if (!pending) return { kind: 'pass' }
  // A bound-elsewhere modifier chord (e.g. ⌘K) is a command in its own right
  // even mid-sequence, so it falls through to normal dispatch rather than
  // being eaten; only a bare/unmodified miss is swallowed as a typo.
  return hasPrimaryModifier(combo) ? { kind: 'pass' } : { kind: 'swallow' }
}

export function useKeybindings() {
  return {
    /** Effective id → combos[] map (defaults ⊕ overrides). */
    bindings: effectiveBindings,
    recording,
    combosFor,
    resolve,
    addBinding,
    removeBinding,
    resetToDefault,
    clearAll,
    isOverridden,
    conflicts,
    stepSequence,
    /** Module state; the hint pill reads this to know what's pending. */
    pendingSequence,
    clearPendingSequence,
  }
}
