// Dev-only mock of the Speaker Trail API (docs/api.md).
// Used by `MOCK=1 npm run dev`. Never imported by the app, never built.
// State lives in memory and resets when the dev server restarts.
// The mock password is "speakertrail".

import type { Plugin } from 'vite'
import type {
  Appearance, Check, EventPerson, Fit, Json, Person, PersonDetail, PrivateEvent, Profile, PublicEvent,
  Setting, Source, SourceStatus, Stats,
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

const PASSWORD = 'speakertrail'
const SESSION = 'st_session=mock'
const DELAY_MS = 280

const dayOf = (iso: string) =>
  new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Berlin', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date(iso))

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
        .map(([organisation, role, current]) => ({ organisation, role, current })),
    }
    people.set(name, p)
    return p
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
    people: [...people.values()],
    sources,
    runs,
    checks,
    settings: buildSettings(),
    nextId: { source: 100 },
  }
}

type State = ReturnType<typeof createState>

function setting(s: State, key: string): Json | undefined {
  return s.settings.find((x) => x.key === key)?.value
}

function toPublic(e: PrivateEvent, showPeople: boolean): PublicEvent {
  const { fit: _f, fit_reason: _r, sources: _s, first_seen: _fs, people, ...rest } = e
  return { ...rest, people: showPeople ? people.map(({ name, role, affiliation }): EventPerson => ({ name, role, affiliation })) : [] }
}

