import { inject, ref, type InjectionKey, type Ref } from 'vue'

export interface Toast {
  message: Ref<string>
  show(text: string): void
}

export const toastKey: InjectionKey<Toast> = Symbol('toast')

/** One short confirmation at a time, shown by ToastHost. */
export function createToast(): Toast {
  const message = ref('')
  let timer: ReturnType<typeof setTimeout> | undefined
  return {
    message,
    show(text) {
      message.value = text
      if (timer !== undefined) clearTimeout(timer)
      timer = setTimeout(() => {
        message.value = ''
      }, 2500)
    },
  }
}

/** The app's toast; a component mounted without it drops the message. */
export function useToast(): Toast {
  return inject(toastKey, { message: ref(''), show: () => {} })
}
