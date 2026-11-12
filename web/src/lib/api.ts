// Typed client for the Speaker Trail HTTP API. See docs/api.md.
// All times are RFC 3339 strings in UTC.

export type EventType = 'pitch' | 'talk' | 'panel' | 'meetup' | 'workshop' | 'conference' | 'sport' | 'other'
export type EventFormat = 'in_person' | 'online' | 'hybrid'
export type Fit = 'kept' | 'dropped'
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
  | 'organiser_page' | 'profile_page' | 'newsletter' | 'search_query' | 'post_search' | 'profile_search' | 'portfolio' | 'directory'
// What a source's page lists: events, startups for a portfolio, or
// businesses run by people for a directory.
export type SourceLists = 'events' | 'startups' | 'businesses'
export type FetchMode = 'auto' | 'http' | 'browser'
export type Health = 'ok' | 'warning' | 'error' | 'never'
export type PeopleSort = 'young' | 'new' | 'fit' | 'next' | 'name'
export type PeopleFilter = 'all' | 'young' | 'connected' | 'fits' | 'upcoming' | 'profile' | 'founder'

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

// Where the app's updates stand, as cmd/speakertrail-app/updates.go says
// it. See docs/UPDATES.md.
export interface UpdateChannel {
  // main, or pr- and the pull request's number.
  id: string
  name: string
  version: string
  // A pull request that no longer merges into main. Its build is still
  // there to try.
  conflict: boolean
}

export type UpdatePhase = '' | 'checking' | 'current' | 'gone' | 'downloading' | 'ready' | 'failed'

export interface UpdateState {
  version: string
  commit: string
  // Where the running build came from, empty for one made on this Mac.
  channel: string
  // Why this build does not update itself, empty when it does.
  off: string
  channels: UpdateChannel[] | null
  picked: string
  follows: string
  gone: string
  building: string
  phase: UpdatePhase
  next: string
  nextName: string
  nextCommit: string
  checked: string
  written: number
  total: number
  problem: string
  detail: string
}

