// Where the app's updates stand, read in two places: the Updates page,
// which says it in words, and Updates in the sidebar, which wears a dot
// for it. Both read the one state kept here, so the sidebar never says
// less than the page, or something else. Ported from FrameFairy: the app
// sends where things stand as an event, updates, whenever it changes, and
// the page asks /api/updates only when it starts and after a click. See
// docs/UPDATES.md.

import { Events } from '@wailsio/runtime'
import { api, type UpdateState } from './api'

// A check that finds nothing is over in a tenth of a second, which is a
// flash nobody can read. So looking is shown for at least this long.
const LOOK_AT_LEAST = 1400

// What the line under the build starts with. A dot is work, running or
// waiting to be picked up: look pulses, new stands still. Something that
// needs a look is the triangle, warn or err. ok is the check, and idle and
// off show nothing.
export type Mark = '' | 'off' | 'look' | 'new' | 'err' | 'warn' | 'ok' | 'idle'

// A channel by what it is: main, or a pull request by its number.
export const channelName = (id: string) => (id.startsWith('pr-') ? `PR #${id.slice(3)}` : id === 'main' ? 'main' : id)

// The channel followed: the one picked, or the one this build came from.
// Empty for a build made on the Mac until a channel is chosen.
export const following = (u: UpdateState | null) => (u ? u.picked || u.follows || u.gone || u.channel || '' : '')

// The pull request followed no longer merges into main. Its builds still
// come, but it wears the colour of a warning.
export const conflicted = (u: UpdateState | null) => {
  const f = following(u)
  return !!u?.channels?.some((c) => c.id === f && c.conflict)
}

function when(checked: string | undefined, now: number): string {
  if (!checked) return ''
  const at = Date.parse(checked)
  // Go's zero time is the year 1.
  if (!isFinite(at) || at < Date.UTC(2000, 0, 1)) return ''
  if (now - at < 60_000) return 'Checked just now.'
  const t = new Date(at)
  return `Checked at ${String(t.getHours()).padStart(2, '0')}:${String(t.getMinutes()).padStart(2, '0')}.`
}

const short = (commit: string) => commit.slice(0, 7)

export function size(bytes: number): string {
  if (bytes >= 1 << 20) return `${(bytes / (1 << 20)).toFixed(1)} MB`
  return `${Math.max(1, Math.round(bytes / 1024))} KB`
}

// Where things stand, in the words of the line under the build: its mark,
// what it is in a few words, then what that means. working is the work
// Relaunch waits for, in its own words, empty when there is none.
export function standing(
  u: UpdateState | null,
  how: { working?: string; now?: number } = {},
): { mark: Mark; head: string; more: string } {
  if (!u) return { mark: '', head: '', more: '' }
  if (u.off) return { mark: 'off', head: 'This build does not update itself', more: u.off }
  const now = how.now ?? Date.now()
  const working = how.working ?? ''
  const followingName = following(u) ? channelName(following(u)) : ''
  const next = u.next ?? ''
  // A commit pushed to the channel whose build has not come yet. The
  // build there is is still offered, but never as the newest commit.
  const building = u.building ? short(u.building) : ''
  const stillBuilding = building ? ` ${building} is being built.` : ''
  switch (u.phase) {
    case 'checking':
      return { mark: 'look', head: 'Looking for a newer build', more: followingName ? `Of ${followingName}.` : '' }
    case 'downloading': {
      const part = u.total > 0 ? `${Math.floor((u.written / u.total) * 100)} % of ${size(u.total)}.` : ''
      return { mark: 'look', head: 'A newer build is downloading', more: `${next}. ${part}${stillBuilding}`.trim() }
    }
    case 'ready':
      return {
        mark: 'new',
        head: 'A newer build is ready',
        more: working
          ? `${next}. Relaunch once ${working} is done, or it goes in when you quit.${stillBuilding}`
          : `${next}. Relaunch to finish updating, or it goes in when you quit.${stillBuilding}`,
      }
    case 'failed':
      return { mark: 'err', head: 'The check did not get through', more: `${u.problem} ${when(u.checked, now)}`.trim() }
    case 'current':
      if (building)
        return {
          mark: 'look',
          head: 'A newer commit is being built',
          more: `${building}. This is the build before it, the newest of ${followingName} so far. ${when(u.checked, now)}`.trim(),
        }
      if (conflicted(u))
        return {
          mark: 'warn',
          head: 'Up to date',
          more: `This is the newest build of ${followingName}. It conflicts with main. ${when(u.checked, now)}`.trim(),
        }
      return { mark: 'ok', head: 'Up to date', more: `This is the newest build of ${followingName}. ${when(u.checked, now)}`.trim() }
    // The pull request followed was merged or closed. Nothing downloads
    // by itself: which channel to follow next is the person's to say.
    case 'gone':
      return { mark: 'warn', head: `${channelName(u.gone)} is closed`, more: 'Nothing downloads until you choose what to follow next.' }
  }
  if (!followingName) {
    return { mark: 'idle', head: 'Built on this Mac', more: 'It stays as it is until you choose what to follow.' }
  }
  return { mark: 'idle', head: 'Not checked yet', more: 'It looks when the app starts and every ten minutes.' }
}

// The dot Updates in the sidebar wears, as the classes of app.css's .dot,
// empty for none. All is well is no dot: the sidebar only speaks when
// there is something to look at.
export function railDot(u: UpdateState | null): string {
  // Work on a pull request in conflict keeps its dot, in the colour of a
  // warning, the way the page wears it.
  const warn = conflicted(u) ? ' warn' : ''
  switch (standing(u).mark) {
    case 'look':
      return `busy${warn}`
    case 'new':
      return `ready${warn}`
    case 'warn':
      return 'warn'
    case 'err':
      return 'err'
  }
  return ''
}

export class Updates {
  // What is on screen, which trails what the app last said while a check
  // is held.
  shown = $state<UpdateState | null>(null)
  #latest: UpdateState | null = null
  #lookingSince = 0
  #held: ReturnType<typeof setTimeout> | undefined
  // Events heard so far, so an answer to an asking that an event came
  // after is known to be the older of the two.
  #told = 0

  // Takes what the app said, or what a click shows at once.
  heard(u: UpdateState) {
    this.#latest = u
    if (u.phase === 'checking') {
      if (this.shown?.phase !== 'checking') this.#lookingSince = Date.now()
      clearTimeout(this.#held)
      this.shown = u
      return
    }
    // Only the looking is held. Everything else the answer brings, the
    // channel list above all, is shown at once.
    const left = LOOK_AT_LEAST - (Date.now() - this.#lookingSince)
    if (this.shown?.phase === 'checking' && left > 0) {
      clearTimeout(this.#held)
      this.shown = { ...u, phase: 'checking' }
      this.#held = setTimeout(() => this.#latest && (this.shown = this.#latest), left)
      return
    }
    this.shown = u
  }

  // Asks the app where things stand. Asking also reads the channel list,
  // and what that read found is sent as an event, which can arrive before
  // the answer to the asking does. The answer is then the older of the
  // two, so it only counts while no event has come since it was asked.
  ask() {
    const at = this.#told
    api
      .updates()
      .then((u) => this.#told === at && this.heard(u))
      .catch(() => {})
  }

  // Listens from the start of the workspace until it closes.
  listen(): () => void {
    const off = Events.On('updates', (ev) => {
      this.#told++
      this.heard(ev.data as UpdateState)
    })
    this.ask()
    return () => {
      off()
      clearTimeout(this.#held)
    }
  }
}

export const updates = new Updates()
