// Dev-only mock of the Speaker Trail API (docs/api.md).
// Used by `MOCK=1 npm run dev`. Never imported by the app, never built.
// State lives in memory and resets when the dev server restarts.
// The mock password is "speakertrail".

import type { Plugin } from 'vite'
import type {
  Activity, Appearance, DraftJob, FitLine, FitPart, FitSignal, Json, Person, PersonDetail, PrivateEvent, Profile,
  Run, RunFailure, Setting, Source, SourceStatus, UpdateChannel, UpdateState,
  FirstMessageTemplate, TemplateField, TemplatePart, UndoThing,
} from '../src/lib/api.ts'
import {
  addDays, berlin, buildRuns, buildSettings, buildSources, EVENT_SEEDS, hoursAgo, PERSON_INFO,
  todayBerlin, type MockSource,
} from './fixtures.ts'
// Tim's first template, the very file the app holds, see
// internal/store/first_message_template.json.
import firstTemplate from '../../internal/store/first_message_template.json'

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

// How long the mock waits before each answer unless told otherwise, so a
// person trying the interface in `make mock` sees what shows while an
// answer is on its way. The walks set MOCK_DELAY_MS to 0, see
// walks/lib/mock.mjs: a walk that needs an answer on its way holds that
// request itself.
const DELAY_MS = 280

