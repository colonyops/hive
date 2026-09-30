import { describe, expect, it } from 'vitest'
import { appErrorKind, appErrorMessage, errorText } from '../appError'

// A thrown binding error is a real Error whose `cause` is the JSON the Go
// adapter's MarshalError produced. These fixtures mirror that exactly — if
// the wire shape changes, this is what should fail.
function bindingError(cause: unknown): Error {
  const error = new Error('a bound method returned an error')
  ;(error as Error & { cause?: unknown }).cause = cause
  return error
}

describe('appErrorKind', () => {
  it('narrows every Kind the Go core assigns', () => {
    for (const kind of ['internal', 'invalid', 'not_found', 'conflict', 'unauthenticated', 'unavailable']) {
      expect(appErrorKind(bindingError({ kind, message: 'x' }))).toBe(kind)
    }
  })

  it('returns null for an error that did not come from a binding call', () => {
    expect(appErrorKind(new Error('plain'))).toBeNull()
    expect(appErrorKind('a string')).toBeNull()
    expect(appErrorKind(null)).toBeNull()
    expect(appErrorKind(undefined)).toBeNull()
  })

  it('returns null rather than trusting an unknown kind', () => {
    // An older or newer backend must not be able to make a caller take a
    // branch it did not mean.
    expect(appErrorKind(bindingError({ kind: 'teapot', message: 'x' }))).toBeNull()
    expect(appErrorKind(bindingError({ kind: 7 }))).toBeNull()
    expect(appErrorKind(bindingError({}))).toBeNull()
    expect(appErrorKind(bindingError('not an object'))).toBeNull()
  })
})

describe('appErrorMessage', () => {
  it('reads the core message', () => {
    expect(appErrorMessage(bindingError({ kind: 'not_found', message: 'action run 7 not found' })))
      .toBe('action run 7 not found')
  })

  it('is empty when there is no message to read', () => {
    expect(appErrorMessage(new Error('plain'))).toBe('')
    expect(appErrorMessage(bindingError({ kind: 'internal' }))).toBe('')
  })
})

describe('errorText', () => {
  it('prefers the core message over the runtime one the binding threw', () => {
    expect(errorText(bindingError({ kind: 'invalid', message: 'flow id must be a slug' }), 'fallback'))
      .toBe('flow id must be a slug')
  })

  it('falls back to the thrown message, then to the caller\'s wording', () => {
    expect(errorText(new Error('fetch failed'), 'fallback')).toBe('fetch failed')
    expect(errorText(new Error(''), 'fallback')).toBe('fallback')
    expect(errorText('a string', 'fallback')).toBe('fallback')
    expect(errorText(undefined, 'fallback')).toBe('fallback')
  })
})
