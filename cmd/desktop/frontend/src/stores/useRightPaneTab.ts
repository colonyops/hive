import { useStorage } from '@vueuse/core'
import { defineStore } from './defineStore'

export type RightPaneTab = 'drive' | 'git'

// The tab the right toggle reopens in Code. It starts on Drive, which is what
// the toggle opened before Git shared the slot.
export const useRightPaneTab = defineStore('rightPaneTab', () => ({
  lastTab: useStorage<RightPaneTab>('hive.code.rightPaneTab', 'drive'),
}))