export interface SearchUse {
  provider: string
  used: number
  budget: number
  set: boolean
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

// One reason for or against a fit, by the fit rubric, with the passage
// that shows it. found_in: title, event page, event, imprint, lookup,
// portfolio, about page, model, LinkedIn post or LinkedIn profile.
export interface FitSignal {
  key: string
  label: string
  for: boolean
  passage: string
  found_in: string
}

export interface Person {
  id: number
  name: string
  known_as: string
  headline: string
  city: string
  fit: PersonFit
  first_seen: string
  // The signals for less those against. The signals are the reasons.
  fit_score: number
  signals: FitSignal[]
  // Whether Tim connected with the person on LinkedIn.
  connected: boolean
  appearances: number
  next_appearance: NextAppearance | null
  profiles: Profile[]
  // For a founder: how their startup looked at its last lookup, the most
  // alive one when there are several. Null when nobody looked.
  activity: { state: Activity; since: string | null; note: string; company: string } | null
  // What the confirmed LinkedIn profile says. Null without one.
  linkedin: LinkedInFacts | null
}

export interface LinkedInFacts {
  // The likely year of birth, and the step on the profile that shows it.
  born_about: number | null
  age_reason: string
  // The roles held now, like "Gründerin, Studio Beispiel".
  roles: string
  // The newest post, comment or reaction.
  last_active: string | null
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
  // Each business the person runs or belongs to, with its website and its
  // imprint when a lookup found them. The imprint gives an email address,
  // by law. The app stores the link, never the address.
  affiliations: { organisation: string; role: string; current: boolean; website: string; imprint_url: string }[]
  sightings: { source_id: number; source: string; checked_at: string }[]
}

export interface PeopleQuery {
  q?: string
  sort?: PeopleSort
  filter?: PeopleFilter
}

export interface LastCheck {
  events_found: number
  // For a portfolio or a directory: the startups or businesses its page
  // lists, in NRW for a directory.
  startups_found: number
  // For a post search: the people in NRW who wrote the posts it found.
  // For a profile search: the people it kept.
  people_found: number
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
  // Everyone the source ever found, and how many of them Tim connected
  // with.
  people: number
  people_connected: number
}

// An exclusion is a word or phrase, like "Online Marketing". Profile
// searches leave out anyone whose headline or roles contain it, and People
// hides those found before, unless Tim connected with them. people is how
// many it hides now.
export interface Exclusion {
  id: number
  phrase: string
  people: number
}

// lists switches what a source's page is read as.
export type SourcePatch = Partial<Pick<Source, 'status' | 'fetch_mode' | 'notes' | 'name'>> & { lists?: SourceLists }

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

// A run's true state. Interrupted: the app stopped under it, and it ended
// when the app started again.
export type RunState = 'going' | 'finished' | 'stopped' | 'interrupted'

export interface Run {
  id: number
  // nightly, manual for a run started by hand, check for Check now
  kind: 'nightly' | 'manual' | 'check'
  state: RunState
  // Event pages the language model read in this run, and how many of
  // those reads failed.
  pages_read: number
  reads_failed: number
  // Startups whose imprint this run looked up, and the lookups that failed.
  startups_looked_up: number
  lookups_failed: number
  // New people who fit, like founders.
  fits_new: number
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

// What failed in a run, in plain words. One per source for checks, one per
// reason for reads and lookups. action is the one thing that fixes it.
export interface RunFailure {
  what: 'check' | 'read' | 'lookup'
  source: (SourceRef & { status: SourceStatus }) | null
  reason: string
  action: '' | 'retire' | 'check'
  count: number
  detail: string
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

function qs(params: object): string {
  const s = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null && v !== '') s.set(k, String(v))
  }
  const out = s.toString()
  return out ? `?${out}` : ''
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      method,
      credentials: 'same-origin',
      headers: body === undefined ? { Accept: 'application/json' } : { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    throw new ApiError(0, 'Could not reach the app. Try again.')
  }
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
  people: (q: PeopleQuery) => request<{ people: Person[]; counts: Record<PeopleFilter, number> }>('GET', `/api/people${qs(q)}`),
  person: (id: number) => request<PersonDetail>('GET', `/api/people/${id}`),
  patchPerson: (id: number, patch: { notes?: string; connected?: boolean }) => request<PersonDetail>('PATCH', `/api/people/${id}`, patch),
  patchProfile: (id: number, review: Review) => request<void>('PATCH', `/api/profiles/${id}`, { review }),
  sources: (q: { status?: string; q?: string; kind?: SourceKind } = {}) => request<{ sources: Source[] }>('GET', `/api/sources${qs(q)}`).then((r) => r.sources),
  addProfileSearch: (query: string) => request<Source>('POST', '/api/sources', { query, kind: 'profile_search' }),
  patchSource: (id: number, patch: SourcePatch) => request<Source>('PATCH', `/api/sources/${id}`, patch),
  checkSource: (id: number) => request<void>('POST', `/api/sources/${id}/check`),
  exclusions: () => request<Exclusion[]>('GET', '/api/exclusions'),
  addExclusion: (phrase: string) => request<Exclusion>('POST', '/api/exclusions', { phrase }),
  deleteExclusion: (id: number) => request<void>('DELETE', `/api/exclusions/${id}`, {}),
  startRun: () => request<{ run_id: number; started: boolean }>('POST', '/api/runs', {}),
  currentRun: () => request<{ run: Run | null }>('GET', '/api/runs/current').then((r) => r.run),
  stopRun: (id: number) => request<void>('POST', `/api/runs/${id}/stop`, {}),
  run: (id: number) => request<{ run: Run; checks: Check[]; failures: RunFailure[] }>('GET', `/api/runs/${id}`),
  settings: () => request<{ settings: Setting[] }>('GET', '/api/settings').then((r) => r.settings),
  // Whether the Exa key is saved, and this month's searches. Never the key.
  keys: () => request<{ keys: SearchUse[] }>('GET', '/api/keys').then((r) => r.keys),
  // Saves the Exa key in the keychain. An empty key removes it.
  setKey: (key: string) => request<void>('PUT', '/api/keys/exa', { key }),
  patchSetting: (key: string, value: Json) => request<Setting>('PATCH', `/api/settings/${encodeURIComponent(key)}`, { value }),
  updates: () => request<UpdateState>('GET', '/api/updates'),
  checkForUpdates: () => request<void>('POST', '/api/updates/check', {}),
  followChannel: (channel: string) => request<UpdateState>('POST', '/api/updates/follow', { channel }),
  relaunch: () => request<void>('POST', '/api/updates/relaunch', {}),
  openCommit: () => request<void>('POST', '/api/updates/commit', {}),
}

export function fetchUrl(fetchId: number, kind: 'text' | 'html' | 'screenshot'): string {
  return `/api/fetches/${fetchId}/${kind}`
}
