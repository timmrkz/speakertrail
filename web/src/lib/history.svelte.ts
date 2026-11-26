// Undo and Redo in the Edit menu, for every change Tim makes, kept by the
// app, see internal/store/history.go. Ported from FrameFairy's undo in
// Episode.svelte. A field being typed in keeps its own undo, see
// PrivateApp.svelte. Taking something back shows what came back, so
// nothing changes where nobody is looking: the person it changed opens,
// Searches for a search or an exclusion, Settings for the template or a
// setting. Whatever shows it reads again when changed counts up.

import { api, type UndoThing } from './api'
import { navigate, router } from './router.svelte'
import { errorText, toast } from './toast.svelte'

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
  // What the last step taken back or done again changed, so the part of
  // the page that shows it can open, like the exclusions.
  shows = $state<UndoThing | null>(null)
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
      this.shows = done.shows ?? null
      this.changed++
      return true
    } catch (e) {
      toast.show(errorText(e), 'bad')
      return false
    } finally {
      this.busy = false
    }
  }
}

export const undoHistory = new UndoHistory()
