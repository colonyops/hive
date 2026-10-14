import { readonly, ref } from 'vue'
import { defineStore } from './defineStore'

export const useCodeInstallation = defineStore('codeInstallation', () => {
  const installation = ref<'local' | 'remote'>('local')
  function select(value: 'local' | 'remote'): void {
    installation.value = value
  }
  return { installation: readonly(installation), select }
})
