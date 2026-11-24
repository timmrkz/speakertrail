// Typed client for the Speaker Trail HTTP API. See docs/api.md.
// All times are RFC 3339 strings in UTC.
import { Events } from '@wailsio/runtime'

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
export type PeopleSort = 'fit' | 'young' | 'new' | 'next' | 'name'
export type PeopleFilter = 'all' | 'new' | 'connected' | 'rejected' | 'fits' | 'upcoming' | 'profile' | 'founder'

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

// Where macOS put the title bar and its buttons, in whole CSS pixels, as
// cmd/speakertrail-app/chrome.go says. All zeros means the system draws
// its own title bar, which is every other system, and then the
// stylesheet keeps its own numbers.
export interface Chrome {
  // The title bar's height, which is the height of the bar the app draws.
  bar: number
  // The left edge of the first of those buttons and the right edge of the
  // last, so the name of what is on screen starts after them.
  left: number
  right: number
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
  // What became of the pull request in gone: merged, closed, or empty
  // when the channel list does not say.
  ended: '' | 'merged' | 'closed'
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
  // The fit from 0 to 100, as of now. fit_parts on the person say why.
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
  // Where the profile of someone added from a link stands: being read now
  // since read_since, a read taking about read_ms, waiting its turn in the
  // run, read by the next run, not in Exa's index yet, or empty once
  // read. See docs/api.md.
  read: '' | 'reading' | 'waiting' | 'next' | 'not_indexed' | 'failed'
  read_since: string | null
  read_ms: number | null
  read_error?: string
}

// What a search that holds links would add: profiles new to the app, the
// people here already by id, and other links, like a company page. act
// is what Run turns into, as the app decides: Add, Open the one person
// here, or a greyed Add for links that add nobody.
// The answer to a paste: the person now, and whether the page gave
// their name.
export interface PastedProfile {
  person: PersonDetail
  named: boolean
}

// What Undo or Redo in the Edit menu did: whether there was anything to
// take, and what it changed, see docs/api.md, Undo.
export interface Undone {
  done: boolean
  shows?: { kind: 'person'; id: number }
}

export interface LinksFound {
  new: number
  here: number[]
  other: number
  act: 'add' | 'open' | 'none'
}

// What came of adding links. wait says why the profiles wait for the
// next run, like a missing Exa key.
export interface Added {
  run: number
  new: number[]
  here: number[]
  // Those here already whom adding marked connected, for Undo.
  connected: number[]
  other: number
  // The one person here the links lead to, to open, when they add
  // nobody new.
  open?: number
  wait?: string
}

export interface LinkedInFacts {
  // The profile the LinkedIn button shows and opens.
  url: string
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
  // The first message to them, as the language model wrote it at
  // drafted_at and as it was changed since, empty before the first.
  draft: string
  drafted_at: string | null
  // When Exa's index gave the profile text a draft is written from, null
  // while the app has none and a draft reads it first.
  profile_read_at: string | null
  // What fit_score adds up, in the order Why shows them.
  fit_parts: FitPart[]
}

// One part of the fit, with the points it gave and why: who, younger,
// active or near.
export interface FitPart {
  key: string
  label: string
  points: number
  why: string
  // What the part shows under it, each line with a tick, a cross or
  // neither: the signals of Who they are, or the why of the others.
  lines: FitLine[]
}

