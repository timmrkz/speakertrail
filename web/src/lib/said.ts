// What the window says, written into the app's own log on the Go side,
// ~/Library/Logs/Speaker Trail/app.log on a Mac. The webview keeps its
// console to itself, so without this a fault on Tim's Mac leaves nothing
// to read. Ported from FrameFairy's lib/said.ts, see docs/LOGGING.md.
//
// It says enough that a fault is found by reading it, not by asking Tim
// to try again: every click and key as an act, every call to the Go side
// that fails or is slow, every act slow to reach the screen, and every
// error the window meets. Each line has its level.
//
// It never says a person's name, headline, link or notes: a control
// inside what shows a person is named by its kind, or by a button's own
// words, never by what is written on it. See control below.

import { api, tellTo } from './api'

// error: something failed. warn: something went wrong and the app carried
// on. info: what the person did, and what started. debug: the steps
// between. trace: what comes with every step of something that repeats,
// kept in memory, not in the file.
export type Level = 'error' | 'warn' | 'info' | 'debug' | 'trace'
export type Said = { level?: Level; toast?: boolean }

type Line = { at: number; text: string; act?: string } & Said

// The person's last act: a click or a key. Every line said after it
// carries its id, and so does every line the Go side writes for it, so
// what one act set off is read together. See docs/LOGGING.md.
let acts = 0
let acting = ''

// Starts an act: says what the person did, at once, and gives its id.
export function act(what: string): string {
  acting = `a${(++acts).toString(36)}`
  api.acted({ id: acting, at: Date.now(), what }).catch(() => {})
  return acting
}

let waiting: Line[] = []
let timer: ReturnType<typeof setTimeout> | null = null
let last = ''
let repeats = 0
let repeatedAt = 0

// Most lines kept between two writes: a loop gone wrong says so, and does
// not fill the disk.
const MOST = 400

// Adds a line to the log, written within a quarter of a second with what
// came with it, stamped with the moment it was said. It is a debug line
// unless it says otherwise. A line said again straight after itself is
// counted, not written again.
export function said(text: string, how: Said = {}) {
  const at = Date.now()
  const key = `${acting} ${how.level ?? ''} ${how.toast ?? ''} ${text}`
  if (key === last) {
    repeats++
    repeatedAt = at
    return
  }
  if (repeats) waiting.push({ at: repeatedAt, text: `(said ${repeats} more times)` })
  last = key
  repeats = 0
  waiting.push({ at, text, act: acting || undefined, ...how })
  if (waiting.length > MOST) waiting.splice(0, waiting.length - MOST)
  timer ??= setTimeout(write, 250)
}

function write() {
  timer = null
  if (repeats) waiting.push({ at: repeatedAt, text: `(said ${repeats} more times)` })
  repeats = 0
  last = ''
  const lines = waiting
  waiting = []
  if (lines.length) api.said(lines).catch(() => {})
}

// Anything as text for a line of the log.
export function text(args: unknown[]): string {
  return args
    .map((a) => {
      if (a instanceof Error) return `${a.message}${a.stack ? `\n${a.stack}` : ''}`
      if (typeof a === 'string') return a
      try {
        return JSON.stringify(a)
      } catch {
        return String(a)
      }
    })
    .join(' ')
}

// How long an act may take to reach the screen before it is said, in ms.
// A tenth of a second is where a click starts to feel slow. So each act
// says how long it took from the hand to the first frame drawn after it,
// and how much of that the window was still busy with something else
// before it heard the act at all, which tells work in the window from
// work it waits on.
const SLOWLY = 150

function shown(what: string, e: Event) {
  const heard = performance.now()
  requestAnimationFrame(() =>
    setTimeout(() => {
      const took = performance.now() - e.timeStamp
      if (took <= SLOWLY || document.hidden) return
      said(`window: ${what} took ${Math.round(took)} ms to reach the screen, ${Math.round(heard - e.timeStamp)} ms of it before the window heard it`, { level: 'warn' })
    }),
  )
}

