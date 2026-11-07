import { describe, expect, it } from 'vitest'
import { useHubOverlays } from '../useHubOverlays'

describe('useHubOverlays', () => {
  it('toggles Activity', () => {
    const overlays = useHubOverlays()
    overlays.toggleActivity()
    expect(overlays.activityOpen.value).toBe(true)
    overlays.toggleActivity()
    expect(overlays.activityOpen.value).toBe(false)
  })
})
