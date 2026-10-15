import { describe, expect, it } from 'vitest'
import { parseKittyCommand } from '../kitty/kittyCommand'

describe('parseKittyCommand', () => {
  it('reads the command Pi sends for an image', () => {
    const command = parseKittyCommand('a=T,f=100,q=2,C=1,c=40,r=12,i=3735928559,m=1;iVBORw0KGgo=')
    expect(command).toMatchObject({
      action: 'T',
      format: 100,
      quiet: 2,
      cursorMovement: 1,
      columns: 40,
      rows: 12,
      id: 3735928559,
      more: true,
      payload: 'iVBORw0KGgo=',
    })
    expect([...command.keys]).toEqual(['a', 'f', 'q', 'C', 'c', 'r', 'i', 'm'])
  })

  it('applies the protocol defaults to absent keys', () => {
    expect(parseKittyCommand('')).toMatchObject({
      action: 't',
      format: 32,
      medium: 'd',
      compression: '',
      target: 'a',
      more: false,
      id: 0,
      payload: '',
    })
  })

  it('reads a continuation chunk as only m and its payload', () => {
    const command = parseKittyCommand('m=0;QUJD')
    expect([...command.keys]).toEqual(['m'])
    expect(command.more).toBe(false)
    expect(command.payload).toBe('QUJD')
  })

  it('reads the delete forms', () => {
    expect(parseKittyCommand('a=d,d=I,i=9,q=2')).toMatchObject({ action: 'd', target: 'I', id: 9 })
    expect(parseKittyCommand('a=d,d=A,q=2')).toMatchObject({ action: 'd', target: 'A' })
    expect(parseKittyCommand('a=d,d=a,q=2')).toMatchObject({ action: 'd', target: 'a' })
  })

  it('reads placement, crop, offset, raw size, compression and a negative z', () => {
    expect(parseKittyCommand('a=p,i=2,p=5,x=1,y=2,w=3,h=4,X=5,Y=6,z=-1,s=10,v=20,o=z,t=d,I=8,U=1')).toMatchObject({
      action: 'p',
      id: 2,
      placement: 5,
      cropX: 1,
      cropY: 2,
      cropWidth: 3,
      cropHeight: 4,
      offsetX: 5,
      offsetY: 6,
      z: -1,
      sourceWidth: 10,
      sourceHeight: 20,
      compression: 'z',
      medium: 'd',
      number: 8,
      unicode: 1,
    })
  })

  it('skips malformed and unknown keys', () => {
    const command = parseKittyCommand('i=abc,c=1.5,a=Tx,Q=1,=3,r,f=24')
    expect([...command.keys]).toEqual(['f'])
    expect(command).toMatchObject({ id: 0, columns: 0, action: 't', format: 24 })
  })

  it('keeps every byte after the first semicolon as payload', () => {
    expect(parseKittyCommand('a=t;AB;CD').payload).toBe('AB;CD')
  })
})
