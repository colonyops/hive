import { describe as group, expect, it } from 'vitest'
import { buildCron, defaultShape, describe, parseCron, type ScheduleShape } from '../scheduleShape'

const shapes: ScheduleShape[] = [
  { kind: 'hourly', minute: 30 },
  { kind: 'daily', hour: 9, minute: 0 },
  { kind: 'weekly', days: [1, 3, 5], hour: 9, minute: 0 },
  { kind: 'weekly', days: [0], hour: 23, minute: 59 },
  { kind: 'monthly', day: 1, hour: 9, minute: 0 },
  { kind: 'custom', cron: '0 */2 * * *' },
]

group('scheduleShape', () => {
  it('round-trips every kind through cron', () => {
    for (const shape of shapes) expect(parseCron(buildCron(shape))).toEqual(shape)
  })

  it('compiles each kind to the cron the manifest stores', () => {
    expect(buildCron({ kind: 'hourly', minute: 15 })).toBe('15 * * * *')
    expect(buildCron({ kind: 'daily', hour: 9, minute: 0 })).toBe('0 9 * * *')
    expect(buildCron({ kind: 'weekly', days: [5, 1, 3], hour: 9, minute: 0 })).toBe('0 9 * * 1,3,5')
    expect(buildCron({ kind: 'monthly', day: 28, hour: 7, minute: 5 })).toBe('5 7 28 * *')
    expect(buildCron({ kind: 'custom', cron: '@daily' })).toBe('@daily')
  })

  it('reads a weekday range as the days it covers', () => {
    expect(parseCron('0 9 * * 1-5')).toEqual({ kind: 'weekly', days: [1, 2, 3, 4, 5], hour: 9, minute: 0 })
    expect(parseCron('0 9 * * 5-6,0')).toEqual({ kind: 'weekly', days: [0, 5, 6], hour: 9, minute: 0 })
  })

  // Anything the picker cannot state stays editable as the text it already is,
  // rather than being rewritten into a shape that means something else.
  it('falls back to custom for everything the picker cannot state', () => {
    const unstatable = [
      '@daily',
      '0 */2 * * *',
      '0 9 * *',
      '0 9 * * 1 *',
      '0 9 1 6 *',
      '0 9 1 * 1',
      '0 9 * * 9',
      '0 9 * * 5-1',
      '0 9 * * mon',
      '0 9 32 * *',
      '60 9 * * *',
      '0 24 * * *',
    ]
    for (const cron of unstatable) expect(parseCron(cron)).toEqual({ kind: 'custom', cron })
  })

  it('states a shape the way the card summarises it', () => {
    expect(describe({ kind: 'hourly', minute: 30 })).toBe('Every hour at :30')
    expect(describe({ kind: 'daily', hour: 9, minute: 0 })).toBe('Every day at 09:00')
    expect(describe({ kind: 'weekly', days: [1, 2, 3, 4, 5], hour: 9, minute: 0 })).toBe('Weekdays at 09:00')
    expect(describe({ kind: 'weekly', days: [5], hour: 9, minute: 0 })).toBe('Every Friday at 09:00')
    expect(describe({ kind: 'weekly', days: [1, 3, 5], hour: 9, minute: 0 })).toBe('Mon, Wed, Fri at 09:00')
    expect(describe({ kind: 'weekly', days: [0, 6], hour: 9, minute: 0 })).toBe('Sat, Sun at 09:00')
    expect(describe({ kind: 'weekly', days: [0, 1, 2, 3, 4, 5, 6], hour: 9, minute: 0 })).toBe('Every day at 09:00')
    // A card mid-edit, not a timetable: saying " at 09:00" would read as a
    // schedule that had simply lost its days.
    expect(describe({ kind: 'weekly', days: [], hour: 9, minute: 0 })).toBe('No days picked')
    expect(describe({ kind: 'monthly', day: 1, hour: 9, minute: 0 })).toBe('Monthly on the 1st at 09:00')
    expect(describe({ kind: 'monthly', day: 22, hour: 18, minute: 5 })).toBe('Monthly on the 22nd at 18:05')
    expect(describe({ kind: 'monthly', day: 13, hour: 0, minute: 0 })).toBe('Monthly on the 13th at 00:00')
    expect(describe({ kind: 'custom', cron: '0 */2 * * *' })).toBe('Custom: 0 */2 * * *')
  })

  it('starts a new schedule on Friday at 09:00', () => {
    expect(defaultShape()).toEqual({ kind: 'weekly', days: [5], hour: 9, minute: 0 })
    expect(buildCron(defaultShape())).toBe('0 9 * * 5')
  })
})
