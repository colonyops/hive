import { nextTick, onMounted, type Ref } from 'vue'

type Focusable = HTMLElement | { focus: () => void }

export function useAutofocus(target: Ref<Focusable | null>): void {
  onMounted(async () => {
    await nextTick()
    target.value?.focus()
  })
}
