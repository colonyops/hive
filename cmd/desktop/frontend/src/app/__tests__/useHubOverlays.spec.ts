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

  it('opens Action runs on a run, and a plain toggle on the newest', () => {
    const overlays = useHubOverlays()
    overlays.openActionRuns(7)
    expect(overlays.actionRunsOpen.value).toBe(true)
    expect(overlays.actionRunsRunId.value).toBe(7)

    overlays.toggleActionRuns()
    overlays.toggleActionRuns()
    expect(overlays.actionRunsOpen.value).toBe(true)
    expect(overlays.actionRunsRunId.value).toBeNull()
  })
})
