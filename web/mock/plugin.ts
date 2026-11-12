// Dev-only mock of the Speaker Trail API (docs/api.md).
// Used by `MOCK=1 npm run dev`. Never imported by the app, never built.
// State lives in memory and resets when the dev server restarts.
// The mock password is "speakertrail".

import type { Plugin } from 'vite'
import type {
  Activity, Appearance, FitSignal, Json, Person, PersonDetail, PrivateEvent, Profile,
  Run, RunFailure, RunProgress, Setting, Source, SourceStatus, UpdateChannel, UpdateState,
} from '../src/lib/api.ts'
import {
  addDays, berlin, buildRuns, buildSettings, buildSources, EVENT_SEEDS, hoursAgo, PERSON_INFO,
  todayBerlin, type MockSource,
} from './fixtures.ts'

// Just enough of Node's request and response for this file.
interface Req {
  url?: string
  method?: string
  headers: Record<string, string | string[] | undefined>
  on(event: 'data', cb: (chunk: Uint8Array) => void): void
  on(event: 'end' | 'error', cb: () => void): void
}
interface Res {
  statusCode: number
  setHeader(name: string, value: string): void
  end(body?: string): void
}

const DELAY_MS = 280

interface PersonRow {
  id: number
  name: string
  headline: string
  city: string
  fit: Person['fit']
  first_seen: string
  notes: string
  profiles: Profile[]
  affiliations: PersonDetail['affiliations']
  connected?: boolean
  // What the confirmed LinkedIn profile says, for people a profile search
  // found.
  linkedin?: Person['linkedin']
  searchId?: number
}

function createState() {
  const today = todayBerlin()
  const sources = buildSources()
  const people = new Map<string, PersonRow>()
  let profileId = 1

  function personFor(name: string, affiliation: string, firstSeen: string): PersonRow {
    let p = people.get(name)
    if (p) return p
    const info = PERSON_INFO[name]
    p = {
      id: people.size + 1,
      name,
      headline: info?.headline ?? affiliation,
      city: info?.city ?? '',
      fit: info?.fit ?? 'other',
      first_seen: firstSeen,
      notes: info?.notes ?? '',
      profiles: (info?.profiles ?? []).map(([platform, url, review]) => ({ id: profileId++, platform, url, review })),
      affiliations: (info?.affiliations ?? (affiliation ? [[affiliation.replace(/^.*,\s*/, ''), 'member', true]] : []))
        .map(([organisation, role, current]) => ({ organisation, role, current, website: '', imprint_url: '' })),
    }
    people.set(name, p)
    return p
  }

  // Founders a lookup found in a startup's imprint, with no event yet.
  // All names are invented.
  function lookedUp(): PersonRow[] {
    return [
      ['Hanna Hellwig', 'Hellwig Robotics GmbH'], ['Jonas Probst', 'Probst Health UG (haftungsbeschränkt)'],
      ['Ida Ingwersen', 'Ingwersen Labs GmbH'], ['Ole Osterkamp', 'Osterkamp Analytics GmbH'],
    ].map(([name, company], i) => ({
      id: 900 + i, name, headline: `Managing director, ${company}`, city: 'Köln', fit: 'founder' as const,
      first_seen: hoursAgo(30 + i * 7), notes: '',
      // The first runs her business alone, so her site's Instagram is hers to confirm.
      profiles: i === 0 ? [{ id: 9000, platform: 'instagram' as const, url: 'https://www.instagram.com/example.hellwig/', review: 'open' as const }] : [],
      affiliations: [{
        organisation: company, role: 'founder', current: true,
        website: `https://${company.split(' ')[0].toLowerCase()}.example/`, imprint_url: `https://${company.split(' ')[0].toLowerCase()}.example/impressum`,
      }],
    }))
  }

  // Invented people as profile searches find them: on LinkedIn, in NRW,
  // active lately and running their own business.
  function foundByProfileSearches(): PersonRow[] {
    const rows: [string, string, string, number | null, string, number, number][] = [
      ['Lea Probe', 'Personal Trainerin | Selbstständig', 'Köln', 2000, 'Bachelor from 2019', 3, 101],
      ['Tom Muster', 'Gründer @ Probe Gym', 'Düsseldorf', 1986, 'First job from 2008', 40, 102],
      ['Sara Probe', 'Gründerin | Online-Marketing für Coaches', 'Köln', 1998, 'Bachelor from 2017', 5, 104],
      ['Mia Beispiel', 'Yogalehrerin mit eigenem Studio', 'Bonn', 1999, 'Bachelor from 2018', 9, 103],
      ['Jan Probst', 'Gen Z Unternehmer | Gründer von Probst Media', 'Essen', 2003, 'School until 2022', 1, 104],
      ['Nora Muster', 'Ernährungsberaterin, eigene Praxis', 'Dortmund', null, '', 21, 105],
      ['Emil Beispiel', 'Inhaber, Beispiel Kampfsport Akademie', 'Köln', 1994, 'Bachelor from 2013', 60, 106],
      ['Lina Probe', 'Gründerin direkt nach dem Studium · Probe Interiors', 'Düsseldorf', 1998, 'Master from 2021', 12, 107],
      ['Paul Muster', 'Life Coach | Selbstständig seit 2021', 'Bonn', 1991, 'Bachelor from 2010', 100, 101],
    ]
    return rows.map(([name, headline, city, born, reason, activeDaysAgo, searchId], i) => ({
      id: 1000 + i, name, headline, city, fit: 'founder' as const, first_seen: hoursAgo(2 + i * 9), notes: '',
      profiles: [{ id: 10000 + i, platform: 'linkedin' as const, url: `https://www.linkedin.com/in/example-${1000 + i}`, review: 'confirmed' as const }],
      affiliations: [], searchId,
      connected: i === 1,
      linkedin: { born_about: born, age_reason: reason, roles: headline.replace(' | ', ', '), last_active: hoursAgo(activeDaysAgo * 24) },
    }))
  }

  const events: PrivateEvent[] = EVENT_SEEDS.map((e, i) => {
    const day = addDays(today, e.day)
    const firstSeen = hoursAgo(Math.max(5, (Math.min(e.day, 0) + 12 - (i % 9)) * 24 + 5))
    return {
      id: i + 1,
      title: e.title,
      starts_at: berlin(day, e.time),
      ends_at: e.end ? berlin(addDays(today, e.end[0]), e.end[1]) : null,
      city: e.city,
      venue: e.venue,
      address: e.address,
      format: e.format ?? 'in_person',
      type: e.type,
      url: e.url,
      price: e.price ?? '',
      organisers: e.organisers,
      fit: e.fit ?? 'kept',
      fit_reason: e.reason ?? '',
      people: (e.people ?? []).map(([name, role, affiliation]) => ({
        id: personFor(name, affiliation, firstSeen).id, name, role, affiliation,
      })),
      sources: e.sources.map((id) => {
        const s = sources.find((x) => x.id === id)!
        return { id: s.id, name: s.name, url: s.url ?? '' }
      }),
      first_seen: firstSeen,
    }
  })

  const { runs, checks } = buildRuns(sources)
  return {
    events,
    people: [...people.values(), ...lookedUp(), ...foundByProfileSearches()],
    sources,
    runs,
    checks,
    settings: buildSettings(),
    exclusions: ['Online Marketing', 'Social Media Marketing', 'Performance Marketing', 'Personal Branding', 'LinkedIn', 'Sichtbarkeit']
      .map((phrase, i) => ({ id: i + 1, phrase })),
    nextId: { source: 200, exclusion: 100 },
  }
}

