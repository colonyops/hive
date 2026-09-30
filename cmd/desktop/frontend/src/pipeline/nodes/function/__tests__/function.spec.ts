// Evaluation is Go's (internal/app/runtime/js, through goja) and its tests
// live there — including the return-shape rules, state across messages, and
// timeouts. What is left here is the editor's half: the live syntax check the
// drawer shows while typing.

import { describe, expect, it } from 'vitest'
import { DEFAULT_OUTPUTS, DEFAULT_TIMEOUT_MS, checkSyntax, compile, outputs, timeoutMs, validate } from '../config'

describe('function node: the drawer syntax check', () => {
  it('compiles a valid on_message body without throwing', () => {
    expect(() => compile('return msg')).not.toThrow()
  })

  it('reports no errors for valid source', () => {
    expect(checkSyntax('return msg')).toEqual([])
  })

  it('reports a syntax error without throwing', () => {
    const errors = checkSyntax('return msg(')
    expect(errors).toHaveLength(1)
    expect(errors[0]).toEqual(expect.any(String))
  })

  it('says nothing about code that is only wrong at run time — that is the engine\'s to report', () => {
    expect(checkSyntax('throw new Error("boom")')).toEqual([])
    expect(checkSyntax('return undefinedGlobal.field')).toEqual([])
  })
})

describe('function node config defaults', () => {
  it('defaults to one output and a five-second budget', () => {
    expect(outputs({ on_message: 'return msg' })).toBe(DEFAULT_OUTPUTS)
    expect(timeoutMs({ on_message: 'return msg' })).toBe(DEFAULT_TIMEOUT_MS)
  })

  it('requires on_message — the node has no other entry point', () => {
    expect(validate({ on_message: '' })).toEqual(['on_message is required'])
  })
})
