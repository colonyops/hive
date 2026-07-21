import { describe, expect, it } from 'vitest'
import { isEditableTarget } from '../isEditableTarget'

describe('isEditableTarget', () => {
  it('returns true for input elements', () => {
    expect(isEditableTarget(document.createElement('input'))).toBe(true)
  })

  it('returns true for textarea elements', () => {
    expect(isEditableTarget(document.createElement('textarea'))).toBe(true)
  })

  it('returns true for select elements', () => {
    expect(isEditableTarget(document.createElement('select'))).toBe(true)
  })

  it('returns true for contentEditable elements', () => {
    const el = document.createElement('div')
    el.contentEditable = 'true'

    expect(isEditableTarget(el)).toBe(true)
  })

  it('returns false for plain div elements', () => {
    expect(isEditableTarget(document.createElement('div'))).toBe(false)
  })

  it('returns false for null', () => {
    expect(isEditableTarget(null)).toBe(false)
  })
})