type State = ReturnType<typeof createState>

const UPDATE_CHANNELS: UpdateChannel[] = [
  { id: 'main', name: 'main', version: '0.1.0-main.4b1c0de', conflict: false },
  { id: 'pr-9', name: '#9 Speaker Trail as a Mac app, without Docker', version: '0.1.0-pr9.e808a87', conflict: false },
  { id: 'pr-12', name: '#12 A calmer layout, list and detail', version: '0.1.0-pr12.77aa310', conflict: true },
]
const upd = { running: 'pr-9', picked: '', lookFrom: Date.now(), downloadFrom: 0 }
function updateState(): UpdateState {
  const now = Date.now()
  const follows = upd.picked || upd.running
  const ch = UPDATE_CHANNELS.find((c) => c.id === follows)!
  const base: UpdateState = {
    version: '0.1.0-pr9.e808a87', commit: 'e808a87ee3a7', channel: upd.running, off: '', channels: UPDATE_CHANNELS,
    picked: upd.picked, follows, gone: '', building: '', phase: 'current', next: '', nextName: '', nextCommit: '',
    checked: new Date(upd.lookFrom + 600).toISOString(), written: 0, total: 0, problem: '', detail: '',
  }
  if (now - upd.lookFrom < 600) return { ...base, phase: 'checking' }
  if (!upd.downloadFrom) return base
  const total = 21 << 20
  const done = Math.min(1, Math.max(0, (now - upd.downloadFrom) / 6000))
  const next = { next: ch.version, nextName: ch.name, nextCommit: ch.version.slice(-7) }
  if (done < 1) return { ...base, ...next, phase: 'downloading', written: Math.floor(total * done), total }
  return { ...base, ...next, phase: 'ready' }
}