export interface FitLine {
  mark: 'for' | 'against' | 'neither'
  text: string
  passage: string
  found_in: string
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
  // manual for a run started by hand, add for one that reads the profiles
  // of links Tim added, check for Check now
  kind: 'nightly' | 'manual' | 'add' | 'check'
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
  // The app needs a choice in Settings first, like where LinkedIn opens.
  choose: boolean
  constructor(status: number, message: string, choose = false) {
    super(message)
    this.status = status
    this.choose = choose
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

// Every request to the Go side. One that fails, or takes longer than a
// person waits without noticing, goes to the app's log, and so do the
// requests of a second with more of them than the app makes on its own,
// which is what a loop gone wrong looks like. A request is named by its
// method and path, never its query, which can hold what was typed in a
// search. Ported from FrameFairy's lib/api.ts, see lib/said.ts.
// The speech models in Settings, see docs/api.md, Speech models.
export interface SpeechModel {
  name: string
  title: string
  about: string
  maker: string
  languages: string
  // The archive's size and what it takes once unpacked, in bytes.
  download: number
  unpacked: number
  url: string
  recommended: boolean
  installed: boolean
  // The one Dictate hears with, as the app says.
  inUse: boolean
}

// How far a download has come: what it does now, the share done from 0
// to 1 and the seconds left, -1 where it cannot say.
export interface ModelProgress {
  text: string
  fraction: number
  remaining: number
}

// A download of a model, a speech model or a language model, in the
// shape of FrameFairy's job.
export interface ModelJob {
  id: string
  kind: 'model'
  title: string
  state: 'queued' | 'running' | 'done' | 'failed' | 'cancelled'
  progress?: ModelProgress
  error?: string
}

export interface SpeechState {
  models: SpeechModel[]
  // The models folder, with the home folder as ~.
  folder: string
  job?: ModelJob
}

// The language models in Settings, the way the speech models are, see
// docs/api.md, Language models.
export interface LanguageModel {
  name: string
  title: string
  maker: string
  about: string
  // The file's size, and the memory it takes once loaded, in bytes.
  download: number
  needs: number
  url: string
  recommended: boolean
  installed: boolean
  // The one that drafts, as the app says.
  inUse: boolean
  // What this Mac can do with it.
  fit: 'fits' | 'tight' | 'too big' | 'unknown'
  // Taken from FrameFairy's folder, one file on disk for both apps.
  shared: boolean
  // In memory now, ready to draft at once, being loaded ahead now, and
  // why the last load ahead failed.
  loaded: boolean
  loading: boolean
  notLoaded?: string
}

// Whether a draft can be written now, and what stands in the way when it
// cannot, as the app judges it: ok, warn when a download would do, err
// when this Mac or this build cannot.
export interface LanguageStanding {
  mark: 'ok' | 'warn' | 'err'
  why: string
}

export interface LanguageState {
  standing: LanguageStanding
  models: LanguageModel[]
  folder: string
  // Whether llama-server is in the app, to run a model with.
  server: boolean
  job?: ModelJob
}

// A first message being drafted for a person, see docs/api.md, Drafts.
// It comes as the event draft each time it moves on.
export interface DraftJob {
  id: string
  person_id: number
  state: 'queued' | 'reading' | 'loading' | 'writing' | 'done' | 'failed' | 'cancelled'
  model?: string
  // What the model wrote so far. Untrusted, and shown as text.
  text?: string
  started_at: string
  // How far it is from 0 to 1, and about how long is left, from how long
  // earlier drafts took. -1 and null before there was one.
  fraction: number
  seconds_left: number | null
  error?: string
  // Why a draft that is done lacks the model's part, with … in its place.
  problem?: string
  // Higher for every change than for the one before it, of any draft, so
  // an answer that arrives after a newer event is told apart from it.
  seq: number
}

// The template a first message is put together from, see docs/api.md,
// First message template.
export interface TemplatePart {
  name: string
  variations: string[]
  // Why the part cannot be taken out, from the app. Missing when it can.
  kept?: string
}
export interface TemplateField {
  name: string
  by: 'app' | 'model'
  about: string
}
export interface FirstMessageTemplate {
  parts: TemplatePart[]
  fields: TemplateField[]
  changed: boolean
}

export function onDraft(fn: (j: DraftJob) => void): () => void {
  return Events.On('draft', (ev) => fn(ev.data as DraftJob))
}

// What is being written down while somebody dictates: every piece settled
// so far, in order, and what follows them, still being heard. See
// listen.go and docs/api.md, Dictate.
export interface Dictation {
  recording: number
  said: string[]
  hearing: string
  error?: string
  // Why the recording stopped by itself, with said all of it.
  ended?: string
  // Why the microphone hears nothing, while it goes on recording.
  unheard?: string
  // How many settleListening and breakListening calls said holds.
  asked: number
}

// How loud the microphone is while it records, from 0 for a quiet room to
// 1 for a raised voice, ten times a second. See listen.go.
export interface Loudness {
  recording: number
  level: number
}

// A picture attached to a report, which the window shows from
// /api/report/picture?id=.
export interface ReportPicture {
  id: string
  name: string
  width: number
  height: number
  // Removed by the person and kept to be put back. Never true in a draft
  // the window is sent, which leaves removed pictures out.
  removed?: boolean
}

// A report before it is made. error says why pictures just given were
// left out.
export interface ReportDraft {
  text: string
  pictures: ReportPicture[]
  // Whether the report takes People along, without names or links.
  people: boolean
  error?: string
}

export function onLoudness(fn: (n: Loudness) => void): () => void {
  return Events.On('loudness', (ev) => fn(ev.data as Loudness))
}

export function onDictation(fn: (d: Dictation) => void): () => void {
  return Events.On('dictation', (ev) => fn(ev.data as Dictation))
}

// The report window opened or closed, for Bugs in the sidebar.
export function onReportWindow(fn: (open: boolean) => void): () => void {
  return Events.On('report-window', (ev) => fn(Boolean(ev.data)))
}

// What became of files dropped on the report window.
export function onReportDraft(fn: (d: ReportDraft) => void): () => void {
  return Events.On('report-draft', (ev) => fn(ev.data as ReportDraft))
}

// Undo or Redo in the Edit menu, see menu.go. The menu takes Cmd+Z
// before the page sees it. The event goes to every window, and only the
// one Tim is in takes it: Cmd+Z typing in Report a Problem must not take
// back a change in the app's window behind it. Ported from FrameFairy.
export function onUndo(fn: (how: 'undo' | 'redo') => void): () => void {
  return Events.On('undo', (ev) => {
    if (document.hasFocus()) fn(ev.data === 'redo' ? 'redo' : 'undo')
  })
}

export function pictureUrl(id: string): string {
  return `/api/report/picture?id=${encodeURIComponent(id)}`
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const call = `${method} ${path.split('?')[0]}`
  const from = performance.now()
  counted(call)
  try {
    const v = await send<T>(method, path, body)
    const ms = performance.now() - from
    if (ms > SLOW) tell(`calls: ${call} took ${Math.round(ms)} ms`)
    return v
  } catch (e) {
    tell(`calls: ${call} failed after ${Math.round(performance.now() - from)} ms: ${e instanceof Error ? e.message : String(e)}`)
    throw e
  }
}

// What the log is told through, set by lib/said.ts, which needs this file
// and so cannot be imported here.
let tell: (line: string) => void = () => {}
export function tellTo(fn: (line: string) => void) {
  tell = fn
}

// A request slower than this is said, in ms. Low enough to catch what a
// person feels as the app taking a moment, see SLOWLY in said.ts.
const SLOW = 150
// More requests than this in a second are said, with which they were.
const MANY = 60
let second = 0
let calls = new Map<string, number>()
function counted(call: string) {
  const now = Math.floor(performance.now() / 1000)
  if (now !== second) {
    const n = [...calls.values()].reduce((a, b) => a + b, 0)
    if (n > MANY) {
      const most = [...calls].sort((a, b) => b[1] - a[1]).slice(0, 5)
      tell(`calls: ${n} in one second, ${most.map(([m, k]) => `${m} ${k}`).join(', ')}`)
    }
    second = now
    calls = new Map()
  }
  calls.set(call, (calls.get(call) ?? 0) + 1)
}

async function send<T>(method: string, path: string, body?: unknown): Promise<T> {
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
    let choose = false
    try {
      const data = await res.json()
      if (data && typeof data.error === 'string') message = data.error
      choose = data?.choose === true
    } catch {
      // Not JSON. Keep the generic message.
    }
    throw new ApiError(res.status, message, choose)
  }
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

export interface BrowserProfile {
  folder: string
  name: string
}

export interface LinkedinWhere {
  app: string
  folder: string
  // Whether the chosen profile is still there.
  known: boolean
  browsers: { app: string; profiles: BrowserProfile[] }[]
}

// A line the window says for the app's log, see lib/said.ts and
// cmd/speakertrail-app/applog.go.
export interface SaidLine {
  at: number
  level?: 'error' | 'warn' | 'info' | 'debug' | 'trace'
  act?: string
  toast?: boolean
  text: string
}

// The app's log as Settings shows it, as LogStatus in
// cmd/speakertrail-app/applog.go: the file in use, how many files there
// are with the older ones, their bytes and lines, and the last toasts
// about something that went wrong, newest first.
export interface LogStatus {
  path: string
  files: number
  bytes: number
  lines: number
  toasts: { at: number; text: string }[]
}

// The log's own requests are sent as they are, never timed or said: a
// line about the request that carries lines would be one more line to
// carry, without end.
const quietly = (path: string, body: unknown) =>
  send<void>('POST', path, body)

export const api = {
  said: (lines: SaidLine[]) => quietly('/api/log/said', lines),
  acted: (act: { id: string; at: number; what: string }) => quietly('/api/log/acted', act),
  appLog: () => request<LogStatus>('GET', '/api/log'),
  // Whether Settings is open, so the log is sent to it as lines are
  // written, see onAppLog in lib/applog.svelte.ts.
  watchAppLog: (on: boolean) => request<void>('POST', '/api/log/watch', { on }),
  showAppLog: () => request<void>('POST', '/api/log/show', {}),
  // Every file of the log goes, after Settings asked. It gives the log as
  // it is then: the one line that says so.
  removeAppLog: () => request<LogStatus>('DELETE', '/api/log'),
  people: (q: PeopleQuery) => request<{ people: Person[]; counts: Record<PeopleFilter, number>; links: LinksFound | null }>('GET', `/api/people${qs(q)}`),
  addPeople: (text: string) => request<Added>('POST', '/api/people', { text }),
  // Takes out again someone added from a link, for Undo.
  removePerson: (id: number) => request<void>('DELETE', `/api/people/${id}`),
  person: (id: number) => request<PersonDetail>('GET', `/api/people/${id}`),
  // Keeps a LinkedIn page Tim copied in Chrome as a person's profile.
  pasteProfile: (id: number, text: string) => request<PastedProfile>('PUT', `/api/people/${id}/profile`, { text }),
  // Undo and Redo in the Edit menu, for every change Tim makes.
  undo: () => request<Undone>('POST', '/api/undo', {}),
  redo: () => request<Undone>('POST', '/api/redo', {}),
  patchPerson: (id: number, patch: { notes?: string; connected?: boolean; draft?: string }) => request<PersonDetail>('PATCH', `/api/people/${id}`, patch),
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
  // Where LinkedIn opens: the browser and its profile, and what this Mac has.
  linkedin: () => request<LinkedinWhere>('GET', '/api/linkedin'),
  setLinkedin: (app: string, folder: string) => request<LinkedinWhere>('PUT', '/api/linkedin', { app, folder }),
  // Opens a profile link in the chosen browser profile.
  openLinkedin: (id: number) => request<void>('POST', `/api/people/${id}/linkedin`, {}),
  chrome: (window: 'app' | 'report' = 'app') => request<Chrome>('GET', window === 'report' ? '/api/chrome?window=report' : '/api/chrome'),
  // Report a Problem, in a window of its own, see docs/api.md. The report
  // is made on the Desktop and its path, with the home folder as ~, comes
  // back.
  reportProblem: (what: string) => request<{ path: string }>('POST', '/api/report/send', { what }).then((r) => r.path),
  // Whether the report takes People along.
  takePeople: (people: boolean) => request<ReportDraft>('PUT', '/api/report/people', { people }),
  // Opens the report window, or brings it forward, and says how it was
  // asked for: "bugs" from the sidebar.
  openReport: (how: string) => request<void>('POST', '/api/report/open', { how }),
  // The Mac's clipboard, which the app writes, since the web view refuses
  // the window's own. See docs/api.md, Copy.
  copy: (text: string) => request<void>('POST', '/api/clipboard', { text }),
  // Opens it with a toast's words, as a report of its own.
  reportAbout: (text: string) => request<void>('POST', '/api/report/about', { text }),
  reportWindowOpen: () => request<{ open: boolean }>('GET', '/api/report/window').then((r) => r.open),
  // The draft the Go side keeps until Report: what was written and the
  // pictures, so closing the window loses nothing.
  reportDraft: () => request<ReportDraft>('GET', '/api/report'),
  saveReportText: (text: string) => request<void>('PUT', '/api/report/text', { text }),
  // Attach asks in the open box, a paste hands the picture over in
  // base64. Each gives the draft, with why something was left out.
  attachPictures: () => request<ReportDraft>('POST', '/api/report/pick', {}),
  attachPasted: (name: string, data: string) => request<ReportDraft>('POST', '/api/report/pasted', { name, data }),
  removePicture: (id: string) => request<ReportDraft>('POST', '/api/report/remove', { id }),
  putBackPicture: (id: string) => request<ReportDraft>('POST', '/api/report/put-back', { id }),
  clearReport: () => request<ReportDraft>('POST', '/api/report/clear', {}),
  // Dictate: listen gives the recording's number once the microphone is
  // open, what is written down comes as the event dictation, and stopping
  // gives every piece of it. Cancelling drops it.
  listen: () => request<{ recording: number }>('POST', '/api/listen', {}).then((r) => r.recording),
  stopListening: () => request<Dictation>('POST', '/api/listen/stop', {}),
  cancelListening: () => request<void>('POST', '/api/listen/cancel', {}),
  // The window took the first pieces of the recording in, the last of
  // them as text, the way the person left it.
  settleListening: (recording: number, pieces: number, text: string) => request<void>('POST', '/api/listen/settle', { recording, pieces, text }),
  // The window took the first pieces in and moved on, a new line.
  breakListening: (recording: number, pieces: number) => request<void>('POST', '/api/listen/break', { recording, pieces }),
  // The speech models, fetched and removed in Settings. Where a download
  // stands comes as the event speech.
  speech: () => request<SpeechState>('GET', '/api/speech'),
  installSpeechModel: (name: string) => request<ModelJob>('POST', '/api/speech/install', { name }),
  cancelSpeechDownload: (id: string) => request<void>('POST', '/api/speech/cancel', { id }),
  removeSpeechModel: (name: string) => request<void>('DELETE', `/api/speech/models/${encodeURIComponent(name)}`, {}),
  // The language models, the same as the speech models. Where a download
  // stands comes as the event language.
  language: () => request<LanguageState>('GET', '/api/language'),
  installLanguageModel: (name: string) => request<ModelJob>('POST', '/api/language/install', { name }),
  // Picks the model that drafts, downloaded or not, and answers how they
  // all stand then.
  useLanguageModel: (name: string) => request<LanguageState>('POST', '/api/language/use', { name }),
  cancelLanguageDownload: (id: string) => request<void>('POST', '/api/language/cancel', { id }),
  removeLanguageModel: (name: string) => request<void>('DELETE', `/api/language/models/${encodeURIComponent(name)}`, {}),
  // First messages: the drafts under way and the latest of each person,
  // one started for a person, and stopped again.
  drafts: () => request<DraftJob[]>('GET', '/api/drafts'),
  template: () => request<FirstMessageTemplate>('GET', '/api/template'),
  // Gives a part its variations, or adds it at a place.
  putTemplatePart: (name: string, variations: string[], at?: number) =>
    request<FirstMessageTemplate>('PUT', `/api/template/parts/${encodeURIComponent(name)}`, { variations, at }),
  deleteTemplatePart: (name: string) => request<FirstMessageTemplate>('DELETE', `/api/template/parts/${encodeURIComponent(name)}`, {}),
  // Puts Tim's first template back, and putTemplate the one before.
  resetTemplate: () => request<FirstMessageTemplate>('DELETE', '/api/template', {}),
  putTemplate: (parts: TemplatePart[]) => request<FirstMessageTemplate>('PUT', '/api/template', { parts }),
  // The same seed makes the same picks.
  templateExample: (seed: number) => request<{ text: string }>('GET', `/api/template/example?seed=${seed}`).then((r) => r.text),
  startDraft: (id: number) => request<DraftJob>('POST', `/api/people/${id}/draft`, {}),
  cancelDraft: (id: number) => request<void>('POST', `/api/people/${id}/draft/cancel`, {}),
  updates: () => request<UpdateState>('GET', '/api/updates'),
  checkForUpdates: () => request<void>('POST', '/api/updates/check', {}),
  followChannel: (channel: string) => request<UpdateState>('POST', '/api/updates/follow', { channel }),
  relaunch: () => request<void>('POST', '/api/updates/relaunch', {}),
  openCommit: () => request<void>('POST', '/api/updates/commit', {}),
  // The question Cmd+Q asked taken away, with Escape or a click, so the
  // next Cmd+Q asks again rather than quitting.
  stayOpen: () => request<void>('POST', '/api/quit/stay', {}),
}

// Cmd+Q heard by the Go side: "ask" for the first press, which only asks,
// with how long the question stays, as long as the second press may come,
// and "going" once the app is on its way out. See cmd/speakertrail-app/quit.go.
export interface Quitting {
  what: 'ask' | 'going'
  ms?: number
}

export function onQuit(fn: (q: Quitting) => void): () => void {
  return Events.On('quit', (ev) => fn(ev.data as Quitting))
}

