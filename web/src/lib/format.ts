// Dates and labels. Every date is shown in Europe/Berlin time.

const TZ = 'Europe/Berlin'

// en-US parts give "Sep" rather than the British "Sept". The order is ours.
const dayFmt = new Intl.DateTimeFormat('en-US', { timeZone: TZ, weekday: 'short', day: 'numeric', month: 'short' })
const timeFmt = new Intl.DateTimeFormat('en-GB', { timeZone: TZ, hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
const keyFmt = new Intl.DateTimeFormat('en-CA', { timeZone: TZ, year: 'numeric', month: '2-digit', day: '2-digit' })
const shortDayFmt = new Intl.DateTimeFormat('en-US', { timeZone: 'UTC', day: 'numeric', month: 'short' })
const rel = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })

function toDate(v: string | Date): Date {
  return typeof v === 'string' ? new Date(v) : v
}

// "Mon 28 Sep"
export function fmtDay(v: string | Date): string {
  const p = partsOf(dayFmt, toDate(v))
  return `${p.weekday} ${p.day} ${p.month}`
}

function partsOf(fmt: Intl.DateTimeFormat, d: Date): Record<string, string> {
  return Object.fromEntries(fmt.formatToParts(d).map((x) => [x.type, x.value]))
}

// "18:00"
export function fmtTime(v: string | Date): string {
  return timeFmt.format(toDate(v))
}

// "Mon 28 Sep, 18:00"
export function fmtDateTime(v: string | Date): string {
  return `${fmtDay(v)}, ${fmtTime(v)}`
}

// "2026-09-28", the calendar day in Berlin. Used to group events by day.
export function dayKey(v: string | Date): string {
  return keyFmt.format(toDate(v))
}

// Today in Berlin as "YYYY-MM-DD".
export function todayKey(): string {
  return dayKey(new Date())
}