// Like the server: what failed, one per source, in plain words, with the
// action that fixes it. Reads that failed are grouped by reason.
function failuresOf(s: State, run: Run): RunFailure[] {
  const out: RunFailure[] = []
  for (const c of (s.checks.get(run.id) ?? []).filter((x) => x.error).sort((a, b) => a.source.name.localeCompare(b.source.name))) {
    const src = s.sources.find((x) => x.id === c.source.id)
    const status = (src?.status ?? 'active') as SourceStatus
    const [reason, kind] = c.http_status === 404 ? ['The page is gone', 'gone'] : c.http_status === 403 ? ['The site refuses the bot', 'blocked']
      : c.http_status === 0 ? ['The site did not answer in time', 'passing'] : [`The site answered with an error, HTTP ${c.http_status}`, 'lasting']
    const action = status === 'retired' || status === 'manual' ? '' : kind === 'passing' ? 'check' : 'retire'
    out.push({ what: 'check', source: { ...c.source, status }, reason, action, count: 1, detail: c.error })
  }
  if (run.reads_failed) {
    out.push({ what: 'read', source: null, reason: 'The language model did not answer. Is Docker Model Runner on?', action: '', count: run.reads_failed,
      detail: 'no language model answers. Is Docker Model Runner on? Turn it on with: docker desktop enable model-runner (dial tcp: connection refused)' })
  }
  if (run.lookups_failed) {
    out.push({ what: 'lookup', source: null, reason: 'The site did not answer in time', action: '', count: run.lookups_failed,
      detail: 'get https://schweigen.example/: context deadline exceeded' })
  }
  return out
}

function setting(s: State, key: string): Json | undefined {
  return s.settings.find((x) => x.key === key)?.value
}

function appearancesOf(s: State, id: number): Appearance[] {
  return s.events
    .filter((e) => e.fit === 'kept' && e.people.some((p) => p.id === id))
    .sort((a, b) => a.starts_at.localeCompare(b.starts_at))
    .map((e, i) => ({
      role: e.people.find((p) => p.id === id)!.role,
      // Every third one came from the rules, which quote nothing.
      evidence: i % 3 === 2 ? '' : `Auf der Bühne: ${e.people.find((p) => p.id === id)!.name}, mit einer Geschichte über den Anfang.`,
      event: { id: e.id, title: e.title, starts_at: e.starts_at, city: e.city, venue: e.venue, url: e.url },
    }))
}

// A small copy of the fit rubric (internal/rubric) for the invented
// people: their title, the events they host and where lookups found them.
const MOCK_RUBRIC: [RegExp, string, string, boolean][] = [
  [/coach|trainer|lehrer|teacher|yoga|kampfsport|bjj|psychologist|run club/i, 'works_with_people', 'Works with people', true],
  [/inhaber|owner|runs the|founder|gründer/i, 'owner_operator', 'Runs their own business', true],
  [/organiser|host at|runs the/i, 'runs_events', 'Runs their own events', true],
  [/autor|author/i, 'author', 'Author', true],
  [/athlete|athlet/i, 'athlete', 'Athlete', true],
  [/head of|\bvp\b|director of/i, 'corporate', 'Corporate title', false],
  [/researcher|data lead/i, 'large_company', 'Large company', false],
]

function signalsOf(p: PersonRow, apps: Appearance[]): FitSignal[] {
  const out: FitSignal[] = []
  const add = (key: string, label: string, isFor: boolean, passage: string, found_in: string) => {
    if (!out.some((x) => x.key === key)) out.push({ key, label, for: isFor, passage, found_in })
  }
  // Some invented people have a profile with dates, some a recent post.
  const year = new Date().getFullYear()
  if (p.linkedin?.born_about && year - p.linkedin.born_about < 30) add('young', 'Likely under 30', true, `${p.linkedin.age_reason}, so about ${year - p.linkedin.born_about}`, 'LinkedIn profile')
  else if (!p.linkedin && p.id % 4 === 0) add('young', 'Likely under 30', true, `Bachelor from ${2015 + (p.id % 6)}, so about ${26 - (p.id % 6)}`, 'LinkedIn profile')
  for (const [re, key, label, isFor] of MOCK_RUBRIC) if (re.test(p.headline)) add(key, label, isFor, p.headline, 'title')
  const hosted = apps.find((a) => a.role === 'host' || a.role === 'moderator')
  if (hosted) add('runs_events', 'Runs their own events', true, `Hosts "${hosted.event.title}"`, 'event')
  if (p.linkedin?.last_active) add('active_on_linkedin', 'Active on LinkedIn', true, `Last active on ${new Date(p.linkedin.last_active).toLocaleDateString('en-GB', { day: 'numeric', month: 'long', year: 'numeric' })}`, 'LinkedIn profile')
  else if (p.id % 5 === 0) add('active_on_linkedin', 'Active on LinkedIn', true, 'Last active on 14 October 2026', 'LinkedIn profile')
  if (p.id >= 900 && p.id < 1000) {
    add('owner_operator', 'Runs their own business', true, `${p.headline.replace(', ', ' of ')}, by its imprint. In the portfolio of Beispiel Hub`, 'imprint')
    add('backed', 'Backed by a startup programme', false, 'In the portfolio of Beispiel Hub', 'portfolio')
  }
  return out.sort((a, b) => Number(b.for) - Number(a.for))
}

