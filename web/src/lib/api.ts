// Typed client for the Speaker Trail HTTP API. See docs/api.md.
// All times are RFC 3339 strings in UTC.

export type EventType = 'pitch' | 'talk' | 'panel' | 'meetup' | 'workshop' | 'conference' | 'sport' | 'other'
export type EventFormat = 'in_person' | 'online' | 'hybrid'
export type Fit = 'kept' | 'dropped'
export type FitFilter = Fit | 'all'
export type AppearanceRole = 'speaker' | 'panelist' | 'pitch' | 'host' | 'moderator'
export type PersonFit = 'founder' | 'athlete' | 'maker' | 'creator' | 'other'
// What a lookup saw of a startup: its website changed lately, it shows no
// date, nothing changed for a year, it is being wound up, or it is gone.
export type Activity = 'active' | 'unknown' | 'quiet' | 'dissolved' | 'gone'
export type ProfilePlatform =
  | 'linkedin' | 'instagram' | 'youtube' | 'tiktok' | 'x' | 'website' | 'luma' | 'meetup' | 'podcast' | 'other'
export type Review = 'open' | 'confirmed' | 'rejected'
export type SourceStatus = 'candidate' | 'probation' | 'active' | 'retired' | 'manual'
export type SourceKind =
  | 'listing' | 'calendar_luma' | 'calendar_meetup' | 'calendar_eventbrite' | 'calendar_ical'
  | 'organiser_page' | 'profile_page' | 'newsletter' | 'search_query' | 'portfolio'
export type FetchMode = 'auto' | 'http' | 'browser'
export type Health = 'ok' | 'warning' | 'error' | 'never'
export type PeopleSort = 'next' | 'new' | 'name'
export type PeopleFilter = 'all' | 'upcoming' | 'profile' | 'founder'

export const EVENT_TYPES: EventType[] = ['pitch', 'talk', 'panel', 'meetup', 'workshop', 'conference', 'sport', 'other']
export const SOURCE_STATUSES: SourceStatus[] = ['active', 'probation', 'candidate', 'manual', 'retired']

// Public

export interface PublicConfig {
  public_calendar: boolean
  // True for Tim, logged in. He sees the calendar before it opens.
  owner: boolean
  show_people: boolean
  cities: string[]
}

export interface EventPerson {
  id?: number
  name: string
  role: string
  affiliation: string
}

export interface PublicEvent {
  id: number
  title: string
  starts_at: string
  ends_at: string | null
  city: string
  venue: string
  address: string
  format: EventFormat
  type: EventType
  url: string
  price: string
  organisers: string[]
  people: EventPerson[]
}

export interface EventQuery {
  from?: string
  to?: string
  city?: string
  type?: string
}

// Private

export interface SourceRef {
  id: number
  name: string
  url: string
}

export interface PrivateEvent extends PublicEvent {
  fit: Fit | null
  fit_reason: string
  people: (EventPerson & { id: number })[]
  sources: SourceRef[]
  first_seen: string
}

export interface PrivateEventQuery extends EventQuery {
  fit?: FitFilter
  q?: string
}

export interface Stats {
  totals: {
    people: number
    organisations: number
    profiles: number
    events_upcoming: number
    events_kept_upcoming: number
    sources: Record<SourceStatus, number>
  }
  last_7_days: { people: number; events: number; organisations: number; sources: number }
  weekly: { week_start: string; people: number; events: number; sources: number }[]
  cities: { city: string; events: number }[]
  last_run: Run | null
}

export interface Profile {
  id: number
  platform: ProfilePlatform
  url: string
  review: Review
}

export interface NextAppearance {
  event_id: number
  title: string
  starts_at: string
  city: string
  role: string
}

export interface Person {
  id: number
  name: string
  known_as: string
  headline: string
  city: string
  fit: PersonFit
  first_seen: string
  appearances: number
  next_appearance: NextAppearance | null
  profiles: Profile[]
  // For a founder: how their startup looked at its last lookup, the most
  // alive one when there are several. Null when nobody looked.
  activity: { state: Activity; since: string | null; note: string; company: string } | null
}

export interface Appearance {
  role: string
  // The passage from the event page that puts the person on stage, when
  // the language model found them. Empty when the rules found them.
  evidence: string
  event: { id: number; title: string; starts_at: string; city: string; venue: string; url: string }
}

export interface PersonDetail extends Omit<Person, 'appearances'> {
  // The list gives a count. The detail replaces it with the list itself.
  appearances: Appearance[]
  notes: string
  // Where an event page says the person founded or runs something.
  fit_evidence: string
  affiliations: { organisation: string; role: string; current: boolean }[]
  sightings: { source_id: number; source: string; checked_at: string }[]
}

export interface PeopleQuery {
  q?: string
  sort?: PeopleSort
  filter?: PeopleFilter
}

export interface LastCheck {
  events_found: number
  // For a portfolio: the startups its page lists.
  startups_found: number
  http_status: number
  mode: string
  error: string
}

export interface Source {
  id: number
  name: string
  kind: SourceKind
  url: string | null
  query: string | null
  category: string
  city: string
  status: SourceStatus
  fetch_mode: FetchMode
  notes: string
  last_checked_at: string | null
  next_check_at: string | null
  checks: number
  empty_checks_in_row: number
  points: number
  last_check: LastCheck | null
  health: Health
  health_note: string
  discovered_from: string
}

// portfolio switches a source between a page of startups and a page of events.
export type SourcePatch = Partial<Pick<Source, 'status' | 'fetch_mode' | 'notes' | 'name'>> & { portfolio?: boolean }

