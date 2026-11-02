import { onBeforeUnmount, watch } from 'vue'
import { useApplets } from '../../stores/useApplets'

/** Says this session area can show an applet as a tab while the user is in it. */
export function useAppletTabHost(active: () => boolean, open: (id: string) => void): void {
  const applets = useApplets()
  watch(active, (isActive) => applets.setTabHost(open, isActive), { immediate: true })
  onBeforeUnmount(() => applets.setTabHost(open, false))
}
