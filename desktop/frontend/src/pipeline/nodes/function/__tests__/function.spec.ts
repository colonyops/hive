import { describe, expect, it } from 'vitest'
import { checkSyntax, compile, type Config } from '../config'
import functionRuntime from '../runtime'
import type { NodeContext } from '../../../engine/transport'
import type { Msg } from '../../../types'

function msg(id: string, payload: any = {}): Msg {
  return { ID: id, Key: id, Topic: 'source:test', Ts: 0, Payload: payload, Meta: null }
}

function ctx(config: Config, state: Record<string, any> = {}): NodeContext<Config> {
  return { config, state }
}

describe('function node config: compile/checkSyntax', () => {
  it('compiles a valid on_message body into a callable', () => {
    const fn = compile('return msg')
    const m = msg('1', { a: 1 })
    expect(fn(m, {}, {})).toBe(m)
  })

  it('checkSyntax reports no errors for valid source', () => {
    expect(checkSyntax('return msg')).toEqual([])
  })

  it('checkSyntax reports a syntax error without throwing', () => {
    const errors = checkSyntax('return msg(')
    expect(errors).toHaveLength(1)
    expect(errors[0]).toEqual(expect.any(String))
  })
})

describe('function node runtime', () => {
  it('on_message returning a single msg goes to port 0', async () => {
    const c = ctx({ on_message: 'msg.Payload.tag = "seen"; return msg' })
    const m = msg('1', { x: 1 })
    const result = await functionRuntime.onMsg(m, c)
    expect(result).toBe(m)
    expect((result as Msg).Payload.tag).toBe('seen')
  })

  it('on_message returning an array (outputs=1, the default) means multiple messages on port 0', async () => {
    const c = ctx({ on_message: 'return [msg, msg]' })
    const result = await functionRuntime.onMsg(msg('1', {}), c)
    expect(Array.isArray(result)).toBe(true)
    expect((result as Msg[]).length).toBe(2)
  })

  it('on_message returning a port-indexed array routes per port', async () => {
    const c = ctx({ on_message: 'return msg.Payload.ok ? [msg, null] : [null, msg]', outputs: 2 })
    const passMsg = msg('1', { ok: true })
    const failMsg = msg('2', { ok: false })
    expect(await functionRuntime.onMsg(passMsg, c)).toEqual([passMsg, null])
    expect(await functionRuntime.onMsg(failMsg, c)).toEqual([null, failMsg])
  })

  it('on_message returning null is a discard', async () => {
    const c = ctx({ on_message: 'return null' })
    expect(await functionRuntime.onMsg(msg('1', {}), c)).toBeNull()
  })

  it('on_start initializes state that on_message reads and mutates across calls', async () => {
    const c = ctx({
      on_start: 'state.count = 0',
      on_message: 'state.count++; msg.Payload.count = state.count; return msg',
    })
    await functionRuntime.start?.(c)
    expect(c.state.count).toBe(0)
    const r1 = (await functionRuntime.onMsg(msg('1', {}), c)) as Msg
    const r2 = (await functionRuntime.onMsg(msg('2', {}), c)) as Msg
    expect(r1.Payload.count).toBe(1)
    expect(r2.Payload.count).toBe(2)
  })

  it('on_stop runs with access to the accumulated state', async () => {
    const c = ctx({ on_message: 'return msg', on_stop: 'state.stopped = true' })
    await functionRuntime.stop?.(c)
    expect(c.state.stopped).toBe(true)
  })

  it('on_start/on_stop are no-ops when unset', () => {
    const c = ctx({ on_message: 'return msg' })
    expect(() => functionRuntime.start?.(c)).not.toThrow()
    expect(() => functionRuntime.stop?.(c)).not.toThrow()
  })
})