export interface RunProgress {
  checks: number
  checks_done: number
  reads: number
  reads_done: number
  // The reads the run should end with: those queued, and as many more as
  // checks brought lately for the checks still to come.
  reads_expected: number
  // Lookups of startups from portfolios, and the ones the run should end
  // with, counting those its portfolio checks still to come will bring.
  lookups: number
  lookups_done: number
  lookups_expected: number
  // What runs at this moment: a check of a source, a read of an event page
  // or a lookup of a startup.
  now: { kind: 'check' | 'read' | 'lookup'; label: string; since: string }[]
  // Measured from how long earlier checks and reads took. Null until there
  // is anything to measure against.
  seconds_left: number | null
}

export interface Run {
  id: number
  // nightly, manual for a run started by hand, check for Check now
  kind: 'nightly' | 'manual' | 'check'
  // Event pages the language model read in this run.
  pages_read: number
  // Startups whose imprint this run looked up.
  startups_looked_up: number
  // How far a going run is. Null once it has ended.
  progress: RunProgress | null
  started_at: string
  finished_at: string | null
  sources_checked: number
  events_found: number
  events_new: number
  people_new: number
  errors: number
}

export interface Check {
  id: number
  source: SourceRef
  mode: string
  http_status: number
  events_found: number
  events_kept: number
  people_found: number
  error: string
  duration_ms: number
  checked_at: string
  fetch_id: number | null
}

export type Json = null | boolean | number | string | Json[] | { [key: string]: Json }

export interface Setting {
  key: string
  value: Json
  description: string
  updated_at: string
}

// Transport

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

let onUnauthorized: (() => void) | null = null

// The app registers this to send people to the login on any 401.
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

function qs(params: object): string {
  const s = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') s.set(k, String(v))
  }
  const out = s.toString()
  return out ? `?${out}` : ''
}

async function request<T>(method: string, path: string, body?: unknown, opts: { quiet401?: boolean } = {}): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: body === undefined ? { Accept: 'application/json' } : { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError(0, 'Could not reach the server. Check your connection.')
  }
  if (res.status === 401 && !opts.quiet401) onUnauthorized?.()
  if (!res.ok) {
    let message = `Request failed with status ${res.status}`
    try {
      const data = await res.json()
      if (data && typeof data.error === 'string') message = data.error
    } catch {
      // Not JSON. Keep the generic message.
    }
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

export const api = {
  // Public
  publicConfig: () => request<PublicConfig>('GET', '/api/public/config', undefined, { quiet401: true }),
  publicEvents: (q: EventQuery) =>
    request<{ events: PublicEvent[] }>('GET', `/api/public/events${qs(q)}`, undefined, { quiet401: true }).then((r) => r.events),

  // Login
  login: (password: string) => request<void>('POST', '/api/login', { password }, { quiet401: true }),
  logout: () => request<void>('POST', '/api/logout', undefined, { quiet401: true }),
  me: () => request<{ logged_in: boolean }>('GET', '/api/me', undefined, { quiet401: true }),

  // Private
  stats: () => request<Stats>('GET', '/api/stats'),
  events: (q: PrivateEventQuery) => request<{ events: PrivateEvent[] }>('GET', `/api/events${qs(q)}`).then((r) => r.events),
  patchEvent: (id: number, patch: { fit: Fit; fit_reason?: string }) => request<PrivateEvent>('PATCH', `/api/events/${id}`, patch),
  people: (q: PeopleQuery) => request<{ people: Person[]; counts: Record<PeopleFilter, number> }>('GET', `/api/people${qs(q)}`),
  person: (id: number) => request<PersonDetail>('GET', `/api/people/${id}`),
  patchPerson: (id: number, patch: { notes: string }) => request<PersonDetail>('PATCH', `/api/people/${id}`, patch),
  patchProfile: (id: number, review: Review) => request<void>('PATCH', `/api/profiles/${id}`, { review }),
  sources: (q: { status?: string; q?: string } = {}) => request<{ sources: Source[] }>('GET', `/api/sources${qs(q)}`).then((r) => r.sources),
  addSource: (url: string, name?: string, portfolio = false) =>
    request<Source>('POST', '/api/sources', { url, ...(name ? { name } : {}), ...(portfolio ? { portfolio } : {}) }),
  patchSource: (id: number, patch: SourcePatch) => request<Source>('PATCH', `/api/sources/${id}`, patch),
  checkSource: (id: number) => request<void>('POST', `/api/sources/${id}/check`),
  runs: () => request<{ runs: Run[] }>('GET', '/api/runs').then((r) => r.runs),
  startRun: () => request<{ run_id: number; started: boolean }>('POST', '/api/runs', {}),
  currentRun: () => request<{ run: Run | null }>('GET', '/api/runs/current').then((r) => r.run),
  stopRun: (id: number) => request<void>('POST', `/api/runs/${id}/stop`, {}),
  run: (id: number) => request<{ run: Run; checks: Check[] }>('GET', `/api/runs/${id}`),
  settings: () => request<{ settings: Setting[] }>('GET', '/api/settings').then((r) => r.settings),
  patchSetting: (key: string, value: Json) => request<Setting>('PATCH', `/api/settings/${encodeURIComponent(key)}`, { value }),
}

export function fetchUrl(fetchId: number, kind: 'text' | 'html' | 'screenshot'): string {
  return `/api/fetches/${fetchId}/${kind}`
}
