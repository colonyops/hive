import { describe, expect, it } from 'vitest'
import IconTag from '~icons/lucide/tag'
import { defaultReviewLabels, labelOf, labelText, resolveReviewLabels, reviewLabelIcons } from '../reviewLabels'

describe('reviewLabels', () => {
  it('reads back the label of the text it saves, with or without extra words', () => {
    for (const label of defaultReviewLabels) {
      expect(labelOf(defaultReviewLabels, labelText(label, ''))).toBe(label)
      expect(labelOf(defaultReviewLabels, labelText(label, '  see step 2 '))).toBe(label)
    }
    expect(labelText(defaultReviewLabels[2], '  see step 2 ')).toBe('Needs tests: see step 2')
  })

  it('finds no label in text that only resembles one', () => {
    expect(labelOf(defaultReviewLabels, 'Needs testsX')).toBeNull()
    expect(labelOf(defaultReviewLabels, 'needs tests')).toBeNull()
    expect(labelOf(defaultReviewLabels, 'This needs tests')).toBeNull()
  })

  it('keys configured labels by position, at most four, with a tag for an unknown icon', () => {
    const labels = resolveReviewLabels([
      { title: 'Nit', icon: 'scissors' },
      { title: 'Why?', icon: 'nope' },
      { title: 'c', icon: '' },
      { title: 'd', icon: '' },
      { title: 'e', icon: '' },
    ])
    expect(labels.map((label) => label.key)).toEqual(['1', '2', '3', '4'])
    expect(labels[0].icon).toBe(reviewLabelIcons.scissors)
    expect(labels[1].icon).toBe(IconTag)
  })
})
