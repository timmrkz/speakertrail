// Undo and Redo in the Edit menu, for every change Tim makes, kept by the
// app, see internal/store/history.go. Ported from FrameFairy's undo in
// Episode.svelte. A field being typed in keeps its own undo, see
// PrivateApp.svelte. Taking something back shows what came back: the
// person it changed opens, so nothing changes where nobody is looking.
// Whatever shows it reads again when changed counts up.

import { api } from './api'
import { pasting } from './paste.svelte'
import { navigate, router } from './router.svelte'
import { errorText, toast } from './toast.svelte'

class UndoHistory {
  // Counts every step taken back or done again.
  changed = $state(0)
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
      if (done.shows?.kind === 'person') {
        const to = `/app/people/${done.shows.id}`
        if (router.path !== to) navigate(to)
        if (how === 'undo') void pasting.undone(done.shows.id)
      }
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
