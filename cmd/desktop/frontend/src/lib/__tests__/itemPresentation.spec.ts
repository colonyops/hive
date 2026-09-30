import { describe, expect, it } from 'vitest'
import {
  DEFAULT_ITEM_KIND,
  bodySnippet,
  canonicalPayload,
  clipboardText,
  container,
  containerLine,
  kind,
  kindLabel,
  kindStyle,
  presentationFor,
  searchText,
  sourceKindForNodeType,
  sourceSummary,
} from '../itemPresentation'
import type { InboxItem } from '../../types/feed'

const baseItem: InboxItem = {
  id: 42, profileId: 'triage', sourceKind: 'github', sourceScope: 'colonyops/hive', externalId: 'pr-42', title: 'Add desktop shell', url: 'https://github.com/hay-kot/hive-desktop/pull/42',
  payload: { id: 'pr-42', kind: 'PR', repo: 'colonyops/hive', num: 42, author: 'octocat', branch: 'feat/desktop-ui-shell', body: 'First line\n\nSecond line' }, revision: 3, unread: true, lifecycle: 'active', firstSeenAt: 1, lastEventAt: 2,
}

describe('canonicalPayload', () => {
  it('decodes the canonical fields from an object payload', () => {
    expect(canonicalPayload(baseItem)).toEqual({
      kind: 'PR', repo: 'colonyops/hive', num: 42, author: 'octocat', body: 'First line\n\nSecond line',
      url: baseItem.url, labels: [], state: '',
    })
  })

  it('degrades a non-object payload to empty fields', () => {
    for (const payload of [null, undefined, 'a string', 42, true]) {
      expect(canonicalPayload({ ...baseItem, payload })).toEqual({ kind: '', repo: '', num: 0, author: '', body: '', url: baseItem.url, labels: [], state: '' })
    }
  })

  it('degrades an array payload to empty fields', () => {
    expect(canonicalPayload({ ...baseItem, payload: ['not', 'an', 'object'] })).toEqual({ kind: '', repo: '', num: 0, author: '', body: '', url: baseItem.url, labels: [], state: '' })
  })

  it('degrades a mistyped num to 0 rather than throwing', () => {
    expect(canonicalPayload({ ...baseItem, payload: { ...baseItem.payload as object, num: '42' } }).num).toBe(0)
  })

  it('degrades labels that are not a string array to an empty array', () => {
    expect(canonicalPayload({ ...baseItem, payload: { ...baseItem.payload as object, labels: 'not-an-array' } }).labels).toEqual([])
    expect(canonicalPayload({ ...baseItem, payload: { ...baseItem.payload as object, labels: ['a', 2, 'b'] } }).labels).toEqual(['a', 'b'])
  })

  it('falls back to the item url column when the payload carries none', () => {
    expect(canonicalPayload({ ...baseItem, payload: { kind: 'PR' } }).url).toBe(baseItem.url)
  })
})

describe('kind / kindLabel / kindStyle', () => {
  it('maps known kinds to human labels and styles', () => {
    expect(kind(baseItem)).toBe('PR')
    expect(kindLabel(baseItem)).toBe('Pull Request')
    expect(kindStyle(baseItem)).toBe('pr')

    const issue = { ...baseItem, payload: { ...baseItem.payload as object, kind: 'Issue' } }
    expect(kindLabel(issue)).toBe('Issue')
    expect(kindStyle(issue)).toBe('issue')
  })

  it('falls back to the raw kind and neutral style for provider-defined kinds', () => {
    const alert = { ...baseItem, payload: { ...baseItem.payload as object, kind: 'Alert' } }
    expect(kindLabel(alert)).toBe('Alert')
    expect(kindStyle(alert)).toBe('neutral')
  })

  // Every item carries a kind so every item is automatable — an untyped
  // payload reports DEFAULT_ITEM_KIND rather than an empty string, which is
  // what puts it in the actions editor's applies_to autocomplete.
  it('gives an untyped payload the default kind', () => {
    for (const payload of [{}, null, 'a string', { kind: '   ' }]) {
      const untyped = { ...baseItem, payload }
      expect(kind(untyped)).toBe(DEFAULT_ITEM_KIND)
      expect(kindLabel(untyped)).toBe('Item')
      expect(kindStyle(untyped)).toBe('neutral')
    }
  })

  it('keeps the raw decode free of the default, so "was a kind sent?" stays answerable', () => {
    expect(canonicalPayload({ ...baseItem, payload: {} }).kind).toBe('')
  })
})

describe('container / containerLine', () => {
  it('joins the repo and a positive ordinal', () => {
    expect(container(baseItem)).toBe('colonyops/hive')
    expect(containerLine(baseItem)).toBe('colonyops/hive #42')
  })

  it('omits the ordinal when num is absent or non-positive', () => {
    expect(containerLine({ ...baseItem, payload: { ...baseItem.payload as object, num: 0 } })).toBe('colonyops/hive')
  })
})

