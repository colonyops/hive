import { describe, expect, it, vi } from 'vitest'
import { backdropForPixels, imagePreviewKind, imageReads, isImagePath } from '../diffImages'

function pixels(...rgba: [number, number, number, number][]): Uint8ClampedArray {
  return new Uint8ClampedArray(rgba.flat())
}
const white: [number, number, number, number] = [255, 255, 255, 255]
const black: [number, number, number, number] = [0, 0, 0, 255]
const clear: [number, number, number, number] = [0, 0, 0, 0]

describe('backdropForPixels', () => {
  it('puts light content on a dark grid and dark content on a light one', () => {
    expect(backdropForPixels(pixels(white, clear))).toBe('dark')
    expect(backdropForPixels(pixels(black, clear))).toBe('light')
  })

  it('leaves an opaque or empty image to the theme', () => {
    expect(backdropForPixels(pixels(white, white))).toBeNull()
    expect(backdropForPixels(pixels(clear, clear))).toBeNull()
  })

  it('weighs a pixel by how much of it shows', () => {
    expect(backdropForPixels(pixels([255, 255, 255, 20], black, clear))).toBe('light')
  })
})

describe('imagePreviewKind', () => {
  const file = (path: string) => ({
    path,
    oldPath: '',
    status: 'modified',
    additions: 0,
    deletions: 0,
    binary: true,
    stamp: '',
  })
  const diff = (state: string) => ({ path: '', state, text: '', error: '' })

  it('previews an image git could not diff, and an SVG above its text diff', () => {
    expect(imagePreviewKind(file('a.PNG'), diff('binary'))).toBe('binary')
    expect(imagePreviewKind(file('a.svg'), diff('text'))).toBe('text')
    expect(imagePreviewKind(file('a.png'), diff('truncated'))).toBeNull()
    expect(imagePreviewKind(file('a.bin'), diff('binary'))).toBeNull()
    expect(isImagePath('dir.png/file')).toBe(false)
  })
})

describe('imageReads', () => {
  const image = { path: 'a.png', state: 'ready', before: null, after: null, error: '' }

  it('reads a path once per version', async () => {
    const load = vi.fn().mockResolvedValue(image)
    const reads = imageReads(load)

    await reads.read('a.png', '1')
    await reads.read('a.png', '1')
    expect(load).toHaveBeenCalledTimes(1)

    await reads.read('a.png', '2')
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('reads again after a failed read', async () => {
    const load = vi.fn().mockRejectedValueOnce(new Error('daemon away')).mockResolvedValue(image)
    const reads = imageReads(load)

    await expect(reads.read('a.png', '1')).rejects.toThrow('daemon away')
    await expect(reads.read('a.png', '1')).resolves.toEqual(image)
    expect(load).toHaveBeenCalledTimes(2)
  })

  it('reads a pruned path again', async () => {
    const load = vi.fn().mockResolvedValue(image)
    const reads = imageReads(load)

    await reads.read('a.png', '1')
    reads.prune(() => false)
    await reads.read('a.png', '1')
    expect(load).toHaveBeenCalledTimes(2)
  })
})