// Adds days to a "YYYY-MM-DD" key.
export function addDays(key: string, days: number): string {
  const d = new Date(`${key}T12:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

// Day of week for a "YYYY-MM-DD" key, Monday = 0.
export function weekdayIndex(key: string): number {
  return (new Date(`${key}T12:00:00Z`).getUTCDay() + 6) % 7
}

// "Mon 28 Sep" for a "YYYY-MM-DD" key.
export function fmtDayKey(key: string): string {
  return fmtDay(new Date(`${key}T12:00:00Z`))
}

// "28 Sep" for a "YYYY-MM-DD" key, used on chart axes.
export function fmtShortDayKey(key: string): string {
  const p = partsOf(shortDayFmt, new Date(`${key}T12:00:00Z`))
  return `${p.day} ${p.month}`
}

export type Range = 'week' | 'next-week' | '30'

export const RANGES: { id: Range; label: string }[] = [
  { id: 'week', label: 'This week' },
  { id: 'next-week', label: 'Next week' },
  { id: '30', label: 'Next 30 days' },
]

// Inclusive "from" and "to" days for a range, weeks run Monday to Sunday.
export function rangeDays(range: Range, today = todayKey()): { from: string; to: string } {
  const monday = addDays(today, -weekdayIndex(today))
  if (range === 'week') return { from: today, to: addDays(monday, 6) }
  if (range === 'next-week') return { from: addDays(monday, 7), to: addDays(monday, 13) }
  return { from: today, to: addDays(today, 30) }
}

// "Today", "Tomorrow" or "Mon 28 Sep" for a day key.
export function dayLabel(key: string, today = todayKey()): string {
  if (key === today) return 'Today'
  if (key === addDays(today, 1)) return 'Tomorrow'
  return fmtDayKey(key)
}

// "3 hours ago", "in 2 days", "yesterday".
export function fmtAgo(v: string | Date | null | undefined, now = new Date()): string {
  if (!v) return 'never'
  const diff = (toDate(v).getTime() - now.getTime()) / 1000
  const abs = Math.abs(diff)
  if (abs < 60) return 'just now'
  if (abs < 3600) return rel.format(Math.round(diff / 60), 'minute')
  if (abs < 86400) return rel.format(Math.round(diff / 3600), 'hour')
  if (abs < 86400 * 30) return rel.format(Math.round(diff / 86400), 'day')
  return fmtDay(v)
}

// "1.8 s", "420 ms"
export function fmtDuration(ms: number): string {
  if (ms < 1000) return `${ms} ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(1)} s`
  const m = Math.floor(ms / 60000)
  const s = Math.round((ms % 60000) / 1000)
  return `${m} min ${s} s`
}

const numFmt = new Intl.NumberFormat('en-GB')
export function fmtNum(n: number): string {
  return numFmt.format(n)
}

export function plural(n: number, one: string, many = `${one}s`): string {
  return `${fmtNum(n)} ${n === 1 ? one : many}`
}

export const TYPE_LABEL: Record<string, string> = {
  pitch: 'Pitch',
  talk: 'Talk',
  panel: 'Panel',
  meetup: 'Meetup',
  workshop: 'Workshop',
  conference: 'Conference',
  sport: 'Sport',
  other: 'Other',
}

export const ROLE_LABEL: Record<string, string> = {
  speaker: 'speaker',
  panelist: 'panel',
  pitch: 'pitch',
  host: 'host',
  moderator: 'moderator',
}

export const STATUS_LABEL: Record<string, string> = {
  active: 'Active',
  probation: 'Probation',
  candidate: 'Candidate',
  manual: 'Manual',
  retired: 'Retired',
}

export const KIND_LABEL: Record<string, string> = {
  listing: 'Listing',
  calendar_luma: 'Luma calendar',
  calendar_meetup: 'Meetup group',
  calendar_eventbrite: 'Eventbrite',
  calendar_ical: 'iCal feed',
  organiser_page: 'Organiser page',
  profile_page: 'Profile page',
  newsletter: 'Newsletter',
  search_query: 'Web search',
}

export const MODE_LABEL: Record<string, string> = {
  auto: 'Auto',
  http: 'Plain HTTP',
  browser: 'Needs JavaScript',
}

export const PLATFORM_SHORT: Record<string, string> = {
  linkedin: 'in',
  instagram: 'ig',
  youtube: 'yt',
  tiktok: 'tt',
  x: 'x',
  website: 'web',
  luma: 'luma',
  meetup: 'mu',
  podcast: 'pod',
  other: 'link',
}

export const PLATFORM_LABEL: Record<string, string> = {
  linkedin: 'LinkedIn',
  instagram: 'Instagram',
  youtube: 'YouTube',
  tiktok: 'TikTok',
  x: 'X',
  website: 'Website',
  luma: 'Luma',
  meetup: 'Meetup',
  podcast: 'Podcast',
  other: 'Link',
}

export function initials(name: string): string {
  return name
    .replace(/^(Dr|Prof)\.\s+/i, '')
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0])
    .join('')
    .toUpperCase()
}

// "startplatz.de/events" from a full URL.
export function shortUrl(url: string | null | undefined): string {
  if (!url) return ''
  try {
    const u = new URL(url)
    const path = u.pathname === '/' ? '' : u.pathname.replace(/\/$/, '')
    return `${u.hostname.replace(/^www\./, '')}${path}`
  } catch {
    return url
  }
}

// Groups items by Berlin calendar day, keeping order.
export function groupByDay<T extends { starts_at: string }>(items: T[]): { key: string; items: T[] }[] {
  const groups: { key: string; items: T[] }[] = []
  const sorted = [...items].sort((a, b) => a.starts_at.localeCompare(b.starts_at))
  for (const item of sorted) {
    const key = dayKey(item.starts_at)
    const last = groups[groups.length - 1]
    if (last && last.key === key) last.items.push(item)
    else groups.push({ key, items: [item] })
  }
  return groups
}