function inRange(e: PrivateEvent, from: string, to: string): boolean {
  const d = dayOf(e.starts_at)
  return d >= from && d <= to
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

function personList(s: State, p: PersonRow): Person {
  const apps = appearancesOf(s, p.id)
  const now = new Date().toISOString()
  const next = apps.find((a) => a.event.starts_at >= now)
  return {
    id: p.id, name: p.name, known_as: '', headline: p.headline, city: p.city, fit: p.fit, first_seen: p.first_seen,
    appearances: apps.length,
    next_appearance: next
      ? { event_id: next.event.id, title: next.event.title, starts_at: next.event.starts_at, city: next.event.city, role: next.role }
      : null,
    profiles: p.profiles,
  }
}

function personDetail(s: State, p: PersonRow): PersonDetail {
  const seen = new Map<number, { source_id: number; source: string; checked_at: string }>()
  for (const e of s.events) {
    if (!e.people.some((x) => x.id === p.id)) continue
    for (const src of e.sources) {
      const ms = s.sources.find((x) => x.id === src.id)
      seen.set(src.id, { source_id: src.id, source: src.name, checked_at: hoursAgo(ms?.checked_hours_ago ?? 5) })
    }
  }
  const fit_evidence = p.fit === 'founder' ? `${p.name.split(' ')[0]} hat ${p.affiliations[0]?.organisation ?? 'das eigene Studio'} gegründet.` : ''
  return { ...personList(s, p), appearances: appearancesOf(s, p.id), notes: p.notes, fit_evidence, affiliations: p.affiliations, sightings: [...seen.values()] }
}

function sourceOut(m: MockSource): Source {
  const { last_found, last_mode, last_error, last_http, checked_hours_ago, ...rest } = m
  const checked = checked_hours_ago === null ? null : hoursAgo(checked_hours_ago)
  const every = m.status === 'retired' ? 28 * 24 : 24
  return {
    ...rest,
    last_checked_at: checked,
    next_check_at: checked ? hoursAgo((checked_hours_ago ?? 0) - every) : hoursAgo(-19),
    last_check: last_found === null ? null : { events_found: last_found, http_status: last_http, mode: last_mode, error: last_error },
  }
}

function stats(s: State): Stats {
  const now = new Date().toISOString()
  const upcoming = s.events.filter((e) => e.starts_at >= now)
  const kept = upcoming.filter((e) => e.fit === 'kept')
  const byStatus = { active: 0, probation: 0, candidate: 0, retired: 0, manual: 0 } as Record<SourceStatus, number>
  for (const src of s.sources) byStatus[src.status]++
  const cityCount = new Map<string, number>()
  for (const e of kept) if (e.format !== 'online') cityCount.set(e.city, (cityCount.get(e.city) ?? 0) + 1)
  // Plausible growth over 8 weeks, oldest first, Mondays.
  const today = todayBerlin()
  const monday = addDays(today, -((new Date(`${today}T12:00:00Z`).getUTCDay() + 6) % 7))
  const shape: [number, number, number][] = [[4, 3, 2], [9, 6, 5], [14, 9, 7], [22, 12, 9], [31, 18, 6], [27, 15, 8], [38, 21, 11], [29, 17, 6]]
  const weekly = shape.map(([people, events, sources], i) => ({ week_start: addDays(monday, (i - 7) * 7), people, events, sources }))
  return {
    totals: {
      people: s.people.length + 262, organisations: 140, profiles: s.people.reduce((a, p) => a + p.profiles.length, 0) + 61,
      events_upcoming: upcoming.length + 31, events_kept_upcoming: kept.length + 29, sources: byStatus,
    },
    last_7_days: { people: 41, events: 23, organisations: 12, sources: 9 },
    weekly,
    cities: [...cityCount.entries()].map(([city, events]) => ({ city, events })).sort((a, b) => b.events - a.events),
    last_run: s.runs[0] ?? null,
  }
}

// Routing

interface Out {
  status: number
  body?: unknown
  type?: string
  cookie?: string
}
type Handler = (s: State, m: RegExpMatchArray, q: URLSearchParams, body: Record<string, unknown>, loggedIn: boolean) => Out | undefined

const ok = (body: unknown) => ({ status: 200, body })
const err = (status: number, message: string) => ({ status, body: { error: message } })

const PUBLIC_ROUTES: [string, RegExp, Handler][] = [
  ['GET', /^\/api\/public\/config$/, (s, _m, _q, _b, loggedIn) => {
    const open = setting(s, 'public_calendar') === true
    const show_people = setting(s, 'public_show_people') === true
    if (!open && !loggedIn) return ok({ public_calendar: false, owner: false, show_people, cities: [] })
    const now = new Date().toISOString()
    const count = new Map<string, number>()
    for (const e of s.events) if (e.fit === 'kept' && e.format !== 'online' && e.starts_at >= now) count.set(e.city, (count.get(e.city) ?? 0) + 1)
    const cities = [...count.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c)
    return ok({ public_calendar: open, owner: loggedIn, show_people, cities })
  }],
  ['GET', /^\/api\/public\/events$/, (s, _m, q, _b, loggedIn) => {
    if (setting(s, 'public_calendar') !== true && !loggedIn) return err(404, 'The calendar is not open yet')
    const from = q.get('from') || todayBerlin()
    const to = q.get('to') || addDays(from, Number(setting(s, 'collect_ahead_days') ?? 30))
    const show = setting(s, 'public_show_people') === true
    const events = s.events
      .filter((e) => e.fit === 'kept' && e.format !== 'online' && inRange(e, from, to))
      .filter((e) => !q.get('city') || e.city === q.get('city'))
      .filter((e) => !q.get('type') || e.type === q.get('type'))
      .sort((a, b) => a.starts_at.localeCompare(b.starts_at))
      .map((e) => toPublic(e, show))
    return ok({ events })
  }],
  ['POST', /^\/api\/login$/, (_s, _m, _q, b) => {
    if (b.password !== PASSWORD) return err(401, 'Wrong password')
    return { status: 204, cookie: `${SESSION}; Path=/; HttpOnly; SameSite=Lax` }
  }],
  ['POST', /^\/api\/logout$/, () => ({ status: 204, cookie: `${SESSION}; Path=/; HttpOnly; SameSite=Lax; Max-Age=0` })],
  ['GET', /^\/api\/me$/, (_s, _m, _q, _b, loggedIn) => ok({ logged_in: loggedIn })],
]

const PRIVATE_ROUTES: [string, RegExp, Handler][] = [
  ['GET', /^\/api\/stats$/, (s) => ok(stats(s))],
  ['GET', /^\/api\/events$/, (s, _m, q) => {
    const from = q.get('from') || todayBerlin()
    const to = q.get('to') || addDays(from, Number(setting(s, 'collect_ahead_days') ?? 30))
    const fit = q.get('fit') || 'kept'
    const needle = (q.get('q') ?? '').toLowerCase()
    const events = s.events
      .filter((e) => inRange(e, from, to))
      .filter((e) => fit === 'all' || e.fit === fit)
      .filter((e) => !q.get('city') || e.city === q.get('city'))
      .filter((e) => !q.get('type') || e.type === q.get('type'))
      .filter((e) => !needle || [e.title, e.venue, e.city, ...e.organisers, ...e.people.map((p) => p.name)].join(' ').toLowerCase().includes(needle))
      .sort((a, b) => a.starts_at.localeCompare(b.starts_at))
    return ok({ events })
  }],
  ['PATCH', /^\/api\/events\/(\d+)$/, (s, m, _q, b) => {
    const e = s.events.find((x) => x.id === Number(m[1]))
    if (!e) return err(404, 'No such event')
    if (b.fit !== 'kept' && b.fit !== 'dropped') return err(400, 'fit must be kept or dropped')
    e.fit = b.fit as Fit
    e.fit_reason = typeof b.fit_reason === 'string' && b.fit_reason ? b.fit_reason : `set by hand: ${b.fit}`
    return ok(e)
  }],
  ['GET', /^\/api\/people$/, (s, _m, q) => {
    const needle = (q.get('q') ?? '').toLowerCase()
    const sort = q.get('sort') || 'next'
    const filter = q.get('filter') || 'all'
    let list = s.people.map((p) => personList(s, p))
    if (needle) list = list.filter((p) => `${p.name} ${p.headline} ${p.city}`.toLowerCase().includes(needle))
    const counts = {
      all: list.length,
      founder: list.filter((p) => p.fit === 'founder').length,
      upcoming: list.filter((p) => p.next_appearance).length,
      profile: list.filter((p) => p.profiles.some((x) => x.review !== 'rejected')).length,
    }
    if (filter === 'upcoming') list = list.filter((p) => p.next_appearance)
    if (filter === 'profile') list = list.filter((p) => p.profiles.some((x) => x.review !== 'rejected'))
    if (filter === 'founder') list = list.filter((p) => p.fit === 'founder')
    if (sort === 'name') list.sort((a, b) => a.name.localeCompare(b.name, 'de'))
    else if (sort === 'new') list.sort((a, b) => b.first_seen.localeCompare(a.first_seen) || a.name.localeCompare(b.name))
    else list.sort((a, b) => (a.next_appearance?.starts_at ?? '9999').localeCompare(b.next_appearance?.starts_at ?? '9999') || a.name.localeCompare(b.name))
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
    const list = s.sources
      .filter((x) => !status || x.status === status)
      .filter((x) => !needle || `${x.name} ${x.url ?? ''} ${x.query ?? ''} ${x.city}`.toLowerCase().includes(needle))
      .sort((a, b) => b.points - a.points || a.name.localeCompare(b.name))
      .map(sourceOut)
    return ok({ sources: list })
  }],
  ['POST', /^\/api\/sources$/, (s, _m, _q, b) => {
    const url = typeof b.url === 'string' ? b.url.trim() : ''
    if (!/^https?:\/\/\S+\.\S+/.test(url)) return err(400, 'That does not look like a web address')
    if (s.sources.some((x) => x.url === url)) return err(409, 'This source is already on the list')
    const host = new URL(url).hostname.replace(/^www\./, '')
    const m: MockSource = {
      id: s.nextId.source++, name: typeof b.name === 'string' && b.name.trim() ? b.name.trim() : host, kind: 'listing', url, query: null,
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
    return ok(sourceOut(src))
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
  ['POST', /^\/api\/runs\/(\d+)\/stop$/, (s, m) => {
    const run = s.runs.find((r) => r.id === Number(m[1]))
    if (!run) return err(404, 'No such run')
    if (run.finished_at) return err(409, 'This run has already ended')
    run.finished_at = new Date().toISOString()
    return { status: 204 }
  }],
  ['POST', /^\/api\/runs$/, (s) => {
    const going = s.runs.find((r) => !r.finished_at && r.kind !== 'check')
    if (going) return { status: 202, body: { run_id: going.id, started: false } }
    const run = {
      id: Math.max(0, ...s.runs.map((r) => r.id)) + 1, kind: 'manual' as const, started_at: new Date().toISOString(), finished_at: null as string | null,
      sources_checked: 0, events_found: 0, events_new: 0, pages_read: 0, people_new: 0, errors: 0,
    }
    s.runs.unshift(run)
    s.checks.set(run.id, [])
    // Pretend the run checks a few sources, then finishes.
    const t = setInterval(() => {
      // Stopped by hand.
      if (run.finished_at) return clearInterval(t)
      run.sources_checked += 7
      run.pages_read += 2
      run.events_found += 11
      run.events_new += 3
    }, 3000)
    setTimeout(() => {
      clearInterval(t)
      run.finished_at ??= new Date().toISOString()
    }, 12000)
    return { status: 202, body: { run_id: run.id, started: true } }
  }],
  ['GET', /^\/api\/runs\/(\d+)$/, (s, m) => {
    const run = s.runs.find((r) => r.id === Number(m[1]))
    return run ? ok({ run, checks: s.checks.get(run.id) ?? [] }) : err(404, 'No such run')
  }],
  ['GET', /^\/api\/fetches\/(\d+)\/(text|html|screenshot)$/, (s, m) => {
    const id = Number(m[1])
    const check = [...s.checks.values()].flat().find((c) => c.fetch_id === id)
    if (!check || id < 3130) return err(404, 'This fetch is older than 30 days and was deleted')
    return fetchBody(check, m[2])
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
]

function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!)
}

function fetchBody(c: Check, kind: string) {
  const title = escapeHtml(c.source.name)
  if (kind === 'text') {
    return { status: 200, type: 'text/plain; charset=utf-8', body: `${c.source.name}\n\nUpcoming events\n\nMon 18:00  Rheinland Pitch #129  Startplatz, Köln\nWed 18:30  Gateway Sundowner  InnoDom, Köln\n\n(Mock text of fetch ${c.fetch_id}, checked ${c.checked_at})\n` }
  }
  if (kind === 'html') {
    return { status: 200, type: 'text/html; charset=utf-8', body: `<!doctype html><title>${title}</title><h1>${title}</h1><p>Mock HTML of fetch ${c.fetch_id}.</p><ul><li>Rheinland Pitch #129</li><li>Gateway Sundowner</li></ul>` }
  }
  // The real server answers a PNG. An SVG stands in here.
  return {
    status: 200, type: 'image/svg+xml',
    body: `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="800" viewBox="0 0 1280 800"><rect width="1280" height="800" fill="#f3f5f8"/><rect x="0" y="0" width="1280" height="72" fill="#2448b8"/><text x="40" y="46" font-family="sans-serif" font-size="26" fill="#fff">${title}</text><text x="40" y="150" font-family="sans-serif" font-size="22" fill="#141b26">Mock screenshot of fetch ${c.fetch_id}</text>${[0, 1, 2, 3].map((i) => `<rect x="40" y="${200 + i * 120}" width="1200" height="96" rx="10" fill="#fff" stroke="#dde2ea"/>`).join('')}</svg>`,
  }
}

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
      server.config.logger.info(`\n  Mock API on. Log in with the password "${PASSWORD}".\n`)
      server.middlewares.use(async (rawReq: unknown, rawRes: unknown, next: () => void) => {
        const req = rawReq as Req
        const res = rawRes as Res
        const url = new URL(req.url ?? '/', 'http://mock')
        if (!url.pathname.startsWith('/api/')) return next()
        const body = req.method === 'POST' || req.method === 'PATCH' ? await readBody(req) : {}
        await new Promise((r) => setTimeout(r, DELAY_MS))
        const cookie = String(req.headers.cookie ?? '')
        const loggedIn = cookie.split(/;\s*/).includes(SESSION)
        const send = (out: Out) => {
          res.statusCode = out.status
          if (out.cookie) res.setHeader('Set-Cookie', out.cookie)
          if (out.status === 204 || out.body === undefined) return res.end()
          res.setHeader('Content-Type', out.type ?? 'application/json; charset=utf-8')
          res.end(typeof out.body === 'string' && out.type ? out.body : JSON.stringify(out.body))
        }
        for (const [method, re, handler] of PUBLIC_ROUTES) {
          const m = url.pathname.match(re)
          if (m && req.method === method) return send(handler(state, m, url.searchParams, body, loggedIn) ?? err(500, 'No answer'))
        }
        for (const [method, re, handler] of PRIVATE_ROUTES) {
          const m = url.pathname.match(re)
          if (!m || req.method !== method) continue
          if (!loggedIn) return send(err(401, 'Log in first'))
          return send(handler(state, m, url.searchParams, body, loggedIn) ?? err(500, 'No answer'))
        }
        send(err(404, 'Not found'))
      })
    },
  }
}
