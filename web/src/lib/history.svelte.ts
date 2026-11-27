// Undo and Redo in the Edit menu, for every change Tim makes, kept by the
// app, see internal/store/history.go. Ported from FrameFairy's undo in
// Episode.svelte. A field being typed in keeps its own undo, see
// PrivateApp.svelte. Taking something back shows what came back, so
// nothing changes where nobody is looking: the person it changed opens,
// Searches for a search or an exclusion, Settings for the template or a
// setting. Whatever shows it reads again when changed counts up, and the
// part of the page that shows the thing brings it into view, by reveal,
// whether it was open already or opens for it.

import { api, type UndoThing } from './api'
import { navigate, router } from './router.svelte'
import { errorText } from './problem.svelte'
import { toast } from './toast.svelte'

// The page that shows a thing an undo changed.
function pageOf(t: UndoThing): string {
  switch (t.kind) {
    case 'person':
      return `/app/people/${t.id}`
    case 'search':
    case 'exclusion':
      return '/app/searches'
    case 'template':
    case 'setting':
      return '/app/settings'
  }
}

class UndoHistory {
  // Counts every step taken back or done again.
  changed = $state(0)
  // What the last step taken back or done again changed, until the part
  // of the page that shows it has brought it into view.
  private toShow = $state<UndoThing | null>(null)

  // reveal gives what the last step changed when it is what match looks
  // for and not yet in view, and counts it in view from then on. The part
  // of the page that shows it calls it in an effect, which runs again
  // after each step, and as the part comes on screen when a step opened
  // its page.
  reveal(match: (t: UndoThing) => boolean): UndoThing | null {
    const t = this.toShow
    if (!t || !match(t)) return null
    this.toShow = null
    return t
  }
  // One at a time. A key held down would otherwise ask for the same step
  // twice before the first answer is back.
  private busy = false

  // step says whether it took a step.
  async step(how: 'undo' | 'redo'): Promise<boolean> {
    // Nothing changes behind the box asking something.
    if (this.busy || document.querySelector('dialog[open]')) return false
    this.busy = true
    try {
      const done = how === 'undo' ? await api.undo() : await api.redo()
      if (!done.done) return false
      if (done.shows) {
        const to = pageOf(done.shows)
        if (router.path !== to) navigate(to)
      }
      this.toShow = done.shows ?? null
      this.changed++
      return true
    } catch (e) {
      // Undo and Redo in the Edit menu have no control of their own, so a
      // toast says what went wrong (Tim, 27 November 2026, #72).
      toast.show(errorText(e), 'bad')
      return false
    } finally {
      this.busy = false
    }
  }
}

export const undoHistory = new UndoHistory()

// bringIntoView scrolls el into view, and keeps it there while what loads
// above it pushes it down, until the person scrolls, clicks or types, or
// el goes. Settings loads its parts one by one, and the row of a setting
// an undo opened Settings for went out of view as the template above it
// came in.
export function bringIntoView(el: HTMLElement, block: ScrollLogicalPosition = 'center') {
  let scroller = el.parentElement
  while (scroller && !/(auto|scroll)/.test(getComputedStyle(scroller).overflowY)) scroller = scroller.parentElement
  const content = scroller?.firstElementChild ?? document.documentElement
  const put = () => {
    if (el.isConnected) el.scrollIntoView({ block })
    else stop()
  }
  const held = new ResizeObserver(put)
  const ways = ['wheel', 'pointerdown', 'keydown', 'touchstart'] as const
  function stop() {
    held.disconnect()
    for (const w of ways) removeEventListener(w, stop, true)
  }
  for (const w of ways) addEventListener(w, stop, true)
  held.observe(content)
  put()
}