// The mock's clock. It runs as the real one does, so in `make mock` a
// run, a download or a draft takes the seconds a person sees it take. A
// walk moves it on with POST /api/mock/clock, and what falls due by then
// happens at once, in order, so no walk waits seconds of real time for
// the mock, #96.
let ahead = 0
const clockNow = (): number => Date.now() + ahead
const clockDate = () => new Date(clockNow())
// What the mock does later by its clock: once, or every so often.
interface Timer { due: number; every: number; fn: () => void; real?: ReturnType<typeof setTimeout> }
const timers = new Set<Timer>()
function later(ms: number, fn: () => void, every = 0): () => void {
  const t: Timer = { due: clockNow() + ms, every, fn }
  timers.add(t)
  arm(t)
  return () => {
    clearTimeout(t.real)
    timers.delete(t)
  }
}
function arm(t: Timer) {
  clearTimeout(t.real)
  t.real = setTimeout(() => fire(t), Math.max(0, t.due - clockNow()))
}
function fire(t: Timer) {
  if (!timers.has(t)) return
  if (t.every) {
    t.due += t.every
    arm(t)
  } else timers.delete(t)
  t.fn()
}
function moveClock(ms: number) {
  const to = clockNow() + Math.max(0, ms)
  for (;;) {
    let next: Timer | undefined
    for (const t of timers) if (t.due <= to && (!next || t.due < next.due)) next = t
    if (!next) break
    ahead = next.due - Date.now()
    fire(next)
  }
  ahead = to - Date.now()
  for (const t of timers) arm(t)
}

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
  linkedin?: Omit<NonNullable<Person['linkedin']>, 'url'>
  searchId?: number
  // The first message to them and when it was drafted.
  draft?: string
  drafted_at?: string
  // Where reading the profile of someone added from a link stands, like
  // the app's, see readsOf.
  read?: Person['read']
  read_since?: string
  read_error?: string
  // A LinkedIn page Tim pasted as their profile, and when.
  text?: string
  text_at?: string
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
      // Founded something, but works at another organisation, so it
      // stands below the owners (Tim, 22 November 2026).
      ['Nele Muster', 'Mitgründerin, Probe AI und Digitale Projekte beim Beispielverein', 'Köln', 2000, 'First job from 2021', 14, 101],
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
const upd = { running: 'pr-9', picked: '', lookFrom: clockNow(), downloadFrom: 0 }
function updateState(): UpdateState {
  const now = clockNow()
  const follows = upd.picked || upd.running
  const ch = UPDATE_CHANNELS.find((c) => c.id === follows)!
  const base: UpdateState = {
    version: '0.1.0-pr9.e808a87', commit: 'e808a87ee3a7', channel: upd.running, off: '', channels: UPDATE_CHANNELS,
    picked: upd.picked, follows, gone: '', ended: '', building: '', phase: 'current', next: '', nextName: '', nextCommit: '',
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
  [/student|in ausbildung/i, 'studies', 'Studies or in training', false],
  [/ beim /i, 'employed', 'Employed elsewhere', false],
  [/social media|content creator|influencer/i, 'social_media', 'Social media work', false],
  [/\bai\b|\bki\b/i, 'ai', 'Builds on AI', false],
]

function signalsOf(p: PersonRow, apps: Appearance[]): FitSignal[] {
  const out: FitSignal[] = []
  const add = (key: string, label: string, isFor: boolean, passage: string, found_in: string) => {
    if (!out.some((x) => x.key === key)) out.push({ key, label, for: isFor, passage, found_in })
  }
  // Some invented people have a profile with dates, some a recent post.
  const year = clockDate().getFullYear()
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

// The centres of the invented people's cities, from internal/place, so
// Near you answers the way the app does.
const CENTRES: Record<string, [number, number]> = {
  Köln: [50.9333, 6.95], Bonn: [50.7344, 7.0955], Düsseldorf: [51.2232, 6.7793], Essen: [51.4566, 7.0123],
  Dortmund: [51.5149, 7.466], Duisburg: [51.4325, 6.7652], Bochum: [51.4817, 7.2165], Aachen: [50.7766, 6.0834],
  Münster: [51.9624, 7.6257], Bielefeld: [52.0333, 8.5333], Neuss: [51.1981, 6.685], Hürth: [50.8708, 6.8676],
  Siegburg: [50.8002, 7.2077],
}
const ALIASES: Record<string, string> = { cologne: 'Köln', koeln: 'Köln', dusseldorf: 'Düsseldorf', duesseldorf: 'Düsseldorf', muenster: 'Münster' }
function cityOf(v: string): string | undefined {
  const t = v.trim().toLowerCase()
  return Object.keys(CENTRES).find((c) => c.toLowerCase() === t) ?? ALIASES[t]
}
function km(a: string, b: string): number | null {
  const p = CENTRES[a], q = CENTRES[b]
  if (!p || !q) return null
  const r = Math.PI / 180
  const h = Math.sin(((q[0] - p[0]) * r) / 2) ** 2 + Math.cos(p[0] * r) * Math.cos(q[0] * r) * Math.sin(((q[1] - p[1]) * r) / 2) ** 2
  return 2 * 6371 * Math.asin(Math.sqrt(h))
}

// The fit's four parts, the way rubric.Fit gives them.
const WHO = new Set(['works_with_people', 'bootstrapped', 'runs_events', 'author', 'athlete', 'career_switch', 'funded', 'enterprise', 'backed', 'studies', 'employed', 'social_media', 'ai'])
function fitParts(s: State, p: Person): FitPart[] {
  // Who they are 40, active lately 30, near you 20, younger 10, as in
  // internal/rubric/fit.go.
  const pts = (most: number, share: number) => Math.round(most * Math.max(0, Math.min(1, share)))
  const counted = p.signals.filter((x) => WHO.has(x.key))
  const pro = counted.filter((x) => x.for).map((x) => x.label)
  const contra = counted.filter((x) => !x.for).map((x) => x.label)
  const why = [...pro, ...contra].join(', ')
  const who = { key: 'who', label: 'Who they are', points: Math.round(40 * Math.max(-1, Math.min(1, (pro.length - contra.length) / 3))), why: why || 'Nothing beyond running their own business' }
  const now = clockDate()
  const born = p.linkedin?.born_about
  const age = born ? now.getFullYear() - born : 0
  const younger = born
    ? { key: 'younger', label: 'Younger', points: Math.round(Math.max(0, Math.min(10, (10 * (40 - age)) / 10))), why: p.linkedin?.age_reason ? `${p.linkedin.age_reason}, so about ${age}` : `About ${age}` }
    : { key: 'younger', label: 'Younger', points: pts(10, 0.5), why: 'Age not known' }
  const last = p.linkedin?.last_active
  const day = (t: string) => new Date(t).toLocaleDateString('en-GB', { day: 'numeric', month: 'long', year: 'numeric' })
  const active = last
    ? { key: 'active', label: 'Active lately', points: pts(30, 0.5 ** ((now.getTime() - new Date(last).getTime()) / (30 * 864e5)) / 2), why: `Last active on ${day(last)}, not known whether they wrote` }
    : { key: 'active', label: 'Active lately', points: 0, why: 'No activity on LinkedIn known' }
  const home = String(setting(s, 'home_city') ?? '')
  const d = home ? km(home, p.city) : null
  const near = !home
    ? { key: 'near', label: 'Near you', points: pts(20, 0.5), why: 'Where you are is not set in Settings' }
    : d === null
      ? { key: 'near', label: 'Near you', points: pts(20, 0.5), why: p.city ? `${p.city}, distance not known` : 'City not known' }
      : d < 1
        ? { key: 'near', label: 'Near you', points: 20, why: `${p.city}, where you are` }
        : { key: 'near', label: 'Near you', points: pts(20, 0.5 ** (d / 40)), why: `${p.city}, about ${Math.round(d)} km` }
  // Who they are takes away no more than the rest gives, so the fit
  // stays 0 or more.
  who.points = Math.max(who.points, -(active.points + near.points + younger.points))
  // The lines under each part, as internal/api fitParts gives them: the
  // signals under Who they are, the why with a tick or a cross under
  // the others, the way rubric.Fit marks it.
  const mark = (pts: number, most: number, known: boolean): FitLine['mark'] =>
    !known ? 'neither' : 2 * pts >= most ? 'for' : 4 * pts < most ? 'against' : 'neither'
  const line = (m: FitLine['mark'], text: string): FitLine => ({ mark: m, text, passage: '', found_in: '' })
  const whoLines = p.signals
    .filter((x) => !['young', 'active_on_linkedin', 'quiet_on_linkedin'].includes(x.key))
    .map((x): FitLine => ({ mark: x.for ? 'for' : 'against', text: x.label, passage: x.passage, found_in: x.found_in }))
  return [
    { ...who, lines: whoLines.length ? whoLines : [line('neither', who.why)] },
    { ...active, lines: [line(mark(active.points, 30, true), active.why)] },
    { ...near, lines: [line(mark(near.points, 20, !!home && d !== null), near.why)] },
    { ...younger, lines: [line(mark(younger.points, 10, !!born), younger.why)] },
  ]
}

function personList(s: State, p: PersonRow): Person {
  const apps = appearancesOf(s, p.id)
  const now = clockDate().toISOString()
  const next = apps.find((a) => a.event.starts_at >= now)
  const signals = signalsOf(p, apps)
  const out: Person = {
    id: p.id, name: p.name, known_as: '', headline: p.headline, city: p.city, fit: p.fit, first_seen: p.first_seen,
    fit_score: 0, signals, connected: !!p.connected,
    appearances: apps.length,
    next_appearance: next
      ? { event_id: next.event.id, title: next.event.title, starts_at: next.event.starts_at, city: next.event.city, role: next.role }
      : null,
    profiles: p.profiles,
    // Like the server: no facts from a profile Tim said is someone else's.
    linkedin: (() => {
      const l = p.profiles.find((x) => x.platform === 'linkedin')
      // Like the server, someone whose profile is not read yet has their
      // link all the same, and no facts.
      if (!l || l.review === 'rejected') return null
      return p.linkedin ? { ...p.linkedin, url: l.url } : { url: l.url, born_about: null, age_reason: '', roles: '', last_active: null }
    })(),
    // Founders of a startup come with what its lookup saw, like the server.
    activity: p.fit === 'founder' && !next && p.affiliations.some((a) => a.role === 'founder')
      ? { ...MOCK_ACTIVITY[p.id % MOCK_ACTIVITY.length], company: p.affiliations[0]?.organisation ?? 'Beispiel GmbH' }
      : null,
    read: p.read ?? '',
    read_since: p.read === 'reading' ? p.read_since ?? null : null,
    read_ms: p.read === 'reading' ? READ_MS : null,
    read_error: p.read_error,
  }
  out.fit_score = fitParts(s, out).reduce((n, x) => n + x.points, 0)
  return out
}

// Links to LinkedIn profiles in a text, in one spelling, and how many
// other links it holds, like engine.Links.
function profileLinks(text: string): { profiles: string[]; other: number } {
  const profiles: string[] = []
  let other = 0
  for (let word of text.split(/\s+/).filter(Boolean)) {
    word = word.replace(/^[<>()[\]"',;]+|[<>()[\]"',;]+$/g, '')
    const lower = word.toLowerCase()
    if (!lower.includes('://') && !lower.startsWith('www.') && !lower.includes('linkedin.com')) continue
    let u: URL
    try {
      u = new URL(lower.includes('://') ? word : `https://${word}`)
    } catch {
      other++
      continue
    }
    const host = u.hostname.toLowerCase().replace(/^www\./, '')
    const path = u.pathname.replace(/\/$/, '')
    if ((host === 'linkedin.com' || host.endsWith('.linkedin.com')) && /^\/in\/[^/]+$/.test(path)) {
      const link = `https://www.linkedin.com${path}`
      if (!profiles.includes(link)) profiles.push(link)
    } else other++
  }
  return { profiles, other }
}

// About how long Exa takes to give one profile in the mock, and how many
// it reads at once, like the app's SideBySide.
const READ_MS = 4000
const SIDE_BY_SIDE = 4

// What adding links does, like actOf in the app.
const actOf = (isNew: number, here: number, other: number) => (isNew > 0 ? 'add' : here === 1 && !other ? 'open' : 'none') as 'add' | 'open' | 'none'

const linkOf = (p: PersonRow) => p.profiles.find((x) => x.platform === 'linkedin')?.url

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
  // Like the app: someone added from a link says so.
  const fit_evidence = search ? `Found by the profile search ${search.name}` : p.read !== undefined ? 'Added by hand' : p.id >= 900 && p.id < 1000
    ? `Managing director of ${p.affiliations[0].organisation}, by its imprint. In the portfolio of Gateway startups`
    : p.fit === 'founder' ? `${p.name.split(' ')[0]} hat ${p.affiliations[0]?.organisation ?? 'das eigene Studio'} gegründet.` : ''
  return {
    ...personList(s, p), appearances: appearancesOf(s, p.id), notes: p.notes, fit_evidence, affiliations: p.affiliations, sightings: [...seen.values()],
    draft: p.draft ?? '', drafted_at: p.drafted_at ?? null, profile_read_at: p.text_at ?? (p.text || p.linkedin ? hoursAgo(30) : null),
    fit_parts: fitParts(s, personList(s, p)),
  }
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

// The profiles being read and waiting, for every run, like the app's
// queue. A run ends once its searches are checked and its profiles read.
const reads = { queue: [] as { id: number; run: number }[] }

// Starts a run like the app's: its searches are checked two every two
// seconds, about 3 s a check four at a time, each finding someone now and
// then, and the profiles of links added are read four at a time, about
// READ_MS each. All the people are invented.
function startRun(s: State, kind: Run['kind'], checks: number): Run {
  const run: Run = {
    id: Math.max(0, ...s.runs.map((r) => r.id)) + 1, kind, state: 'going', started_at: clockDate().toISOString(), finished_at: null,
    sources_checked: 0, events_found: 0, events_new: 0, pages_read: 0, reads_failed: 0, startups_looked_up: 0, lookups_failed: 0,
    people_new: 0, errors: 0,
    progress: { checks, checks_done: 0, reads: 0, reads_done: 0, reads_expected: 0, lookups: 0, lookups_done: 0, lookups_expected: 0, now: [], seconds_left: null },
  }
  s.runs.unshift(run)
  s.checks.set(run.id, [])
  const searches = s.sources.slice(0, checks).map((x) => x.query ?? x.name)
  const found: [string, string][] = [
    ['Frieda Faustmann', 'Coach, Faustmann Coaching'], ['Kemal Kastner', 'Inhaber, Kastner Kampfsport'],
    ['Rike Reimers', 'Yogalehrerin, Studio Reimers'], ['Paul Pannwitz', 'Gründer, Pannwitz Putzteam'],
  ]
  const stop = later(1000, () => {
    const p = run.progress
    if (run.state !== 'going' || !p) {
      run.progress = null
      // Stopped by hand: the profiles not read wait for the next run.
      for (const q of reads.queue.filter((x) => x.run === run.id)) {
        const person = s.people.find((x) => x.id === q.id)
        if (person) person.read = 'next'
      }
      reads.queue = reads.queue.filter((x) => x.run !== run.id)
      return stop()
    }
    const since = clockDate().toISOString()
    if (p.checks_done < p.checks) {
      p.checks_done = Math.min(p.checks, p.checks_done + 2)
      run.sources_checked = p.checks_done
      const next = found.shift()
      if (next && p.checks_done % 4 === 0) {
        s.people.push({
          id: Math.max(...s.people.map((x) => x.id)) + 1, name: next[0], headline: next[1], city: 'Köln', fit: 'founder', first_seen: since, notes: '',
          profiles: [], affiliations: [], linkedin: { born_about: 1996, age_reason: 'Bachelor from 2018', roles: next[1], last_active: since },
        })
        run.people_new++
      }
    }
    // The reads of this run: done ones fill in, the next take their place.
    const mine = reads.queue.filter((x) => x.run === run.id)
    for (const q of mine) {
      const person = s.people.find((x) => x.id === q.id)
      if (!person) continue
      if (person.read === 'reading' && clockNow() - new Date(person.read_since!).getTime() >= READ_MS) {
        readDone(person)
        p.reads_done++
        run.people_new++
        reads.queue = reads.queue.filter((x) => x !== q)
      }
    }
    let busy = s.people.filter((x) => x.read === 'reading').length
    for (const q of reads.queue.filter((x) => x.run === run.id)) {
      const person = s.people.find((x) => x.id === q.id)
      if (person?.read === 'waiting' && busy < SIDE_BY_SIDE) {
        person.read = 'reading'
        person.read_since = since
        busy++
      }
    }
    p.now = [
      ...(p.checks_done < p.checks ? searches.slice(p.checks_done, p.checks_done + 2).map((label) => ({ kind: 'check' as const, label, since })) : []),
      ...s.people.filter((x) => x.read === 'reading').map((x) => ({ kind: 'read' as const, label: x.name, since: x.read_since! })),
    ]
    const left = ((p.checks - p.checks_done) * 3) / SIDE_BY_SIDE + ((p.reads - p.reads_done) * READ_MS) / 1000 / SIDE_BY_SIDE
    p.seconds_left = left
    if (p.checks_done >= p.checks && p.reads_done >= p.reads) {
      run.finished_at = clockDate().toISOString()
      run.state = 'finished'
      run.progress = null
      stop()
    }
  }, 1000)
  return run
}

// What Exa gives for a profile in the mock: an invented person for most
// handles, and nothing for one with "neu" in it, which is not in the
// index yet.
function readDone(p: PersonRow) {
  const handle = p.name
  p.read_since = undefined
  // A draft that waited on the read goes on, see draft.Drafter.Await.
  const waiting = drafts.get(p.id)
  if (waiting?.reads && waiting.readAt === undefined) waiting.readAt = clockNow()
  if (handle.includes('neu')) {
    p.read = 'not_indexed'
    p.read_error = "Not in Exa's index yet"
    if (waiting?.reads) waiting.error = NO_COPY
    return
  }
  p.read = ''
  p.name = handle.split('-').filter((w) => !/^\d+$/.test(w)).map((w) => w[0].toUpperCase() + w.slice(1)).join(' ') || handle
  p.headline = 'Inhaberin, eigenes Studio'
  p.city = 'Köln'
  p.linkedin = { born_about: 1995, age_reason: 'Bachelor from 2017', roles: p.headline, last_active: clockDate().toISOString() }
}

const ROUTES: [string, RegExp, Handler][] = [
  // A walk moves the mock's clock, see moveClock.
  ['POST', /^\/api\/mock\/clock$/, (_s, _m, _q, body) => {
    moveClock(Number(body.ms) || 0)
    return ok({ now: clockNow() })
  }],
  ['GET', /^\/api\/people$/, (s, _m, q) => {
    const needle = (q.get('q') ?? '').toLowerCase()
    const sort = q.get('sort') || 'fit'
    const filter = q.get('filter') || 'all'
    // Like the app: whom profile searches found, and whoever Tim connected
    // with, less whom the exclusions hide.
    const hidden = hiddenBy(s)
    let list = s.people.filter((p) => !hidden.has(p.id) && (p.linkedin || p.connected)).map((p) => personList(s, p))
    // Like the app: in the last new_days days, newest first.
    const newDays = Number(setting(s, 'new_days') ?? 7)
    const fresh = (p: Person) => clockNow() - new Date(p.first_seen).getTime() < newDays * 86400e3
    // A search that holds links to profiles shows whom they lead to, under
    // any filter, and says how many are new.
    const found = profileLinks(q.get('q') ?? '')
    const linked = found.profiles.length > 0 || found.other > 0
    if (needle && !linked) list = list.filter((p) => `${p.name} ${p.headline} ${p.city}`.toLowerCase().includes(needle))
    // Like the app: someone Tim connected with is only under Connected,
    // and someone whose LinkedIn profile he rejected only under Rejected.
    const rejected = (p: Person) => !p.connected && p.profiles.some((x) => x.platform === 'linkedin' && x.review === 'rejected')
    const open = (p: Person) => !p.connected && !rejected(p)
    const counts = {
      all: list.filter(open).length,
      new: list.filter((p) => !rejected(p) && fresh(p)).length,
      connected: list.filter((p) => p.connected).length,
      rejected: list.filter(rejected).length,
      fits: list.filter((p) => p.fit_score > 0).length,
      founder: list.filter((p) => p.fit === 'founder').length,
      upcoming: list.filter((p) => p.next_appearance).length,
      profile: list.filter((p) => p.profiles.some((x) => x.review !== 'rejected')).length,
    }
    if (filter === 'all') list = list.filter(open)
    if (filter === 'new') list = list.filter((p) => !rejected(p) && fresh(p))
    if (filter === 'connected') list = list.filter((p) => p.connected)
    if (filter === 'rejected') list = list.filter(rejected)
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
    // New is newest first, whatever the order, like the app.
    if (filter === 'new') list.sort((a, b) => b.first_seen.localeCompare(a.first_seen) || a.name.localeCompare(b.name))
    if (linked) {
      const here = s.people.filter((p) => found.profiles.includes(linkOf(p) ?? ''))
      return ok({
        people: here.map((p) => personList(s, p)), counts,
        links: { new: found.profiles.length - here.length, here: here.map((p) => p.id), other: found.other, act: actOf(found.profiles.length - here.length, here.length, found.other) },
        new_days: newDays,
      })
    }
    return ok({ people: list, counts, links: null, new_days: newDays })
  }],
  ['POST', /^\/api\/people$/, (s, _m, _q, b) => {
    const found = profileLinks(String(b.text ?? ''))
    if (!found.profiles.length) return err(400, 'There is no link to a LinkedIn profile in it, like linkedin.com/in/name')
    const at = clockDate().toISOString()
    const added: number[] = []
    const here: number[] = []
    const connected: number[] = []
    for (const link of found.profiles) {
      const p = s.people.find((x) => linkOf(x) === link)
      if (p) {
        if (!p.connected) connected.push(p.id)
        p.connected = true
        here.push(p.id)
        continue
      }
      // Named by its handle until the profile is read, like the app.
      const id = Math.max(...s.people.map((x) => x.id)) + 1
      s.people.push({
        id, name: link.slice(link.lastIndexOf('/') + 1), headline: '', city: '', fit: 'other', first_seen: at, notes: '',
        profiles: [{ id: 50000 + id, platform: 'linkedin', url: link, review: 'confirmed' }], affiliations: [], connected: true, read: 'waiting',
      })
      added.push(id)
    }
    let run = s.runs.find((r) => r.state === 'going' && r.kind !== 'check')
    if (added.length) {
      if (!run) run = startRun(s, 'add', 0)
      run.progress!.reads += added.length
      run.progress!.reads_expected = run.progress!.reads
      reads.queue.push(...added.map((id) => ({ id, run: run!.id })))
    }
    let open = actOf(added.length, here.length, found.other) === 'open' ? here[0] : undefined
    // Like the app: one link that adds one person and nothing else opens
    // them and starts their first message, which waits for the run's
    // read of their profile (#115).
    let drafting = false
    if (added.length === 1 && !here.length && !found.other) {
      open = added[0]
      if (languageState().models.some((x) => x.inUse && x.installed)) {
        draftJobs++
        drafts.set(open, { id: `draft-${draftJobs}`, person_id: open, from: clockNow(), started_at: at, reads: true })
        drafting = true
      }
    }
    return ok({ run: run?.id ?? 0, new: added, here, connected, other: found.other, open, drafting })
  }],
  ['DELETE', /^\/api\/people\/(\d+)$/, (s, m) => {
    const p = s.people.find((x) => x.id === Number(m[1]))
    if (!p) return err(404, 'Not found')
    if (p.read === undefined) return err(409, 'Only someone added from a link can be taken out again')
    s.people = s.people.filter((x) => x !== p)
    // Like the app: out of the run that had not read it yet.
    for (const q of reads.queue.filter((x) => x.id === p.id)) {
      const run = s.runs.find((r) => r.id === q.run)
      if (run?.progress) run.progress.reads = run.progress.reads_expected = run.progress.reads - 1
    }
    reads.queue = reads.queue.filter((x) => x.id !== p.id)
    // Like the app: their draft has nobody left to write to.
    const d = drafts.get(p.id)
    if (d && !['done', 'failed', 'cancelled'].includes(draftJob(s, d).state)) d.cancelled = true
    return { status: 204 }
  }],
  ['PUT', /^\/api\/people\/(\d+)\/profile$/, (s, m, _q, b) => {
    const p = s.people.find((x) => x.id === Number(m[1]))
    if (!p) return err(404, 'Not found')
    return pasteOn(s, p, b)
  }],
  // Like engine.Whose: the person whose link the page holds most often,
  // else the one of the name on the page, else the person open.
  ['POST', /^\/api\/people\/profile$/, (s, _m, _q, b) => {
    const lines = String(b.text ?? '').split('\n').map((l) => l.trim()).filter(Boolean)
    if (!lines.length) return err(400, 'The paste holds no text')
    const count = new Map<string, number>()
    for (const h of (b.links as string[] | undefined) ?? []) {
      const n = /\/in\/([^/?#]+)/.exec(h)?.[1]?.toLowerCase()
      if (n) count.set(n, (count.get(n) ?? 0) + 1)
    }
    const top = [...count.entries()].sort((x, y) => y[1] - x[1])[0]?.[0]
    const at = lines.findIndex((l) => /^[·•]?\s*(1st|2nd|3rd\+?|[123]\.)$/.test(l))
    const name = at > 0 ? mockSpelled(lines[at - 1]) : ''
    const named = s.people.filter((p) => name && (mockSpelled(p.name) === name || mockSpelled(handleOf(p)) === name))
    const open = Number(b.open) || 0
    const p = s.people.find((x) => top && handleOf(x) === top)
      ?? named.find((x) => x.id === open)
      ?? (named.length === 1 ? named[0] : undefined)
      ?? (named.length > 1 ? null : s.people.find((x) => x.id === open))
    if (p === null) return err(409, 'More than one person here has this name. Open the one the page belongs to and paste it again')
    if (p) return pasteOn(s, p, b)
    // Someone new is added from the page, like engine.AddPage: under the
    // link in its HTML, connected when it says 1st. The mock has no Exa
    // to ask for the link of a page pasted as text alone.
    if (at <= 0) return err(422, 'The page shows no name above how you are connected, so it is not a LinkedIn profile as the app knows one')
    if (!top) return err(422, 'The page came without the link to their profile, and Exa knows nobody by its name and headline. Paste the link to their profile, then the page again')
    const id = Math.max(...s.people.map((x) => x.id)) + 1
    const added: PersonRow = {
      id, name: lines[at - 1], headline: '', city: '', fit: 'other', first_seen: clockDate().toISOString(), notes: '',
      profiles: [{ id: 50000 + id, platform: 'linkedin', url: `https://www.linkedin.com/in/${top}`, review: 'confirmed' }], affiliations: [],
      connected: /^[·•]?\s*(1st|1\.)$/.test(lines[at]), read: '',
    }
    s.people.push(added)
    return pasteOn(s, added, b, true)
  }],
  ['GET', /^\/api\/people\/(\d+)$/, (s, m) => {
    const p = s.people.find((x) => x.id === Number(m[1]))
    return p ? ok(personDetail(s, p)) : err(404, 'No such person')
  }],
  ['PATCH', /^\/api\/people\/(\d+)$/, (s, m, _q, b) => {
    const p = s.people.find((x) => x.id === Number(m[1]))
    if (!p) return err(404, 'No such person')
    if (b.connected !== undefined && typeof b.connected !== 'boolean') return err(400, 'connected must be true or false')
    edit(s, { kind: 'person', id: p.id }, () => {
      if (typeof b.notes === 'string') p.notes = b.notes
      if (typeof b.draft === 'string') {
        p.draft = b.draft
        // An emptied message was drafted at no time, as the app says.
        if (!b.draft) p.drafted_at = undefined
      }
      if (typeof b.connected === 'boolean') p.connected = b.connected
    })
    return ok(personDetail(s, p))
  }],
  ['PATCH', /^\/api\/profiles\/(\d+)$/, (s, m, _q, b) => {
    const p = s.people.find((x) => x.profiles.some((y) => y.id === Number(m[1])))
    const prof = p?.profiles.find((x) => x.id === Number(m[1]))
    if (!p || !prof) return err(404, 'No such profile')
    if (b.review !== 'open' && b.review !== 'confirmed' && b.review !== 'rejected') return err(400, 'review must be open, confirmed or rejected')
    edit(s, { kind: 'person', id: p.id }, () => {
      p.profiles = p.profiles.map((x) => (x === prof ? { ...x, review: b.review as Profile['review'] } : x))
    })
    return { status: 204 }
  }],
  // Undo and Redo in the Edit menu, like internal/store/history.go.
  ['POST', /^\/api\/undo$/, (s) => undoStep(s, true)],
  ['POST', /^\/api\/redo$/, (s) => undoStep(s, false)],
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
  // The title bar, like chromeAPI in cmd/speakertrail-app/chrome.go, as
  // macOS answered on Tim's Mac in FrameFairy's walks: a title bar 30 high
  // and the three buttons from 12 to 72. The mock draws no buttons, but
  // the bar and the name stand where they do on the Mac.
  ['GET', /^\/api\/chrome$/, () => ok({ bar: 30, left: 12, right: 72 })],
  // Updates, like cmd/speakertrail-app/updates_api.go: a build of pull
  // request 9 that follows its channel. Check finds nothing newer. Picking
  // another channel looks, downloads it over six seconds and has it ready.
  // The app sends each change as an event, which the mock cannot, so what
  // the page shows here moves on only when it asks: on opening the page.
  ['GET', /^\/api\/updates$/, () => ok(updateState())],
  ['POST', /^\/api\/updates\/check$/, () => {
    upd.lookFrom = clockNow()
    return { status: 204 }
  }],
  ['POST', /^\/api\/updates\/follow$/, (_s, _m, _q, b) => {
    const ch = String(b.channel ?? '')
    if (!UPDATE_CHANNELS.some((c) => c.id === ch)) return err(409, `There is no channel ${ch}`)
    upd.picked = ch
    upd.lookFrom = clockNow()
    upd.downloadFrom = ch === upd.running ? 0 : clockNow() + 1200
    return ok(updateState())
  }],
  ['POST', /^\/api\/updates\/relaunch$/, () => {
    const st = updateState()
    if (st.phase !== 'ready') return err(409, 'No newer build is ready')
    return { status: 204 }
  }],
  ['POST', /^\/api\/updates\/commit$/, () => ({ status: 204 })],
  // The speech models, like speechAPI in cmd/speakertrail-app/speech_api.go.
  // A download runs for six seconds from Download and is done after. The
  // app sends where it stands as the event speech, which the mock cannot,
  // so its state is worked out from the clock whenever it is asked.
  ['GET', /^\/api\/speech$/, () => ok(speechState())],
  ['POST', /^\/api\/speech\/install$/, (_s, _m, _q, b) => {
    const st = speechState()
    if (st.job && st.job.state === 'running') return ok(st.job)
    const m = SPEECH_MODELS.find((x) => x.name === b.name)
    speechJobs++
    if (!m) {
      speechJob = { id: `speech-${speechJobs}`, kind: 'model', title: 'Speech model', state: 'failed', error: `There is no speech model called ${String(b.name)}` }
      return ok(speechJob)
    }
    speechJob = { id: `speech-${speechJobs}`, kind: 'model', title: m.title, state: 'running', from: clockNow() }
    return ok(speechState().job)
  }],
  ['POST', /^\/api\/speech\/cancel$/, (_s, _m, _q, b) => {
    const st = speechState()
    if (!st.job || st.job.state !== 'running' || st.job.id !== b.id) return err(409, `There is no download ${String(b.id)} running`)
    speechJob = { ...speechJob!, state: 'cancelled', from: undefined }
    return { status: 204 }
  }],
  ['DELETE', /^\/api\/speech\/models\/(.+)$/, (_s, m) => {
    const name = decodeURIComponent(m[1])
    if (!SPEECH_MODELS.some((x) => x.name === name)) return err(409, `There is no speech model called ${name}`)
    speechHere.delete(name)
    return { status: 204 }
  }],
  // The language models, like languageAPI in
  // cmd/speakertrail-app/languagemodels.go, the same as the speech models.
  // The largest is here from the start, so Draft writes at once.
  ['GET', /^\/api\/language$/, () => ok(languageState())],
  ['POST', /^\/api\/language\/install$/, (_s, _m, _q, b) => {
    const st = languageState()
    if (st.job && st.job.state === 'running') return ok(st.job)
    const m = LANGUAGE_MODELS.find((x) => x.name === b.name)
    languageJobs++
    if (!m) {
      languageJob = { id: `language-${languageJobs}`, kind: 'model', title: 'Language model', state: 'failed', error: `There is no language model called ${String(b.name)}` }
      return ok(languageJob)
    }
    languageJob = { id: `language-${languageJobs}`, kind: 'model', title: m.title, state: 'running', from: clockNow(), name: m.name }
    return ok(languageState().job)
  }],
  ['POST', /^\/api\/language\/cancel$/, (_s, _m, _q, b) => {
    const st = languageState()
    if (!st.job || st.job.state !== 'running' || st.job.id !== b.id) return err(409, `There is no download ${String(b.id)} running`)
    languageJob = { ...languageJob!, state: 'cancelled', from: undefined }
    return { status: 204 }
  }],
  ['DELETE', /^\/api\/language\/models\/(.+)$/, (_s, m) => {
    const name = decodeURIComponent(m[1])
    if (!LANGUAGE_MODELS.some((x) => x.name === name)) return err(409, `There is no language model called ${name}`)
    languageHere.delete(name)
    if (languageChosen === name) languageChosen = ''
    return { status: 204 }
  }],
  ['POST', /^\/api\/language\/use$/, (_s, _m, _q, b) => {
    const name = String(b.name)
    if (!LANGUAGE_MODELS.some((x) => x.name === name)) return err(409, `There is no language model called ${name}.`)
    languageChosen = name
    return ok(languageState())
  }],
  // First messages, like the drafts in internal/api/drafts.go. A draft
  // loads the model for three seconds and writes for five. The app sends
  // where it stands as the event draft, which the mock cannot, so it is
  // worked out from the clock whenever it is asked, and a walk sends the
  // event the way the app does, see walks/draft.mjs.
  // The template a first message is put together from, like
  // internal/api/template.go, with the app's checks that a walk meets.
  ['GET', /^\/api\/template$/, () => ok(templateJSON())],
  ['GET', /^\/api\/template\/example$/, (_s, _m, q) => {
    const seed = q.get('seed')
    const fields = Object.fromEntries(TEMPLATE_FIELDS.map((f) => [f.name, f.name === 'reaction_part' ? ' [reaction]' : `[${f.name}]`]))
    return ok({ text: expand(fields, 'origin', 0, seed ? seeded(Number(seed)) : Math.random) })
  }],
  ['DELETE', /^\/api\/template$/, (s) => {
    edit(s, { kind: 'template' }, () => (template = startTemplate()))
    return ok(templateJSON())
  }],
  ['PUT', /^\/api\/template\/parts\/([^/]+)$/, (s, m, _q, b) => {
    const name = decodeURIComponent(m[1])
    const parts = template.map((p) => ({ ...p }))
    const part = { name, variations: [...(b.variations as string[])] }
    const i = parts.findIndex((p) => p.name === name)
    if (i >= 0) parts[i] = part
    else parts.splice(typeof b.at === 'number' && b.at >= 0 && b.at <= parts.length ? b.at : parts.length, 0, part)
    const wrong = checkTemplate(parts)
    if (wrong) return err(400, wrong)
    edit(s, { kind: 'template' }, () => (template = parts))
    return ok(templateJSON())
  }],
  ['DELETE', /^\/api\/template\/parts\/([^/]+)$/, (s, m) => {
    const name = decodeURIComponent(m[1])
    if (!template.some((p) => p.name === name)) return err(404, `The template has no part ${name}.`)
    const why = keptWhy(name)
    if (why) return err(400, why)
    edit(s, { kind: 'template' }, () => (template = template.filter((p) => p.name !== name)))
    return ok(templateJSON())
  }],
  ['GET', /^\/api\/drafts$/, (s) => ok([...drafts.values()].map((d) => draftJob(s, d)))],
  ['POST', /^\/api\/people\/(\d+)\/draft$/, (s, m) => {
    const p = s.people.find((x) => x.id === Number(m[1]))
    if (!p) return err(404, 'Not found')
    // Without a model it is refused, as Start in internal/draft does.
    const using = languageState().models.find((x) => x.inUse)
    if (!using?.installed) return err(409, 'Download a language model in Settings first. It writes the message on this Mac.')
    const was = drafts.get(p.id)
    if (was && !['done', 'failed', 'cancelled'].includes(draftJob(s, was).state)) return ok(draftJob(s, was))
    draftJobs++
    const d: MockDraft = { id: `draft-${draftJobs}`, person_id: p.id, from: clockNow(), started_at: clockDate().toISOString() }
    drafts.set(p.id, d)
    return ok(draftJob(s, d))
  }],
  ['POST', /^\/api\/people\/(\d+)\/draft\/cancel$/, (s, m) => {
    const d = drafts.get(Number(m[1]))
    if (!d || ['done', 'failed', 'cancelled'].includes(draftJob(s, d).state)) return err(409, 'No draft is under way for them')
    d.cancelled = true
    return { status: 204 }
  }],
  // Report a Problem, like reportAPI in cmd/speakertrail-app/report_api.go.
  // The mock opens no window: the report window is the page at /report.
  // Attach picks one invented screenshot, a picture is drawn as a plain
  // SVG of its size, and Report gives the path a report would have. A
  // file pasted under a recording's ending is a screen recording, kept
  // with no picture and no size.
  ['GET', /^\/api\/report$/, () => ok(draftShown())],
  ['GET', /^\/api\/report\/window$/, () => ok({ open: false })],
  ['GET', /^\/api\/report\/picture$/, (_s, _m, q) => {
    const p = report.pictures.find((x) => x.id === q.get('id'))
    if (!p) return err(404, 'Not found')
    return {
      status: 200, type: 'image/svg+xml',
      body: `<svg xmlns="http://www.w3.org/2000/svg" width="${p.width}" height="${p.height}"><rect width="100%" height="100%" fill="#8fb3c0"/><rect x="8%" y="10%" width="84%" height="12%" rx="8" fill="#f7f9fa"/></svg>`,
    }
  }],
  // The Mac's clipboard, as cmd/speakertrail-app/clipboard.go puts text
  // on it. The walks read what was copied from the request itself.
  ['POST', /^\/api\/clipboard$/, () => ({ status: 204 })],
  ['POST', /^\/api\/report\/(open|about)$/, (_s, m, _q, b) => {
    if (m[1] === 'about') report = { text: String(b.text ?? ''), pictures: [], next: report.next, people: false }
    return { status: 204 }
  }],
  ['PUT', /^\/api\/report\/text$/, (_s, _m, _q, b) => {
    report.text = String(b.text ?? '')
    return { status: 204 }
  }],
  ['PUT', /^\/api\/report\/people$/, (_s, _m, _q, b) => {
    report.people = b.people === true
    return ok(draftShown())
  }],
  ['POST', /^\/api\/report\/pick$/, () => attachMock('Screenshot 2026-11-16 at 10.12.04.png', 1440, 900)],
  ['POST', /^\/api\/report\/pasted$/, (_s, _m, _q, b) => {
    const name = String(b.name || 'Pasted picture.png')
    const ending = /\.(mp4|mov|m4v|mkv|webm)$/i.exec(name)?.[1].toLowerCase()
    return ending ? attachMock(name, 0, 0, ending) : attachMock(name, 800, 500)
  }],
  ['POST', /^\/api\/report\/(remove|put-back)$/, (_s, m, _q, b) => {
    const p = report.pictures.find((x) => x.id === b.id)
    if (p) p.removed = m[1] === 'remove'
    return ok(draftShown())
  }],
  ['POST', /^\/api\/report\/clear$/, () => {
    report.text = ''
    for (const p of report.pictures) p.removed = true
    return ok(draftShown())
  }],
  ['POST', /^\/api\/report\/send$/, () => {
    report = { text: '', pictures: [], next: report.next, people: false }
    return ok({ path: '~/Desktop/Speaker Trail report 2026-11-16 10.15.zip' })
  }],
  // Dictate, like listenAPI. The mock hears nothing and sends no events,
  // so Stop gives one invented sentence, as if it had been said.
  ['POST', /^\/api\/listen$/, () => {
    listened++
    return ok({ recording: listened })
  }],
  ['POST', /^\/api\/listen\/stop$/, () => ok({ recording: listened, said: ['The run stopped at the third search.'], hearing: '', asked: 0 })],
  ['POST', /^\/api\/listen\/(settle|break|cancel)$/, () => ({ status: 204 })],
  // The question Cmd+Q asked taken away, like quitAPI in
  // cmd/speakertrail-app/quit.go. The question itself is the Wails event
  // called quit, which a walk sends the way the app does, through
  // window._wails.dispatchWailsEvent, see walks/quit.mjs.
  ['POST', /^\/api\/quit\/stay$/, () => ({ status: 204 })],
  // The app's log, like logAPI in cmd/speakertrail-app/applog_api.go.
  // What the window says and does is kept, so a walk can read what the
  // log would hold. Settings shows a log of two files with one toast.
  // The app sends the log as it grows as an event, which the mock cannot.
  ['POST', /^\/api\/log\/said$/, (_s, _m, _q, b) => {
    if (Array.isArray(b)) logged.said.push(...(b as { text: string }[]))
    return { status: 204 }
  }],
  ['POST', /^\/api\/log\/acted$/, (_s, _m, _q, b) => {
    logged.acts.push(b as { id: string; what: string })
    return { status: 204 }
  }],
  ['GET', /^\/api\/log\/heard$/, () => ok(logged)],
  ['GET', /^\/api\/log$/, () => ok(logStatus())],
  ['POST', /^\/api\/log\/watch$/, () => ({ status: 204 })],
  ['POST', /^\/api\/log\/show$/, () => (logGone ? err(409, 'the app keeps no log yet') : { status: 204 })],
  ['DELETE', /^\/api\/log$/, () => {
    logGone = true
    return ok(logStatus())
  }],
  ['GET', /^\/api\/exclusions$/, (s) => ok(exclusionsOf(s))],
  ['POST', /^\/api\/exclusions$/, (s, _m, _q, b) => {
    const phrase = words(String(b.phrase ?? '')).length ? String(b.phrase).split(/[^\p{L}\p{N}]+/u).filter(Boolean).join(' ') : ''
    if (!phrase) return err(400, 'Enter a word or phrase, like Online Marketing')
    if (phrase.length > 60) return err(400, 'An exclusion is at most 60 characters')
    if (s.exclusions.some((x) => x.phrase.toLowerCase() === phrase.toLowerCase())) return err(409, 'This exclusion is already there')
    const id = s.nextId.exclusion++
    edit(s, { kind: 'exclusion', id }, () => s.exclusions.push({ id, phrase }))
    return { status: 201, body: exclusionsOf(s).find((x) => x.id === id) }
  }],
  ['DELETE', /^\/api\/exclusions\/(\d+)$/, (s, m) => {
    const id = Number(m[1])
    const i = s.exclusions.findIndex((x) => x.id === id)
    if (i < 0) return err(404, 'Not found')
    edit(s, { kind: 'exclusion', id }, () => s.exclusions.splice(i, 1))
    return { status: 204 }
  }],
  ['POST', /^\/api\/sources$/, (s, _m, _q, b) => {
    const query = typeof b.query === 'string' ? b.query.trim().split(/\s+/).join(' ') : ''
    const kind = b.kind === 'profile_search' ? 'profile_search' : 'search_query'
    if (b.kind !== undefined && b.kind !== 'profile_search' && b.kind !== 'search_query') return err(400, 'kind must be search_query or profile_search')
    if (query) {
      if (s.sources.some((x) => x.kind === kind && x.query?.toLowerCase() === query.toLowerCase())) return err(409, 'That search is there already')
      const city = ['Köln', 'Düsseldorf', 'Dortmund', 'Essen', 'Duisburg', 'Bochum', 'Wuppertal', 'Bielefeld', 'Bonn', 'Münster', 'Aachen'].find((c) => query.includes(c)) ?? ''
      const m: MockSource = {
        id: s.nextId.source++, name: query, kind, url: null, query,
        category: kind === 'profile_search' ? 'Profile search' : 'Search', city, status: 'candidate', fetch_mode: 'auto', notes: '', checks: 0, empty_checks_in_row: 0, points: kind === 'profile_search' ? 2 : 0,
        health: 'never', health_note: '', discovered_from: 'Added by Tim', last_found: null, last_mode: 'http', last_error: '', last_http: 0, checked_hours_ago: null,
      }
      edit(s, { kind: 'search', id: m.id }, () => s.sources.push(m))
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
    edit(s, { kind: 'search', id: src.id }, () => {
      if (typeof b.status === 'string') src.status = b.status as SourceStatus
      if (b.fetch_mode === 'auto' || b.fetch_mode === 'http' || b.fetch_mode === 'browser') src.fetch_mode = b.fetch_mode
      if (typeof b.notes === 'string') src.notes = b.notes
      if (typeof b.name === 'string') src.name = b.name
      if (typeof b.lists === 'string') src.kind = b.lists === 'startups' ? 'portfolio' : b.lists === 'businesses' ? 'directory' : 'listing'
    })
    return ok(sourceOut(src, s))
  }],
  ['POST', /^\/api\/sources\/(\d+)\/check$/, (s, m) => {
    const src = s.sources.find((x) => x.id === Number(m[1]))
    if (!src) return err(404, 'No such source')
    // Pretend the check finishes a few seconds later.
    later(3000, () => {
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
    run.finished_at = clockDate().toISOString()
    run.state = 'stopped'
    run.progress = null
    return { status: 204 }
  }],
  ['POST', /^\/api\/runs$/, (s) => {
    const going = s.runs.find((r) => r.state === 'going' && r.kind !== 'check')
    if (going) return { status: 202, body: { run_id: going.id, started: false } }
    // Like the app: the due searches, four at a time, and the profiles of
    // links added before that still wait to be read.
    const waiting = s.people.filter((p) => p.read === 'next')
    const run = startRun(s, 'manual', 8)
    for (const p of waiting) p.read = 'waiting'
    run.progress!.reads = run.progress!.reads_expected = waiting.length
    reads.queue.push(...waiting.map((p) => ({ id: p.id, run: run.id })))
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
    // Like the app: a number is 0 or more, and whole where the setting
    // counts whole ones, which every number setting does.
    if (typeof b.value === 'number' && b.value < 0) return err(400, `${row.key} takes a number of 0 or more`)
    if (typeof b.value === 'number' && !Number.isInteger(b.value)) return err(400, `${row.key} takes a whole number`)
    if (row.key === 'new_days' && Number(b.value) < 1) return err(400, 'new_days takes 1 day or more')
    if (row.key === 'hook_prompt' && !String(b.value).includes('{{profile}}')) return err(400, 'The prompt needs {{profile}}, where the profile goes.')
    if (row.key === 'home_city' && String(b.value).trim() !== '') {
      const c = cityOf(String(b.value))
      if (!c) return err(400, `"${String(b.value).trim()}" is no city in NRW the app knows, like Köln, Bonn or Düsseldorf`)
      b.value = c
    }
    edit(s, { kind: 'setting', key: row.key }, () => {
      row.value = b.value as Json
      row.updated_at = clockDate().toISOString()
    })
    return ok(row)
  }],
  // Like the app: whether the Exa key is saved, never the key.
  ['GET', /^\/api\/keys$/, (s) => ok({ keys: [{ provider: 'exa', used: 42, budget: Number(setting(s, 'exa_monthly_searches') ?? 1000), set: exaKey !== '' }] })],
  ['PUT', /^\/api\/keys\/exa$/, (_s, _m, _q, b) => {
    exaKey = String(b.key ?? '').trim()
    return { status: 204 }
  }],
  ['GET', /^\/api\/linkedin$/, () => ok(where())],
  ['PUT', /^\/api\/linkedin$/, (s, _m, _q, b) => {
    const known = browsers.find((x) => x.app === b.app)?.profiles.some((p) => p.folder === b.folder)
    if (!known) return err(400, `There is no such profile in ${b.app}`)
    edit(s, { kind: 'setting', key: 'linkedin_opens' }, () => Object.assign(linkedin, { app: String(b.app), folder: String(b.folder) }))
    return ok(where())
  }],
  // The mock opens nothing. It answers as the app does, a moment later.
  ['POST', /^\/api\/people\/(\d+)\/linkedin$/, (s, m) => {
    const l = s.people.find((p) => p.id === Number(m[1]))?.profiles.find((x) => x.platform === 'linkedin')
    if (!l || l.review === 'rejected') return err(404, 'This person has no LinkedIn profile')
    if (!linkedin.folder) return { status: 409, body: { error: 'Choose in Settings where LinkedIn opens', choose: true } }
    return { status: 204 }
  }],
]

// The mock's Mac: Chrome with two invented profiles, nothing chosen yet.
const browsers = [{ app: 'Google Chrome', profiles: [{ folder: 'Default', name: 'Privat' }, { folder: 'Profile 1', name: 'Arbeit' }, { folder: 'Profile 2', name: 'Kunden, Projekte und alles für den Podcast' }] }]
const linkedin = { app: 'Google Chrome', folder: '' }
const where = () => ({ ...linkedin, known: browsers.some((b) => b.app === linkedin.app && b.profiles.some((p) => p.folder === linkedin.folder)), browsers })

// The mock's Exa key, saved as the app saves it in its file.
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

// The speech models the mock offers, the app's one, and which are here.
const SPEECH_MODELS = [{
  name: 'sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8', title: 'Parakeet TDT 0.6B v3',
  about: "NVIDIA's recogniser, quantised. Fast on any machine and accurate on speech.", maker: 'NVIDIA',
  languages: '25 European languages, German and English among them', download: 487_211_443, unpacked: 671_341_917,
  url: 'https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8.tar.bz2', recommended: true,
}]
// How long the mock's download takes, and then its unpacking.
const SPEECH_MS = 6000
const UNPACK_MS = 4000
const speechHere = new Set<string>()
let speechJobs = 0
let speechJob: { id: string; kind: 'model'; title: string; state: string; error?: string; from?: number } | undefined
function speechState() {
  let job: Record<string, unknown> | undefined
  if (speechJob) {
    const { from, ...rest } = speechJob
    job = rest
    if (speechJob.state === 'running' && from !== undefined) {
      const ran = clockNow() - from
      if (ran >= SPEECH_MS + UNPACK_MS) {
        speechHere.add(SPEECH_MODELS[0].name)
        speechJob = { ...rest, state: 'done' }
        job = speechJob
      } else if (ran >= SPEECH_MS) {
        const share = (ran - SPEECH_MS) / UNPACK_MS
        job = { ...rest, progress: { text: 'unpacking', fraction: share, remaining: (UNPACK_MS * (1 - share)) / 1000 } }
      } else {
        const share = ran / SPEECH_MS
        job = { ...rest, progress: { text: 'fetching', fraction: share, remaining: (SPEECH_MS * (1 - share)) / 1000 } }
      }
    }
  }
  return {
    models: SPEECH_MODELS.map((m) => ({ ...m, installed: speechHere.has(m.name), inUse: m.name === SPEECH_MODELS.find((n) => speechHere.has(n.name))?.name })),
    folder: '~/Library/Application Support/Speaker Trail/models',
    job,
  }
}

// The language models the mock offers, the app's four, and which are
// here. The Mac it pretends to be has 32 GB.
const LANGUAGE_MODELS = [
  { name: 'gemma-4-26B_q4_0-it.gguf', title: 'Gemma 4 26B A4B', maker: 'Google', about: 'Twenty six billion parameters with four of them used per token, so it runs like a far smaller model. The largest of these.', download: 14_439_363_584, needs: 16_624_595_968, url: 'https://huggingface.co/google/gemma-4-26B-A4B-it-qat-q4_0-gguf/resolve/main/gemma-4-26B_q4_0-it.gguf', recommended: true, fit: 'fits' },
  { name: 'Qwen3-14B-Q4_K_M.gguf', title: 'Qwen3 14B', maker: 'Alibaba', about: 'Fourteen billion parameters, quantised to four bits.', download: 9_001_752_960, needs: 11_417_672_064, url: 'https://huggingface.co/Qwen/Qwen3-14B-GGUF/resolve/main/Qwen3-14B-Q4_K_M.gguf', recommended: false, fit: 'fits' },
  { name: 'gemma-4-12b-it-qat-q4_0.gguf', title: 'Gemma 4 12B', maker: 'Google', about: 'Twelve billion parameters, trained to be quantised to four bits rather than cut down to them afterwards.', download: 6_975_879_296, needs: 9_693_788_288, url: 'https://huggingface.co/google/gemma-4-12B-it-qat-q4_0-gguf/resolve/main/gemma-4-12b-it-qat-q4_0.gguf', recommended: false, fit: 'fits' },
  { name: 'Ministral-3-8B-Instruct-2512-Q4_K_M.gguf', title: 'Ministral 3 8B', maker: 'Mistral', about: 'Eight billion parameters, quantised to four bits. The smallest of these, for a machine with less to spare.', download: 5_198_911_904, needs: 7_413_504_416, url: 'https://huggingface.co/mistralai/Ministral-3-8B-Instruct-2512-GGUF/resolve/main/Ministral-3-8B-Instruct-2512-Q4_K_M.gguf', recommended: false, fit: 'fits' },
] as const
const LANGUAGE_MS = 8000
const languageHere = new Set<string>([LANGUAGE_MODELS[0].name])
// The model picked in Settings, like Data.LanguageModel. Empty is none.
let languageChosen = ''
let languageJobs = 0
let languageJob: { id: string; kind: 'model'; title: string; state: string; error?: string; from?: number; name?: string } | undefined
function languageState() {
  let job: Record<string, unknown> | undefined
  if (languageJob) {
    const { from, name, ...rest } = languageJob
    job = rest
    if (languageJob.state === 'running' && from !== undefined) {
      const ran = clockNow() - from
      if (ran >= LANGUAGE_MS) {
        if (name) languageHere.add(name)
        languageJob = { ...rest, state: 'done' }
        job = languageJob
      } else {
        const share = ran / LANGUAGE_MS
        job = { ...rest, progress: { text: 'fetching', fraction: share, remaining: (LANGUAGE_MS * (1 - share)) / 1000 } }
      }
    }
  }
  const using = languageChosen || LANGUAGE_MODELS.find((m) => languageHere.has(m.name))?.name
  return {
    models: LANGUAGE_MODELS.map((m) => ({ ...m, installed: languageHere.has(m.name), inUse: m.name === using, shared: false, loaded: m.name === using && languageHere.has(m.name), loading: false })),
    folder: '~/Library/Application Support/Speaker Trail/models',
    server: true,
    // As standing in cmd/speakertrail-app/languagemodels.go judges it.
    standing: !using
      ? { mark: 'warn', why: 'Drafts need a language model. Download one in Settings' }
      : !languageHere.has(using)
        ? { mark: 'warn', why: `Drafts need ${LANGUAGE_MODELS.find((m) => m.name === using)?.title}. Download it in Settings` }
        : { mark: 'ok', why: '' },
    job,
  }
}

// The template, as the app keeps it: parts in Tim's order.
const TEMPLATE_FIELDS: TemplateField[] = [
  { name: 'first_name', by: 'app', about: 'Their first name' },
  { name: 'portrait', by: 'model', about: 'What they are, the way they show themselves, like Coach or Gründerin' },
  { name: 'reason', by: 'model', about: 'Why they caught Tim\'s eye, the part after "weil"' },
  { name: 'reaction_part', by: 'model', about: 'Nothing, or a space and a short reaction' },
]
function startTemplate(): TemplatePart[] {
  return Object.entries(firstTemplate as Record<string, string[]>)
    .filter(([name]) => !name.startsWith('_'))
    .map(([name, variations]) => ({ name, variations: [...variations] }))
}
let template = startTemplate()
function templateJSON(): FirstMessageTemplate {
  const parts = template.map((p) => ({ ...p, kept: keptWhy(p.name) || undefined }))
  return { parts, fields: TEMPLATE_FIELDS, changed: JSON.stringify(template) !== JSON.stringify(startTemplate()) }
}
// Why a part cannot be taken out, as store.Template.Removable says it.
function keptWhy(name: string): string {
  if (name === 'origin') return 'Origin is where the message starts, so it stays.'
  const user = template.find((p) => p.name !== name && p.variations.some((v) => v.includes(`#${name}#`)))
  return user ? `${user.name[0].toUpperCase()}${user.name.slice(1)} still uses #${name}#. Take it out there first.` : ''
}
// The checks of store.Template.Check a walk meets, in its words.
function checkTemplate(parts: TemplatePart[]): string {
  const names = new Set(parts.map((p) => p.name))
  for (const p of parts) {
    if (!/^[a-z][a-z0-9_]*$/.test(p.name)) return `"${p.name}" cannot name a part. Use small letters, digits and _, starting with a letter.`
    if (TEMPLATE_FIELDS.some((f) => f.name === p.name)) return `${p.name[0].toUpperCase()}${p.name.slice(1)} is filled in by the ${TEMPLATE_FIELDS.find((f) => f.name === p.name)?.by === 'model' ? 'language model' : 'app'}, so no part takes its name.`
    for (const v of p.variations) {
      for (const [, ref] of v.matchAll(/#([a-z][a-z0-9_]*)#/g)) {
        if (!names.has(ref) && !TEMPLATE_FIELDS.some((f) => f.name === ref)) return `${p.name[0].toUpperCase()}${p.name.slice(1)} uses #${ref}#, and there is no part of that name.`
      }
    }
  }
  if (!names.has('origin')) return 'The template needs a part named origin, where the message starts.'
  if (!parts.some((p) => p.variations.some((v) => v.includes('#reason#')))) return 'The template needs #reason#, where the language model\'s reason goes.'
  return ''
}
function expand(fields: Record<string, string>, name = 'origin', depth = 0, random = Math.random): string {
  if (name in fields) return fields[name]
  const p = template.find((x) => x.name === name)
  if (!p || depth > 20 || !p.variations.length) return ''
  const v = p.variations[Math.floor(random() * p.variations.length)]
  return v.replace(/#([a-z][a-z0-9_]*)#/g, (_, ref: string) => expand(fields, ref, depth + 1, random))
}

// The same picks for the same seed, as the app makes them for the example.
function seeded(seed: number): () => number {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

// Undo and Redo, like internal/store/history.go: a change Tim makes to a
// person, a search, an exclusion, the template or a setting is one step,
// the fields it changed as they were before and after. An undo puts back
// only those, and refuses when one changed since. A search or an
// exclusion added is taken out by its undo, and one removed comes back
// with its id.
type Fields = Record<string, string | undefined>
interface UndoStep {
  thing: UndoThing
  // Undefined is a thing that is not there.
  before?: Fields
  after?: Fields
}
const undoSteps = { undo: [] as UndoStep[], redo: [] as UndoStep[] }
// The drafts a paste started, by person, with when the page was kept.
const pastedDrafts = new Map<number, string | undefined>()
function fieldsOf(o: object): Fields {
  const out: Fields = {}
  for (const [k, v] of Object.entries(o)) out[k] = JSON.stringify(v)
  return out
}
// The list a person, a search or an exclusion is kept in.
function listOf(s: State, kind: 'person' | 'search' | 'exclusion'): { id: number }[] {
  return kind === 'person' ? s.people : kind === 'search' ? s.sources : s.exclusions
}
function snap(s: State, t: UndoThing): Fields | undefined {
  switch (t.kind) {
    case 'template':
      return { parts: JSON.stringify(template) }
    case 'setting': {
      if (t.key === 'linkedin_opens') return { value: JSON.stringify(linkedin) }
      const row = s.settings.find((x: Setting) => x.key === t.key)
      return row && { value: JSON.stringify(row.value) }
    }
    default: {
      const x = listOf(s, t.kind).find((y) => y.id === t.id)
      return x && fieldsOf(x)
    }
  }
}
function place(s: State, t: UndoThing, now: Fields | undefined, put: Fields | undefined) {
  const parsed = (f: Fields) => Object.fromEntries(Object.entries(f).filter(([, v]) => v !== undefined).map(([k, v]) => [k, JSON.parse(v!)]))
  if (t.kind === 'template') {
    template = JSON.parse(put!.parts!)
    return
  }
  if (t.kind === 'setting') {
    const value = JSON.parse(put!.value!)
    if (t.key === 'linkedin_opens') Object.assign(linkedin, value)
    else s.settings.find((x: Setting) => x.key === t.key)!.value = value
    return
  }
  const list = listOf(s, t.kind)
  const at = list.findIndex((y) => y.id === t.id)
  if (!put) {
    list.splice(at, 1)
    return
  }
  if (!now) {
    const i = list.findIndex((y) => y.id > t.id)
    list.splice(i < 0 ? list.length : i, 0, parsed(put) as { id: number })
    return
  }
  const row = list[at] as unknown as Record<string, unknown>
  for (const [k, v] of Object.entries(put)) {
    if (v === undefined) delete row[k]
    else row[k] = JSON.parse(v)
  }
}
function edit(s: State, t: UndoThing, fn: () => void) {
  const before = snap(s, t)
  fn()
  const after = snap(s, t)
  let step: UndoStep
  if (!before || !after) {
    if (!before && !after) return
    step = { thing: t, before, after }
  } else {
    step = { thing: t, before: {}, after: {} }
    for (const k of new Set([...Object.keys(before), ...Object.keys(after)])) {
      if (before[k] === after[k]) continue
      step.before![k] = before[k]
      step.after![k] = after[k]
    }
    if (!Object.keys(step.after!).length) return
  }
  undoSteps.undo = [...undoSteps.undo, step].slice(-200)
  undoSteps.redo = []
}
function undoStep(s: State, back: boolean) {
  const from = back ? 'undo' : 'redo'
  const to = back ? 'redo' : 'undo'
  const step = undoSteps[from].at(-1)
  if (!step) return ok({ done: false })
  const [want, put] = back ? [step.after, step.before] : [step.before, step.after]
  const now = snap(s, step.thing)
  if (!now !== !want || (want && now && Object.keys(want).some((k) => now[k] !== want[k]))) {
    undoSteps[from] = []
    return err(409, 'This has changed since, so it cannot be taken back')
  }
  place(s, step.thing, now, put)
  undoSteps[from] = undoSteps[from].slice(0, -1)
  undoSteps[to] = [...undoSteps[to], step]
  // Like draft.Drafter.Undone: an undo of the page whose paste started a
  // draft stops it.
  const t = step.thing
  const p = t.kind === 'person' ? s.people.find((x) => x.id === t.id) : undefined
  const started = p && pastedDrafts.get(p.id)
  if (p && back && started !== undefined && started !== p.text_at) {
    pastedDrafts.delete(p.id)
    const d = drafts.get(p.id)
    if (d && !['done', 'failed', 'cancelled'].includes(draftJob(s, d).state)) d.cancelled = true
  }
  return ok({ done: true, shows: step.thing })
}

// Spelled the way internal/search spells a name, without its initials
// and the numbers of a link.
function mockSpelled(x: string): string {
  return x.toLowerCase().replace(/ä/g, 'a').replace(/ö/g, 'o').replace(/ü/g, 'u').replace(/ß/g, 'ss')
    .split(/[-\s]+/).filter((w) => w && !/\d/.test(w) && w.replace('.', '').length > 1).join('')
}
const handleOf = (p: PersonRow) => (linkOf(p) ?? '').split('/').filter(Boolean).pop()?.toLowerCase() ?? ''

// Keeps a pasted page as a person's profile, like engine.Paste, a step
// Undo takes back unless the page just added them, and starts the first
// message of someone who is waiting for one, like draft.Drafter.Pasted.
function pasteOn(s: State, p: PersonRow, b: Record<string, unknown>, added = false) {
  let named = false
  let drafting = false
  // Like the app, more or less: the name is the line that spells the
  // name in the link, the headline the line after it.
  const lines = String(b.text ?? '').split('\n').map((l) => l.trim()).filter(Boolean)
  if (!lines.length) return err(400, 'The paste holds no text')
  const keep = () => {
    const handle = mockSpelled(handleOf(p))
    const at = lines.findIndex((l) => mockSpelled(l) === handle)
    named = at >= 0
    if (named) {
      p.name = lines[at]
      p.headline = lines[at + 1] ?? p.headline
      const place = lines.slice(at + 1, at + 6).find((l) => /Nordrhein-Westfalen|North Rhine/.test(l))
      if (place) p.city = place.split(',')[0]
    }
    p.text = lines.slice(Math.max(at, 0)).join('\n')
    p.text_at = clockDate().toISOString()
    if (p.read !== undefined) p.read = ''
    p.read_error = undefined
  }
  if (added) keep()
  else edit(s, { kind: 'person', id: p.id }, keep)
  // Like the app: out of the run that had not read it yet, which no
  // longer waits for it.
  for (const q of reads.queue.filter((x) => x.id === p.id)) {
    const run = s.runs.find((r) => r.id === q.run)
    if (run?.progress) run.progress.reads = run.progress.reads_expected = run.progress.reads - 1
  }
  reads.queue = reads.queue.filter((x) => x.id !== p.id)
  const last = drafts.get(p.id)
  const lastState = last ? draftJob(s, last).state : ''
  const using = languageState().models.find((x) => x.inUse)
  if (last && !['done', 'failed', 'cancelled'].includes(lastState)) drafting = true
  else if (!p.draft && p.connected && using?.installed) {
    draftJobs++
    drafts.set(p.id, { id: `draft-${draftJobs}`, person_id: p.id, from: clockNow(), started_at: clockDate().toISOString() })
    pastedDrafts.set(p.id, p.text_at)
    drafting = true
  }
  return ok({ person: personDetail(s, p), named, drafting, added })
}

// A first message being drafted, worked out from the clock.
// One that reads waits for the run's read of the profile, until readAt,
// and fails with error when Exa has no copy.
type MockDraft = {
  id: string; person_id: number; from: number; started_at: string; cancelled?: boolean; kept?: boolean
  reads?: boolean; readAt?: number; error?: string
}
const NO_COPY = 'Exa has no copy of their profile. Open it in Chrome, copy all of the page with ⌘A and ⌘C, and paste it here with ⌘V.'
const DRAFT_LOAD_MS = 3000
const DRAFT_WRITE_MS = 5000
const drafts = new Map<number, MockDraft>()
let draftJobs = 0
let draftSeq = clockNow()
// An invented message, put together from the template, with an invented
// hook, the way internal/draft does. The same picks for a whole draft.
const draftTexts = new Map<string, string>()
function draftText(id: string, name: string): string {
  let text = draftTexts.get(id)
  if (!text) {
    text = expand({ first_name: name.split(' ')[0], portrait: 'Gründerin', reason: 'du dein eigenes Studio aufgebaut hast!', reaction_part: '' })
    draftTexts.set(id, text)
  }
  return text
}
function draftJob(s: State, d: MockDraft): DraftJob {
  const p = s.people.find((x) => x.id === d.person_id)
  const using = languageState().models.find((m) => m.inUse && m.installed)
  // A number that grows with every answer, as Seq does in internal/draft.
  const base = { id: d.id, person_id: d.person_id, started_at: d.started_at, model: using?.title, seq: ++draftSeq }
  if (d.cancelled) return { ...base, state: 'cancelled', fraction: -1, seconds_left: null }
  if (!using) return { ...base, state: 'failed', error: 'Download a language model in Settings first. It writes the message on this Mac.', fraction: -1, seconds_left: null }
  if (d.error) return { ...base, state: 'failed', error: d.error, no_copy: d.error === NO_COPY, fraction: -1, seconds_left: null }
  const whole = DRAFT_LOAD_MS + DRAFT_WRITE_MS
  // Like the app: how far it is, from how long a read, a load and a
  // write take, never past 95 % until it is done.
  const expected = (d.reads ? READ_MS : 0) + whole
  const sofar = clockNow() - d.from
  const share = Math.min(sofar / expected, 0.95)
  const left = Math.max(expected - sofar, 0) / 1000
  if (d.reads && d.readAt === undefined) return { ...base, state: 'reading', fraction: share, seconds_left: left }
  const ran = clockNow() - (d.reads ? d.readAt! : d.from)
  const text = draftText(d.id, p?.name ?? 'Mara Beispiel')
  if (ran >= whole) {
    if (!d.kept && p) {
      d.kept = true
      edit(s, { kind: 'person', id: p.id }, () => {
        p.draft = text
        p.drafted_at = new Date(d.from + whole).toISOString()
      })
    }
    return { ...base, state: 'done', text, fraction: 1, seconds_left: 0 }
  }
  const fraction = d.reads ? share : ran / whole
  const secondsLeft = d.reads ? left : (whole - ran) / 1000
  if (ran < DRAFT_LOAD_MS) return { ...base, state: 'loading', fraction, seconds_left: secondsLeft }
  // While the model writes, the message is there with … for its part.
  return { ...base, state: 'writing', text: text.replace('du dein eigenes Studio aufgebaut hast!', '…').replace('Gründerin', '…'), fraction, seconds_left: secondsLeft }
}

// The draft of a report and the recordings Dictate started.
type MockPicture = { id: string; name: string; width: number; height: number; recording?: string; removed?: boolean }
let report: { text: string; pictures: MockPicture[]; next: number; people: boolean } = { text: '', pictures: [], next: 1, people: false }
let listened = 0
function draftShown() {
  return { text: report.text, people: report.people, pictures: report.pictures.filter((p) => !p.removed).map(({ removed: _, ...p }) => p) }
}
function attachMock(name: string, width: number, height: number, recording?: string) {
  if (report.pictures.filter((p) => !p.removed).length >= 10) {
    return ok({ ...draftShown(), error: `${name} cannot be attached because it would be more than the 10 pictures and recordings a report takes` })
  }
  report.pictures.push({ id: `p${report.next++}`, name, width, height, ...(recording ? { recording } : {}) })
  return ok(draftShown())
}

// What the window told the app's log, for the walks, and whether Settings
// removed the log.
const logged: { said: { text: string }[]; acts: { id: string; what: string }[] } = { said: [], acts: [] }
let logGone = false
function logStatus() {
  if (logGone) return { path: '/Users/invented/Library/Logs/Speaker Trail/app.log', files: 1, bytes: 180, lines: 1, toasts: [] }
  return {
    path: '/Users/invented/Library/Logs/Speaker Trail/app.log', files: 2, bytes: 2_430_000, lines: 9_812,
    toasts: [{ at: clockNow() - 3 * 60_000, text: 'Could not reach the app. Try again.' }],
  }
}

export function mockApi({ delayMs = DELAY_MS }: { delayMs?: number } = {}): Plugin {
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
        await new Promise((r) => setTimeout(r, delayMs))
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
