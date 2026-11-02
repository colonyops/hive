import { describe, expect, it } from 'vitest'
import { appletCommandID, appletDeclaredCommand, appletID } from '../catalog'

describe('applet command ids', () => {
  it('reads the panel of an applet', () => {
    expect(appletCommandID('radio')).toBe('applet.radio')
    expect(appletID('applet.radio')).toBe('radio')
    expect(appletDeclaredCommand('applet.radio')).toBeNull()
  })

  it('reads a command the applet declared', () => {
    expect(appletCommandID('radio', 'next')).toBe('applet.radio.next')
    expect(appletID('applet.radio.next')).toBe('radio')
    expect(appletDeclaredCommand('applet.radio.next')).toBe('next')
  })

  it('leaves every other command alone', () => {
    expect(appletID('palette.toggle')).toBeNull()
    expect(appletDeclaredCommand('palette.toggle')).toBeNull()
  })
})