// The control an act was on, by what the interface calls it. Inside what
// shows a person, marked data-personal, only a button is named, by its
// label or its own words, which are ours. A row of People, a link or a
// line of text there would be named by a person's name or words, so it
// is named by its kind alone. FrameFairy names a word clicked in the
// caption box as the caption box the same way, so its log holds no
// transcript.
export function control(target: Element | null): string {
  const el = target?.closest<HTMLElement>('button, a, input, textarea, select, label, [role], [contenteditable], [title], [aria-label]') ?? (target as HTMLElement | null)
  if (!el || el === document.body) return 'nothing'
  const personal = !!el.closest('[data-personal]')
  const labelled = el.matches('button, a, label, [role=button], [role=menuitem], [role=option], [role=tab], [role=checkbox], [role=switch]')
  const words = (el.textContent ?? '').trim().slice(0, 30)
  const name = personal
    ? el.tagName === 'BUTTON' ? el.getAttribute('aria-label') || words : ''
    : el.getAttribute('aria-label') || el.title || (labelled ? words : '')
  const kind = typeof el.className === 'string' ? el.className.split(' ').find((c) => c && !c.startsWith('svelte-')) : ''
  return `${el.tagName.toLowerCase()}${kind ? `.${kind}` : ''}${name ? ` "${name}"` : ''}`
}

// Whether a key is typing into a field, which is not an act of its own:
// the field the person typed into is, once, when they clicked or tabbed
// into it.
function typing(e: KeyboardEvent): boolean {
  const el = e.target as HTMLElement | null
  const field = !!el && (el.isContentEditable || el.tagName === 'INPUT' || el.tagName === 'TEXTAREA')
  return field && !e.metaKey && !e.ctrlKey && (e.key.length === 1 || ['Backspace', 'Delete', 'ArrowLeft', 'ArrowRight', 'Shift'].includes(e.key))
}

// A key as the person pressed it: ⌘Z, Space, ArrowLeft.
function pressed(e: KeyboardEvent): string {
  const key = e.key === ' ' ? 'Space' : e.key
  return `${e.metaKey ? '⌘' : ''}${e.ctrlKey ? '⌃' : ''}${e.altKey ? '⌥' : ''}${e.shiftKey && key.length > 1 ? '⇧' : ''}${key}`
}

// The console's warnings and errors, and whatever is thrown and not
// caught, go to the log as well as where they went. So does the window
// going hidden and shown. Every click and every key that is not typing is
// an act.
export function listen() {
  window.addEventListener(
    'pointerdown',
    (e) => {
      const what = `pressed ${control(e.target as Element | null)}`
      act(what)
      shown(what, e)
    },
    true,
  )
  window.addEventListener(
    'keydown',
    (e) => {
      if (e.repeat || ['Shift', 'Meta', 'Control', 'Alt', 'CapsLock'].includes(e.key) || typing(e)) return
      const what = `${pressed(e)} on ${control(document.activeElement)}`
      act(what)
      shown(what, e)
    },
    true,
  )
  tellTo((line) => said(line, { level: 'warn' }))
  const warn = console.warn.bind(console)
  const error = console.error.bind(console)
  console.warn = (...args: unknown[]) => {
    warn(...args)
    said(text(args), { level: 'warn' })
  }
  console.error = (...args: unknown[]) => {
    error(...args)
    said(text(args), { level: 'error' })
  }
  window.addEventListener('error', (e) => said(`uncaught: ${text([e.error ?? e.message])}`, { level: 'error' }))
  window.addEventListener('unhandledrejection', (e) => said(`unhandled: ${text([e.reason])}`, { level: 'error' }))
  said(`window: started, ${navigator.userAgent}, ${window.devicePixelRatio}x, ${innerWidth}x${innerHeight}`, { level: 'info' })
  document.addEventListener('visibilitychange', () => {
    said(`window: ${document.hidden ? 'hidden' : 'shown'}`)
  })
}
