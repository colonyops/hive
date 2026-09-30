import { Events } from '@wailsio/runtime'
import { useToasts } from '../composables/useToasts'
import { prepareTerminalImages } from './terminalImagesClient'
import { interceptPaste } from './terminalPaste'

export interface ImagePasteTarget {
  current(): boolean
  paste(text: string, signal: AbortSignal): void | Promise<void>
  focus(): void
}

interface ImageInputOptions {
  capture(): ImagePasteTarget | null
  pasteText(text: string): void
}

const targets = new Map<string, (paths: string[]) => void>()
let releaseDrops: (() => void) | undefined
let nextTarget = 0

export function installTerminalImages(host: HTMLElement, options: ImageInputOptions): () => void {
  const id = `terminal-image-target-${++nextTarget}`
  const previousId = host.id
  host.id = id
  host.setAttribute('data-file-drop-target', '')
  const { showToast, dismissToast } = useToasts()
  const requests = new Set<AbortController>()
  let disposed = false
  let queue = Promise.resolve()
  const visible = () => !disposed && host.isConnected && host.getClientRects().length > 0

  function receive(input: File[] | string[]): void {
    if (!visible() || input.length === 0) return
    const target = options.capture()
    if (!target?.current()) return
    const controller = new AbortController()
    requests.add(controller)
    const current = () => visible() && !controller.signal.aborted && target.current()
    queue = queue.then(async () => {
      if (!current()) return
      const toast = showToast('Preparing image…', { duration: 0 })
      try {
        const pastes = await prepareTerminalImages(input, controller.signal)
        for (const text of pastes) {
          if (!current()) return
          await target.paste(text, controller.signal)
        }
        if (current()) target.focus()
      } catch (error) {
        if (current()) showToast(error instanceof Error ? error.message : 'Could not insert the image.', { severity: 'error' })
      } finally {
        dismissToast(toast)
      }
    }).finally(() => requests.delete(controller))
  }

  targets.set(id, receive)
  if (!releaseDrops) {
    releaseDrops = Events.On('terminal:files-dropped', (event) => {
      const data = event.data as { target: string; paths: string[] }
      targets.get(data.target)?.(data.paths)
    })
  }

  // Native Wails owns OS drops. Browser File drops are used by the headless
  // surface; handling both in Wails would insert the same image twice.
  const native = (window as Window & { _wails?: { flags?: { enableFileDrop?: boolean } } })._wails?.flags?.enableFileDrop === true
  const releasePaste = interceptPaste(host, options.pasteText, receive)
  const dragOver = (event: DragEvent) => {
    if (!native && event.dataTransfer?.types.includes('Files')) {
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy'
    }
  }
  const drop = (event: DragEvent) => {
    if (native || !event.dataTransfer?.files.length) return
    event.preventDefault()
    event.stopPropagation()
    receive(Array.from(event.dataTransfer.files))
  }
  host.addEventListener('dragover', dragOver)
  host.addEventListener('drop', drop)
  const visibility = new MutationObserver(() => {
    if (!visible()) for (const request of requests) request.abort()
  })
  for (let parent: HTMLElement | null = host; parent; parent = parent.parentElement) {
    visibility.observe(parent, { attributes: true, attributeFilter: ['style', 'class', 'hidden'] })
  }
  return () => {
    disposed = true
    for (const request of requests) request.abort()
    targets.delete(id)
    if (targets.size === 0) { releaseDrops?.(); releaseDrops = undefined }
    releasePaste()
    visibility.disconnect()
    host.removeEventListener('dragover', dragOver)
    host.removeEventListener('drop', drop)
    host.removeAttribute('data-file-drop-target')
    host.id = previousId
  }
}
