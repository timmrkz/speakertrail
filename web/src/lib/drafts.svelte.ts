// The first messages being drafted, the latest of each person, as the app
// says they stand. The app sends the event draft each time one moves on,
// and the list is asked once when a person opens. See internal/draft and
// docs/api.md, Drafts.
import { api, onDraft, type DraftJob } from './api'

class DraftWatch {
  // The latest draft of each person, by their id.
  jobs = $state<Record<number, DraftJob>>({})
  private users = 0
  private off: (() => void) | undefined

  async read() {
    try {
      // Taken the way a told job is, see put: the list is asked as a person
      // opens, and an end told while it was on its way is newer than what
      // it says.
      const all = await api.drafts()
      const next: Record<number, DraftJob> = { ...this.jobs }
      for (const j of all) {
        const here = next[j.person_id]
        if (!here || here.seq <= j.seq) next[j.person_id] = j
      }
      this.jobs = next
    } catch {
      // The person pane says what went wrong when a draft is asked for.
    }
  }

  // The job of one person, as the app last told it.
  of(id: number): DraftJob | undefined {
    return this.jobs[id]
  }

  // Takes a job the app answered with, so a click shows before the event,
  // or one it told. What is older than what is here is left out: Draft's
  // answer can arrive after the event that it failed, and then the draft
  // would wait for nothing, as Tim saw on 20 November 2026.
  put(j: DraftJob) {
    const here = this.jobs[j.person_id]
    if (here && here.seq > j.seq) return
    this.jobs = { ...this.jobs, [j.person_id]: j }
  }

  // Follows the event while a screen that shows drafts is open. Returns
  // what stops following, for the screen's effect to call.
  watch(): () => void {
    if (this.users++ === 0) {
      this.off = onDraft((j) => this.put(j))
    }
    this.read()
    return () => {
      if (--this.users === 0) {
        this.off?.()
        this.off = undefined
      }
    }
  }
}

export const draftWatch = new DraftWatch()

// Whether a draft is under way, not yet over.
export function underWay(j: DraftJob | undefined): j is DraftJob {
  return !!j && j.state !== 'done' && j.state !== 'failed' && j.state !== 'cancelled'
}
