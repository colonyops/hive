// Client-side guards for the image picker, shared by the profile-avatar and
// webhook feed-mark pickers. The Go core does the authoritative decode/normalize.

export const imageUploadAccept = 'image/png,image/jpeg,image/gif,image/webp'
export const imageMaxBytes = 8 * 1024 * 1024 // mirrors the core's MaxInputBytes

export class ImageUploadError extends Error {}

/** Reads a picked image file into the bare base64 string the backend expects. */
export async function fileToImageBase64(file: File): Promise<string> {
  if (file.size === 0) throw new ImageUploadError('That file is empty.')
  if (file.size > imageMaxBytes) throw new ImageUploadError('That image is too large. Choose a file under 8 MB.')

  const dataUrl = await readAsDataURL(file)
  const comma = dataUrl.indexOf(',')
  if (comma < 0) throw new ImageUploadError('That image could not be read.')
  return dataUrl.slice(comma + 1)
}

function readAsDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(typeof reader.result === 'string' ? reader.result : '')
    reader.onerror = () => reject(new ImageUploadError('That image could not be read.'))
    reader.readAsDataURL(file)
  })
}