function personList(s: State, p: PersonRow): Person {
  const apps = appearancesOf(s, p.id)
  const now = new Date().toISOString()
  const next = apps.find((a) => a.event.starts_at >= now)
  const signals = signalsOf(p, apps)
  return {
    id: p.id, name: p.name, known_as: '', headline: p.headline, city: p.city, fit: p.fit, first_seen: p.first_seen,
    fit_score: signals.reduce((n, x) => n + (x.for ? 1 : -1), 0), signals, connected: !!p.connected,
    appearances: apps.length,
    next_appearance: next
      ? { event_id: next.event.id, title: next.event.title, starts_at: next.event.starts_at, city: next.event.city, role: next.role }
      : null,
    profiles: p.profiles,
    linkedin: p.linkedin ?? null,
    // Founders of a startup come with what its lookup saw, like the server.
    activity: p.fit === 'founder' && !next && p.affiliations.some((a) => a.role === 'founder')
      ? { ...MOCK_ACTIVITY[p.id % MOCK_ACTIVITY.length], company: p.affiliations[0]?.organisation ?? 'Beispiel GmbH' }
      : null,
  }
}

const MOCK_ACTIVITY: { state: Activity; since: string | null; note: string }[] = [
  { state: 'active', since: '2026-08-14T00:00:00Z', note: 'the website changed August 2026, by its sitemap' },
  { state: 'quiet', since: '2024-05-02T00:00:00Z', note: 'the website changed May 2024, by its sitemap' },
  { state: 'gone', since: null, note: 'the website is a parked domain' },
  { state: 'unknown', since: null, note: 'the website shows no date' },
]

function personDetail(s: State, p: PersonRow): PersonDetail {
  const seen = new Map<number, { source_id: number; source: string; checked_at: string }>()
  for (const e of s.events) {
    if (!e.people.some((x) => x.id === p.id)) continue
    for (const src of e.sources) {
      const ms = s.sources.find((x) => x.id === src.id)
      seen.set(src.id, { source_id: src.id, source: src.name, checked_at: hoursAgo(ms?.checked_hours_ago ?? 5) })
    }
  }
  const search = p.searchId ? s.sources.find((x) => x.id === p.searchId) : undefined
  if (search) seen.set(search.id, { source_id: search.id, source: search.name, checked_at: hoursAgo(search.checked_hours_ago ?? 5) })
  const fit_evidence = search ? `Found by the profile search ${search.name}` : p.id >= 900
    ? `Managing director of ${p.affiliations[0].organisation}, by its imprint. In the portfolio of Gateway startups`
    : p.fit === 'founder' ? `${p.name.split(' ')[0]} hat ${p.affiliations[0]?.organisation ?? 'das eigene Studio'} gegründet.` : ''
  return { ...personList(s, p), appearances: appearancesOf(s, p.id), notes: p.notes, fit_evidence, affiliations: p.affiliations, sightings: [...seen.values()] }
}

function sourceOut(m: MockSource, s?: State): Source {
  const { last_found, last_mode, last_error, last_http, checked_hours_ago, ...rest } = m
  const checked = checked_hours_ago === null ? null : hoursAgo(checked_hours_ago)
  const every = m.status === 'retired' ? 28 * 24 : 24
  // What the last check counted, as the server does: businesses for a
  // portfolio, a directory or a search, people for a post search.
  const lists = m.kind === 'post_search' || m.kind === 'profile_search' ? 'people' : ['portfolio', 'directory', 'search_query'].includes(m.kind) ? 'startups' : 'events'
  const found = s ? s.people.filter((p) => p.searchId === m.id) : []
  return {
    ...rest,
    people: found.length || (m.kind === 'profile_search' && last_found ? last_found : 0),
    people_connected: found.filter((p) => p.connected).length,
    last_checked_at: checked,
    next_check_at: checked ? hoursAgo((checked_hours_ago ?? 0) - every) : hoursAgo(-19),
    last_check: last_found === null ? null : {
      events_found: lists === 'events' ? last_found : 0, startups_found: lists === 'startups' ? last_found : 0, people_found: lists === 'people' ? last_found : 0,
      http_status: last_http, mode: last_mode, error: last_error,
    },
  }
}

// Routing

interface Out {
  status: number
  body?: unknown
  type?: string
  cookie?: string
}
type Handler = (s: State, m: RegExpMatchArray, q: URLSearchParams, body: Record<string, unknown>) => Out | undefined

// Like rubric.Excluded on the server: a phrase matches at the start of a
// word, whatever stands between its words, or written as one word.
function words(text: string): string[] {
  return text.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter(Boolean)
}
function excludedBy(text: string, phrases: string[]): string | null {
  const t = ' ' + words(text).join(' ')
  for (const p of phrases) {
    const w = words(p)
    if (w.length && (t.includes(' ' + w.join(' ')) || t.includes(' ' + w.join('')))) return p
  }
  return null
}
// Whom the exclusions hide, and how many each, never someone connected.
function hiddenBy(s: State): Map<number, string> {
  const phrases = s.exclusions.map((x) => x.phrase)
  const out = new Map<number, string>()
  for (const p of s.people) {
    const by = p.connected ? null : excludedBy(`${p.headline} ${p.linkedin?.roles ?? ''}`, phrases)
    if (by) out.set(p.id, by)
  }
  return out
}
function exclusionsOf(s: State) {
  const hidden = [...hiddenBy(s).values()]
  return [...s.exclusions]
    .sort((a, b) => a.phrase.toLowerCase().localeCompare(b.phrase.toLowerCase()))
    .map((x) => ({ ...x, people: hidden.filter((p) => p === x.phrase).length }))
}

