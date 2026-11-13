// Where the app's updates stand, read in two places: the Updates page,
// which says it in words, and Updates in the sidebar, which wears a dot
// for it. Both read the one state kept here, so the sidebar never says
// less than the page, or something else. Ported from FrameFairy, whose
// page hears events from Go. This one asks /api/updates instead, often
// while something moves and seldom when nothing does. See
// docs/UPDATES.md.

import { api, type UpdateState } from './api'

// A check that finds nothing is over in a tenth of a second, which is a
// flash nobody can read. So looking is shown for at least this long.
const LOOK_AT_LEAST = 1400

// How often the state is asked for: while a check or a download moves,
// while the page is open, and otherwise, for the dot alone. The app
// itself looks for a newer build every ten minutes, and every twenty
// seconds while a newer commit is being built.
const MOVING_MS = 1500
const OPEN_MS = 10_000
const QUIET_MS = 60_000

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
  switch (standing(u).mark) {
    case 'look':
      return 'busy'
    case 'new':
      return 'ready'
    case 'warn':
      return 'warn'
    case 'err':
      return 'err'
  }
  return ''
}

// A commit being built is not moving here: a build takes minutes, and the
// app itself looks every twenty seconds while one is on its way, so the
// page's own pace is soon enough to hear of it.
const moving = (u: UpdateState | null) => u?.phase === 'checking' || u?.phase === 'downloading'

class Updates {
  // What is on screen, which trails what the app last said while a check
  // is held.
  shown = $state<UpdateState | null>(null)
  #latest: UpdateState | null = null
  #lookingSince = 0
  #held: ReturnType<typeof setTimeout> | undefined
  #timer: ReturnType<typeof setTimeout> | undefined
  #users = 0
  #open = 0
  #seq = 0

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

  // Asks the app where things stand, and asks again later: soon while
  // something moves, seldom when nothing does.
  async ask(): Promise<void> {
    clearTimeout(this.#timer)
    const mine = ++this.#seq
    try {
      const u = await api.updates()
      if (mine === this.#seq) this.heard(u)
    } catch {
      // Keep what is shown. The next answer puts it right.
    } finally {
      if (mine === this.#seq && this.#users > 0) {
        const wait = moving(this.#latest) ? MOVING_MS : this.#open > 0 ? OPEN_MS : QUIET_MS
        this.#timer = setTimeout(() => void this.ask(), wait)
      }
    }
  }

  // The workspace watches from its start, for the dot. The page watches
  // too, with open set, so it asks more often while it is on screen.
  watch(open = false): () => void {
    this.#users++
    if (open) this.#open++
    void this.ask()
    return () => {
      this.#users--
      if (open) this.#open--
      if (this.#users === 0) {
        clearTimeout(this.#timer)
        clearTimeout(this.#held)
      }
    }
  }
}

export const updates = new Updates()
