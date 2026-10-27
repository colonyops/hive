import { onScopeDispose } from 'vue'
import { codeCopyButtonClass } from '../lib/githubMarkdown'
import { useHiveUiPlatform } from '../platform'
import { useClipboard } from './useClipboard'

const copiedFor = 1500

/**
 * Click handling for the copy button renderGithubMarkdown puts on every code
 * block. The markup arrives through v-html, so the button is plain HTML and
 * the host's click handler calls copyCode first: it answers true when the
 * click was a copy, so the host leaves links and everything else alone.
 */
export function useCodeCopy() {
  const { copy } = useClipboard()
  const platform = useHiveUiPlatform()
  const timers = new Set<ReturnType<typeof setTimeout>>()

  function copyCode(event: MouseEvent): boolean {
    const target = event.target
    if (!(target instanceof Element)) return false
    const button = target.closest(`button.${codeCopyButtonClass}`)
    if (!(button instanceof HTMLElement)) return false
    event.preventDefault()
    const code = button.parentElement?.querySelector('pre')?.textContent ?? ''
    void copyAndMark(button, code.replace(/\n$/, ''))
    return true
  }

  async function copyAndMark(button: HTMLElement, code: string): Promise<void> {
    if (!(await copy(code))) {
      platform.toast?.('Could not copy to the clipboard')
      return
    }
    button.classList.add('is-copied')
    const timer = setTimeout(() => {
      button.classList.remove('is-copied')
      timers.delete(timer)
    }, copiedFor)
    timers.add(timer)
  }

  onScopeDispose(() => {
    for (const timer of timers) clearTimeout(timer)
  })

  return { copyCode }
}