const ok = (body: unknown) => ({ status: 200, body })
const err = (status: number, message: string) => ({ status, body: { error: message } })

const ROUTES: [string, RegExp, Handler][] = [
  ['GET', /^\/api\/people$/, (s, _m, q) => {
    const needle = (q.get('q') ?? '').toLowerCase()
    const sort = q.get('sort') || 'next'
    const filter = q.get('filter') || 'all'
    // Like the app: whom profile searches found, and whoever Tim connected
    // with, less whom the exclusions hide.
    const hidden = hiddenBy(s)
    let list = s.people.filter((p) => !hidden.has(p.id) && (p.linkedin || p.connected)).map((p) => personList(s, p))
    const year = new Date().getFullYear()
    const young = (p: Person) => !!p.linkedin?.born_about && p.linkedin.born_about > year - 30
    if (needle) list = list.filter((p) => `${p.name} ${p.headline} ${p.city}`.toLowerCase().includes(needle))
    const counts = {
      all: list.length,
      young: list.filter(young).length,
      connected: list.filter((p) => p.connected).length,
      fits: list.filter((p) => p.fit_score > 0).length,
      founder: list.filter((p) => p.fit === 'founder').length,
      upcoming: list.filter((p) => p.next_appearance).length,
      profile: list.filter((p) => p.profiles.some((x) => x.review !== 'rejected')).length,
    }
    if (filter === 'young') list = list.filter(young)
    if (filter === 'connected') list = list.filter((p) => p.connected)
    if (filter === 'upcoming') list = list.filter((p) => p.next_appearance)
    if (filter === 'profile') list = list.filter((p) => p.profiles.some((x) => x.review !== 'rejected'))
    if (filter === 'founder') list = list.filter((p) => p.fit === 'founder')
    if (filter === 'fits') list = list.filter((p) => p.fit_score > 0)
    if (sort === 'name') list.sort((a, b) => a.name.localeCompare(b.name, 'de'))
    else if (sort === 'fit') list.sort((a, b) => b.fit_score - a.fit_score || b.first_seen.localeCompare(a.first_seen))
    else if (sort === 'young') list.sort((a, b) => (b.linkedin?.born_about ?? 0) - (a.linkedin?.born_about ?? 0) || b.fit_score - a.fit_score)
    else if (sort === 'new') list.sort((a, b) => b.first_seen.localeCompare(a.first_seen) || a.name.localeCompare(b.name))
    else {
      // Like the server: next appearance, then founders of startups that look alive.
      const rank = (p: Person) => ['active', 'unknown', 'quiet', 'dissolved', 'gone'].indexOf(p.activity?.state ?? 'unknown')
      list.sort((a, b) => (a.next_appearance?.starts_at ?? '9999').localeCompare(b.next_appearance?.starts_at ?? '9999') || rank(a) - rank(b) || a.name.localeCompare(b.name))
    }
    return ok({ people: list, counts })
  }],
  ['GET', /^\/api\/people\/(\d+)$/, (s, m) => {
    const p = s.people.find((x) => x.id === Number(m[1]))
    return p ? ok(personDetail(s, p)) : err(404, 'No such person')
  }],
  ['PATCH', /^\/api\/people\/(\d+)$/, (s, m, _q, b) => {
    const p = s.people.find((x) => x.id === Number(m[1]))
    if (!p) return err(404, 'No such person')
    if (typeof b.notes === 'string') p.notes = b.notes
    if (typeof b.connected === 'boolean') p.connected = b.connected
    else if (b.connected !== undefined) return err(400, 'connected must be true or false')
    return ok(personDetail(s, p))
  }],
  ['PATCH', /^\/api\/profiles\/(\d+)$/, (s, m, _q, b) => {
    const prof = s.people.flatMap((p) => p.profiles).find((x) => x.id === Number(m[1]))
    if (!prof) return err(404, 'No such profile')
    if (b.review !== 'open' && b.review !== 'confirmed' && b.review !== 'rejected') return err(400, 'review must be open, confirmed or rejected')
    prof.review = b.review
    return { status: 204 }
  }],
  ['GET', /^\/api\/sources$/, (s, _m, q) => {
    const needle = (q.get('q') ?? '').toLowerCase()
    const status = q.get('status')
    const kind = q.get('kind')
    const list = s.sources
      .filter((x) => !status || x.status === status)
      .filter((x) => !kind || x.kind === kind)
      .filter((x) => !needle || `${x.name} ${x.url ?? ''} ${x.query ?? ''} ${x.city}`.toLowerCase().includes(needle))
      .sort((a, b) => Number(a.status === 'retired') - Number(b.status === 'retired') || b.points - a.points || a.name.localeCompare(b.name))
      .map((x) => sourceOut(x, s))
    return ok({ sources: list })
  }],
  // Updates, like cmd/speakertrail-app/updates_api.go: a build of pull
  // request 9 that follows its channel. Check finds nothing newer. Picking
  // another channel looks, downloads it over six seconds and has it ready.
  ['GET', /^\/api\/updates$/, () => ok(updateState())],
  ['POST', /^\/api\/updates\/check$/, () => {
    upd.lookFrom = Date.now()
    return { status: 204 }
  }],
  ['POST', /^\/api\/updates\/follow$/, (_s, _m, _q, b) => {
    const ch = String(b.channel ?? '')
    if (!UPDATE_CHANNELS.some((c) => c.id === ch)) return err(409, `There is no channel ${ch}`)
    upd.picked = ch
    upd.lookFrom = Date.now()
    upd.downloadFrom = ch === upd.running ? 0 : Date.now() + 1200
    return ok(updateState())
  }],
  ['POST', /^\/api\/updates\/relaunch$/, () => {
    const st = updateState()
    if (st.phase !== 'ready') return err(409, 'No newer build is ready')
    return { status: 204 }
  }],
  ['POST', /^\/api\/updates\/commit$/, () => ({ status: 204 })],
  ['GET', /^\/api\/exclusions$/, (s) => ok(exclusionsOf(s))],
  ['POST', /^\/api\/exclusions$/, (s, _m, _q, b) => {
    const phrase = words(String(b.phrase ?? '')).length ? String(b.phrase).split(/[^\p{L}\p{N}]+/u).filter(Boolean).join(' ') : ''
    if (!phrase) return err(400, 'Enter a word or phrase, like Online Marketing')
    if (phrase.length > 60) return err(400, 'An exclusion is at most 60 characters')
    if (s.exclusions.some((x) => x.phrase.toLowerCase() === phrase.toLowerCase())) return err(409, 'This exclusion is already there')
    const id = s.nextId.exclusion++
    s.exclusions.push({ id, phrase })
    return { status: 201, body: exclusionsOf(s).find((x) => x.id === id) }
  }],
  ['DELETE', /^\/api\/exclusions\/(\d+)$/, (s, m) => {
    const i = s.exclusions.findIndex((x) => x.id === Number(m[1]))
    if (i < 0) return err(404, 'Not found')
    s.exclusions.splice(i, 1)
    return { status: 204 }
  }],
  ['POST', /^\/api\/sources$/, (s, _m, _q, b) => {
    const query = typeof b.query === 'string' ? b.query.trim().split(/\s+/).join(' ') : ''
    const kind = b.kind === 'profile_search' ? 'profile_search' : 'search_query'
    if (b.kind !== undefined && b.kind !== 'profile_search' && b.kind !== 'search_query') return err(400, 'kind must be search_query or profile_search')
    if (query) {
      if (s.sources.some((x) => x.kind === kind && x.query?.toLowerCase() === query.toLowerCase())) return err(409, 'This search is already there')
      const city = ['Köln', 'Düsseldorf', 'Dortmund', 'Essen', 'Duisburg', 'Bochum', 'Wuppertal', 'Bielefeld', 'Bonn', 'Münster', 'Aachen'].find((c) => query.includes(c)) ?? ''
      const m: MockSource = {
        id: s.nextId.source++, name: query, kind, url: null, query,
        category: kind === 'profile_search' ? 'Profile search' : 'Search', city, status: 'candidate', fetch_mode: 'auto', notes: '', checks: 0, empty_checks_in_row: 0, points: kind === 'profile_search' ? 2 : 0,
        health: 'never', health_note: '', discovered_from: 'Added by Tim', last_found: null, last_mode: 'http', last_error: '', last_http: 0, checked_hours_ago: null,
      }
      s.sources.push(m)
      return { status: 201, body: sourceOut(m) }
    }
    const url = typeof b.url === 'string' ? b.url.trim() : ''
    if (!/^https?:\/\/\S+\.\S+/.test(url)) return err(400, 'That does not look like a web address')
    if (s.sources.some((x) => x.url === url)) return err(409, 'This source is already on the list')
    const host = new URL(url).hostname.replace(/^www\./, '')
    const m: MockSource = {
      id: s.nextId.source++, name: typeof b.name === 'string' && b.name.trim() ? b.name.trim() : host, kind: b.lists === 'startups' || b.portfolio === true ? 'portfolio' : b.lists === 'businesses' ? 'directory' : 'listing', url, query: null,
      category: '', city: '', status: 'candidate', fetch_mode: 'auto', notes: '', checks: 0, empty_checks_in_row: 0, points: 0,
      health: 'never', health_note: '', discovered_from: 'Added by hand', last_found: null, last_mode: 'http', last_error: '', last_http: 0, checked_hours_ago: null,
    }
    s.sources.push(m)
    return { status: 201, body: sourceOut(m) }
  }],
  ['PATCH', /^\/api\/sources\/(\d+)$/, (s, m, _q, b) => {
    const src = s.sources.find((x) => x.id === Number(m[1]))
    if (!src) return err(404, 'No such source')
    if (typeof b.status === 'string') src.status = b.status as SourceStatus
    if (b.fetch_mode === 'auto' || b.fetch_mode === 'http' || b.fetch_mode === 'browser') src.fetch_mode = b.fetch_mode
    if (typeof b.notes === 'string') src.notes = b.notes
    if (typeof b.name === 'string') src.name = b.name
    if (typeof b.lists === 'string') src.kind = b.lists === 'startups' ? 'portfolio' : b.lists === 'businesses' ? 'directory' : 'listing'
    return ok(sourceOut(src, s))
  }],
  ['POST', /^\/api\/sources\/(\d+)\/check$/, (s, m) => {
    const src = s.sources.find((x) => x.id === Number(m[1]))
    if (!src) return err(404, 'No such source')
    // Pretend the check finishes a few seconds later.
    setTimeout(() => {
      src.checked_hours_ago = 0
      src.checks++
      if (src.last_found === null) src.last_found = 2
      if (src.health === 'never') src.health = 'ok'
    }, 3000)
    return { status: 202 }
  }],
  ['GET', /^\/api\/runs$/, (s) => ok({ runs: s.runs })],
  ['GET', /^\/api\/runs\/current$/, (s) => ok({ run: s.runs.find((r) => r.state === 'going' && r.kind !== 'check') ?? null })],
  ['POST', /^\/api\/runs\/(\d+)\/stop$/, (s, m) => {
    const run = s.runs.find((r) => r.id === Number(m[1]))
    if (!run) return err(404, 'No such run')
    if (run.state !== 'going') return err(409, 'This run has already ended')
    run.finished_at = new Date().toISOString()
    run.state = 'stopped'
    run.progress = null
    return { status: 204 }
  }],
  ['POST', /^\/api\/runs$/, (s) => {
    const going = s.runs.find((r) => r.state === 'going' && r.kind !== 'check')
    if (going) return { status: 202, body: { run_id: going.id, started: false } }
    const run: Run = {
      id: Math.max(0, ...s.runs.map((r) => r.id)) + 1, kind: 'manual' as const, state: 'going', started_at: new Date().toISOString(), finished_at: null as string | null,
      sources_checked: 0, events_found: 0, events_new: 0, pages_read: 0, reads_failed: 0, startups_looked_up: 0, lookups_failed: 0,
      people_new: 0, fits_new: 0, errors: 0,
      // Like the server: 8 checks four at a time, then the reads they bring,
      // one at a time, and 4 lookups of startups from a portfolio among the
      // checks, measured at about 3 s a check, 5 s a read and 4 s a lookup.
      progress: {
        checks: 8, checks_done: 0, reads: 0, reads_done: 0, reads_expected: 6, lookups: 0, lookups_done: 0, lookups_expected: 4,
        now: [] as RunProgress['now'], seconds_left: 40,
      } as RunProgress | null,
    }
    s.runs.unshift(run)
    s.checks.set(run.id, [])
    // Pretend the run checks a few sources, then finishes.
    const names = s.sources.slice(0, 8).map((x) => x.name)
    const titles = s.events.slice(0, 6).map((e) => e.title)
    const startups = ['Beispiel Robotics', 'Probe Labs', 'Muster Health', 'Kontrolle Analytics']
    // People the run finds, one or two at a time, so People fills while
    // Tim watches. All invented.
    const found: [string, string, Person['fit']][] = [
      ['Frieda Faustmann', 'Coach, Faustmann Coaching', 'other'], ['Kemal Kastner', 'Inhaber, Kastner Kampfsport', 'founder'],
      ['Nora Nettelbeck', 'Author, Das leise Jahr', 'other'], ['Paul Pannwitz', 'Founder, Pannwitz Putzteam', 'founder'],
      ['Rike Reimers', 'Yoga teacher, Studio Reimers', 'other'], ['Sven Sandkamp', 'Head coach, TV Beispielstadt', 'athlete'],
    ]
    const addPerson = () => {
      const next = found.shift()
      if (!next) return
      const [name, headline, fit] = next
      s.people.push({
        id: Math.max(...s.people.map((x) => x.id)) + 1, name, headline, city: 'Köln', fit, first_seen: new Date().toISOString(),
        notes: '', profiles: [], affiliations: [],
      })
      run.people_new++
      if (fit !== 'other') run.fits_new++
    }
    const t = setInterval(() => {
      const p = run.progress
      // Stopped by hand, or done.
      if (run.state !== 'going' || !p) {
        run.progress = null
        return clearInterval(t)
      }
      const since = new Date().toISOString()
      if (p.checks_done < p.checks) {
        p.checks_done = Math.min(p.checks, p.checks_done + 2)
        run.sources_checked = p.checks_done
        run.events_found += 5
        run.events_new += 1
        p.reads = Math.min(6, p.checks_done - 1)
        p.now = p.checks_done < p.checks ? names.slice(p.checks_done, p.checks_done + 2).map((label) => ({ kind: 'check' as const, label, since })) : []
      } else if (p.lookups_done < 4) {
        // The portfolio among the checks brought its lookups.
        p.lookups = 4
        p.lookups_done = Math.min(4, p.lookups_done + 2)
        run.startups_looked_up = p.lookups_done
        addPerson()
        p.now = p.lookups_done < 4 ? startups.slice(p.lookups_done, p.lookups_done + 2).map((label) => ({ kind: 'lookup' as const, label, since })) : []
      } else if (p.reads_done < p.reads) {
        p.reads_done++
        run.pages_read = p.reads_done
        addPerson()
        p.now = p.reads_done < p.reads ? [{ kind: 'read' as const, label: titles[p.reads_done % titles.length], since }] : []
      } else {
        run.finished_at = new Date().toISOString()
        run.state = 'finished'
        run.progress = null
        return clearInterval(t)
      }
      p.reads_expected = Math.max(p.reads, 6)
      p.seconds_left = ((p.checks - p.checks_done) * 3 + (p.lookups_expected - p.lookups_done) * 4) / 4 + (p.reads_expected - p.reads_done) * 5
    }, 2000)
    return { status: 202, body: { run_id: run.id, started: true } }
  }],
  ['GET', /^\/api\/runs\/(\d+)$/, (s, m) => {
    const run = s.runs.find((r) => r.id === Number(m[1]))
    return run ? ok({ run, checks: s.checks.get(run.id) ?? [], failures: failuresOf(s, run) }) : err(404, 'No such run')
  }],
  ['GET', /^\/api\/settings$/, (s) => ok({ settings: s.settings })],
  ['PATCH', /^\/api\/settings\/([^/]+)$/, (s, m, _q, b) => {
    const row = s.settings.find((x: Setting) => x.key === decodeURIComponent(m[1]))
    if (!row) return err(404, 'No such setting')
    if (!('value' in b)) return err(400, 'value is missing')
    if (typeof row.value !== typeof b.value) return err(400, `value must be a ${Array.isArray(row.value) ? 'list' : typeof row.value}`)
    row.value = b.value as Json
    row.updated_at = new Date().toISOString()
    return ok(row)
  }],
  // Like the app: whether the Exa key is saved, never the key.
  ['GET', /^\/api\/keys$/, (s) => ok({ keys: [{ provider: 'exa', used: 42, budget: Number(setting(s, 'exa_monthly_searches') ?? 1000), set: exaKey !== '' }] })],
  ['PUT', /^\/api\/keys\/exa$/, (_s, _m, _q, b) => {
    exaKey = String(b.key ?? '').trim()
    return { status: 204 }
  }],
]

