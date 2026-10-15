import type { KittyCommand } from './kittyCommand'

const PNG = 100
const RGBA = 32
const RGB = 24

/**
 * Turns a completed transmission into a bitmap. `maxBytes` bounds what a
 * compressed payload may inflate to, and the pixels a PNG may decode to.
 */
export async function decodeKittyImage(command: KittyCommand, base64: string, maxBytes: number): Promise<ImageBitmap> {
  if (typeof createImageBitmap !== 'function') throw new Error('createImageBitmap is unavailable')
  let bytes = decodeBase64(base64)
  if (command.compression === 'z') bytes = await inflate(bytes, maxBytes)
  else if (command.compression) throw new Error(`unsupported compression ${command.compression}`)

  switch (command.format) {
    case PNG:
      if (pngPixelBytes(bytes) > maxBytes) throw new Error('png exceeds the byte limit')
      return createImageBitmap(new Blob([bytes], { type: 'image/png' }))
    case RGBA:
    case RGB:
      return createImageBitmap(rawPixels(bytes, command))
    default:
      throw new Error(`unsupported format ${command.format}`)
  }
}

const PNG_SIGNATURE = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]

// A PNG compresses a blank canvas about a thousandfold, so a payload well under
// the limit can still decode to gigabytes. The size comes from the IHDR chunk,
// which the format requires first; bytes that are not a PNG are left to the
// decoder to refuse.
function pngPixelBytes(bytes: Uint8Array): number {
  if (bytes.length < 24 || PNG_SIGNATURE.some((byte, i) => bytes[i] !== byte)) return 0
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  return view.getUint32(16) * view.getUint32(20) * 4
}

function decodeBase64(base64: string): Uint8Array<ArrayBuffer> {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

async function inflate(bytes: Uint8Array<ArrayBuffer>, maxBytes: number): Promise<Uint8Array<ArrayBuffer>> {
  const reader = new Blob([bytes]).stream().pipeThrough(new DecompressionStream('deflate')).getReader()
  const parts: Uint8Array[] = []
  let size = 0
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    size += value.length
    if (size > maxBytes) {
      await reader.cancel()
      throw new Error('inflated image exceeds the byte limit')
    }
    parts.push(value)
  }
  const result = new Uint8Array(size)
  let offset = 0
  for (const part of parts) {
    result.set(part, offset)
    offset += part.length
  }
  return result
}

function rawPixels(bytes: Uint8Array, command: KittyCommand): ImageData {
  const { sourceWidth: width, sourceHeight: height } = command
  if (width <= 0 || height <= 0) throw new Error('raw pixels need s and v')
  const channels = command.format === RGBA ? 4 : 3
  if (bytes.length < width * height * channels) throw new Error('raw pixel data is short')
  const rgba = new Uint8ClampedArray(width * height * 4)
  if (channels === 4) {
    rgba.set(bytes.subarray(0, rgba.length))
  } else {
    for (let source = 0, target = 0; target < rgba.length; source += 3, target += 4) {
      rgba[target] = bytes[source]
      rgba[target + 1] = bytes[source + 1]
      rgba[target + 2] = bytes[source + 2]
      rgba[target + 3] = 255
    }
  }
  return new ImageData(rgba, width, height)
}