describe('bodySnippet', () => {
  it('takes the first non-empty line and strips heading markers', () => {
    expect(bodySnippet('## Summary\n\nMore detail')).toBe('Summary')
  })

  it('strips list, task-list, and blockquote markers', () => {
    expect(bodySnippet('- [ ] do the thing')).toBe('do the thing')
    expect(bodySnippet('> quoted intro')).toBe('quoted intro')
  })

  it('reduces links to their text and drops inline emphasis', () => {
    expect(bodySnippet('see **[the docs](https://example.com)** now')).toBe('see the docs now')
  })

  it('returns an empty string for a blank body', () => {
    expect(bodySnippet('\n\n')).toBe('')
  })
})

describe('searchText', () => {
  it('joins title, numbered container line, byline, kind label, source label, and snippet', () => {
    expect(searchText(baseItem)).toBe('Add desktop shell colonyops/hive #42 octocat Pull Request GitHub First line')
  })
})

describe('clipboardText', () => {
  it('formats a self-contained header plus the raw markdown body', () => {
    expect(clipboardText(baseItem)).toBe(
      'Add desktop shell\ncolonyops/hive #42 · https://github.com/hay-kot/hive-desktop/pull/42\n\nFirst line\n\nSecond line',
    )
  })

  it('drops the body block when the item has none', () => {
    const item = { ...baseItem, payload: { ...(baseItem.payload as object), body: '  ' } }
    expect(clipboardText(item)).toBe('Add desktop shell\ncolonyops/hive #42 · https://github.com/hay-kot/hive-desktop/pull/42')
  })

  it('degrades to title and url for a payload without metadata', () => {
    const item = { ...baseItem, payload: null }
    expect(clipboardText(item)).toBe('Add desktop shell\nhttps://github.com/hay-kot/hive-desktop/pull/42')
  })
})

describe('presentationFor', () => {
  it('resolves github, grafana, and webhook adapters', () => {
    expect(presentationFor('github').sourceLabel).toBe('GitHub')
    expect(presentationFor('grafana').sourceLabel).toBe('Grafana')
    expect(presentationFor('webhook').sourceLabel).toBe('Webhook')
  })

  it('resolves the colored grafana logo image, with a glyph fallback', () => {
    // A colored image (the gradient logo) rather than a currentColor glyph, so
    // grafana items stay distinct in a feed that mixes providers.
    expect(presentationFor('grafana').markImage?.(baseItem)).toBeTruthy()
    // The fallback glyph still resolves for when the bundled image fails.
    expect(presentationFor('grafana').mark(baseItem)).toBeDefined()
  })

  it('falls back to the default adapter for an unknown or absent sourceKind, echoing the raw kind', () => {
    expect(presentationFor('generic').sourceLabel).toBe('generic')
    expect(presentationFor(undefined).sourceLabel).toBe('')
  })

  it('resolves the webhook mark from the configured source icon, falling back to the webhook glyph', () => {
    const withIcon = presentationFor('webhook').mark(baseItem, { sourceIcons: { 'colonyops/hive': 'bell' } })
    const withoutIcon = presentationFor('webhook').mark(baseItem, {})
    const withoutContext = presentationFor('webhook').mark(baseItem)
    expect(withIcon).toBeDefined()
    expect(withoutIcon).toBeDefined()
    expect(withoutContext).toBeDefined()
    expect(withIcon).not.toBe(withoutIcon)
  })

  it('resolves an uploaded mark image from sourceImages by scope, and none otherwise', () => {
    const url = 'data:image/png;base64,LOGO'
    for (const kind of ['webhook', 'exec']) {
      expect(presentationFor(kind).markImage?.(baseItem, { sourceImages: { 'colonyops/hive': url } })).toBe(url)
      expect(presentationFor(kind).markImage?.(baseItem, {})).toBeUndefined()
      expect(presentationFor(kind).markImage?.(baseItem)).toBeUndefined()
    }
    // Only the adapters whose node config carries an `image` resolve one.
    expect(presentationFor('github').markImage).toBeUndefined()
    expect(presentationFor('generic').markImage).toBeUndefined()
  })
})

describe('sourceKindForNodeType', () => {
  it('maps known source node types to their sourceKind', () => {
    expect(sourceKindForNodeType('sources.github')).toBe('github')
    expect(sourceKindForNodeType('sources.grafana_metrics')).toBe('grafana')
    expect(sourceKindForNodeType('sources.grafana_alerts')).toBe('grafana')
    expect(sourceKindForNodeType('sources.webhook')).toBe('webhook')
  })

  it('returns null for non-source node types', () => {
    expect(sourceKindForNodeType('feed')).toBeNull()
    expect(sourceKindForNodeType('action')).toBeNull()
  })
})

describe('sourceSummary', () => {
  it('counts sources across kinds, provider-neutral', () => {
    expect(sourceSummary(new Map([['github', 1]]))).toBe('1 source')
    expect(sourceSummary(new Map([['github', 3]]))).toBe('3 sources')
    expect(sourceSummary(new Map([['webhook', 2]]))).toBe('2 sources')
    expect(sourceSummary(new Map([['github', 2], ['webhook', 1]]))).toBe('3 sources')
  })

  it('reports no sources when the map is empty or all-zero', () => {
    expect(sourceSummary(new Map())).toBe('No sources')
    expect(sourceSummary(new Map([['github', 0]]))).toBe('No sources')
  })
})