// The mock's Exa key, saved as the app saves it in the keychain.
let exaKey = 'invented'

function readBody(req: Req): Promise<Record<string, unknown>> {
  return new Promise((resolve) => {
    const chunks: Uint8Array[] = []
    req.on('data', (c) => chunks.push(c))
    req.on('end', () => {
      const total = chunks.reduce((a, c) => a + c.length, 0)
      const buf = new Uint8Array(total)
      let o = 0
      for (const c of chunks) {
        buf.set(c, o)
        o += c.length
      }
      const text = new TextDecoder().decode(buf)
      try {
        const parsed = text ? JSON.parse(text) : {}
        resolve(parsed && typeof parsed === 'object' ? parsed : {})
      } catch {
        resolve({})
      }
    })
    req.on('error', () => resolve({}))
  })
}

export function mockApi(): Plugin {
  const state = createState()
  return {
    name: 'speakertrail-mock-api',
    apply: 'serve',
    configureServer(server) {
      server.config.logger.info(`\n  Mock API on, with invented data.\n`)
      server.middlewares.use(async (rawReq: unknown, rawRes: unknown, next: () => void) => {
        const req = rawReq as Req
        const res = rawRes as Res
        const url = new URL(req.url ?? '/', 'http://mock')
        if (!url.pathname.startsWith('/api/')) return next()
        const body = req.method === 'POST' || req.method === 'PATCH' || req.method === 'PUT' ? await readBody(req) : {}
        await new Promise((r) => setTimeout(r, DELAY_MS))
        const send = (out: Out) => {
          res.statusCode = out.status
          if (out.status === 204 || out.body === undefined) return res.end()
          res.setHeader('Content-Type', out.type ?? 'application/json; charset=utf-8')
          res.end(typeof out.body === 'string' && out.type ? out.body : JSON.stringify(out.body))
        }
        // The app has no login, so neither has the mock.
        for (const [method, re, handler] of ROUTES) {
          const m = url.pathname.match(re)
          if (m && req.method === method) return send(handler(state, m, url.searchParams, body) ?? err(500, 'No answer'))
        }
        send(err(404, 'Not found'))
      })
    },
  }
}
