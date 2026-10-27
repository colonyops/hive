import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { parseKittyCommand } from '../kitty/kittyCommand'
import { decodeKittyImage } from '../kitty/kittyDecode'

function pngHeader(width: number, height: number): string {
  const bytes = new Uint8Array(33)
  bytes.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52])
  const view = new DataView(bytes.buffer)
  view.setUint32(16, width)
  view.setUint32(20, height)
  return btoa(String.fromCharCode(...bytes))
}

describe('decodeKittyImage', () => {
  const createImageBitmap = vi.fn(() => Promise.resolve({} as ImageBitmap))

  beforeEach(() => {
    vi.stubGlobal('createImageBitmap', createImageBitmap)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    createImageBitmap.mockClear()
  })

  it('refuses a PNG whose declared pixels exceed the byte limit before decoding it', async () => {
    const command = parseKittyCommand('a=T,f=100')

    await expect(decodeKittyImage(command, pngHeader(16384, 16384), 1 << 20)).rejects.toThrow('byte limit')
    expect(createImageBitmap).not.toHaveBeenCalled()
  })

  it('decodes a PNG whose declared pixels fit the byte limit', async () => {
    const command = parseKittyCommand('a=T,f=100')

    await decodeKittyImage(command, pngHeader(64, 64), 1 << 20)
    expect(createImageBitmap).toHaveBeenCalledOnce()
  })
})
