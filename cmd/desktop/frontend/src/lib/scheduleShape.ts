// The calendar-shaped view of a schedule's cron expression: what the workspace
// editor's Repeat picker edits, and the two conversions between it and the
// string the manifest stores. Cron stays the stored form -- the Go side parses
// and fires on it -- so this never becomes a second source of truth; it only
// decides which simple shape an expression already is, and falls back to
// `custom` for every expression it cannot state, which keeps a hand-written
// entry editable rather than rewriting it.

export type ScheduleShape =
  | { kind: 'hourly'; minute: number }
  | { kind: 'daily'; hour: number; minute: number }
  | { kind: 'weekly'; days: number[]; hour: number; minute: number }
  | { kind: 'monthly'; day: number; hour: number; minute: number }
  | { kind: 'custom'; cron: string }

const DAY_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const DAY_ABBREVIATIONS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

/**
 * Monday-first, which is the order the day chips sit in and the order a mixed
 * set is read back in. The numbers themselves stay cron's (0 = Sunday).
 */
export const WEEK_ORDER: readonly number[] = [1, 2, 3, 4, 5, 6, 0]

const WEEKDAYS = [1, 2, 3, 4, 5]

function dayName(day: number): string {
  return DAY_NAMES[day] ?? String(day)
}

export function dayAbbreviation(day: number): string {
  return DAY_ABBREVIATIONS[day] ?? String(day)
}

export function buildCron(shape: ScheduleShape): string {
  switch (shape.kind) {
    case 'hourly':
      return `${shape.minute} * * * *`
    case 'daily':
      return `${shape.minute} ${shape.hour} * * *`
    case 'weekly':
      return `${shape.minute} ${shape.hour} * * ${[...shape.days].sort((a, b) => a - b).join(',')}`
    case 'monthly':
      return `${shape.minute} ${shape.hour} ${shape.day} * *`
    case 'custom':
      return shape.cron
  }
}

export function parseCron(cron: string): ScheduleShape {
  const custom: ScheduleShape = { kind: 'custom', cron }
  const fields = cron.trim().split(/\s+/)
  if (fields.length !== 5) return custom

  const [minuteField, hourField, dayField, monthField, weekdayField] = fields
  if (monthField !== '*') return custom

  const minute = numeric(minuteField, 0, 59)
  if (minute === null) return custom

  if (hourField === '*') {
    return dayField === '*' && weekdayField === '*' ? { kind: 'hourly', minute } : custom
  }

  const hour = numeric(hourField, 0, 23)
  if (hour === null) return custom

  if (dayField === '*') {
    if (weekdayField === '*') return { kind: 'daily', hour, minute }
    const days = parseWeekdays(weekdayField)
    return days ? { kind: 'weekly', days, hour, minute } : custom
  }

  if (weekdayField !== '*') return custom
  const day = numeric(dayField, 1, 31)
  return day === null ? custom : { kind: 'monthly', day, hour, minute }
}

export function describe(shape: ScheduleShape): string {
  switch (shape.kind) {
    case 'hourly':
      return `Every hour at :${pad(shape.minute)}`
    case 'daily':
      return `Every day at ${clock(shape.hour, shape.minute)}`
    // An empty day set is a card mid-edit, not a timetable: it compiles to a
    // four-field expression the Go side rejects, so the summary says what is
    // missing rather than reading as a schedule with the days dropped.
    case 'weekly':
      return shape.days.length
        ? `${weekdayPhrase(shape.days)} at ${clock(shape.hour, shape.minute)}`
        : 'No days picked'
    case 'monthly':
      return `Monthly on the ${ordinal(shape.day)} at ${clock(shape.hour, shape.minute)}`
    case 'custom':
      return `Custom: ${shape.cron}`
  }
}

export function defaultShape(): ScheduleShape {
  return { kind: 'weekly', days: [5], hour: 9, minute: 0 }
}

function weekdayPhrase(days: number[]): string {
  const set = new Set(days)
  if (set.size === 7) return 'Every day'
  if (set.size === WEEKDAYS.length && WEEKDAYS.every((day) => set.has(day))) return 'Weekdays'
  if (set.size === 1) return `Every ${dayName(days[0])}`
  return WEEK_ORDER.filter((day) => set.has(day)).map(dayAbbreviation).join(', ')
}

// Weekday lists are the one field a simple shape reads as more than a number:
// `1,3,5` and `1-5` are both the ordinary way to write "these days", and
// rejecting either would send a perfectly plain schedule to the cron box.
function parseWeekdays(field: string): number[] | null {
  const days = new Set<number>()
  for (const term of field.split(',')) {
    const range = term.split('-')
    if (range.length === 1) {
      const day = numeric(range[0], 0, 6)
      if (day === null) return null
      days.add(day)
      continue
    }
    if (range.length !== 2) return null
    const from = numeric(range[0], 0, 6)
    const to = numeric(range[1], 0, 6)
    if (from === null || to === null || from > to) return null
    for (let day = from; day <= to; day++) days.add(day)
  }
  return days.size ? [...days].sort((a, b) => a - b) : null
}

function numeric(field: string, min: number, max: number): number | null {
  if (!/^\d{1,2}$/.test(field)) return null
  const value = Number(field)
  return value >= min && value <= max ? value : null
}

export function pad(value: number): string {
  return String(value).padStart(2, '0')
}

export function clock(hour: number, minute: number): string {
  return `${pad(hour)}:${pad(minute)}`
}

function ordinal(day: number): string {
  const teen = day % 100
  if (teen >= 11 && teen <= 13) return `${day}th`
  switch (day % 10) {
    case 1: return `${day}st`
    case 2: return `${day}nd`
    case 3: return `${day}rd`
    default: return `${day}th`
  }
}
