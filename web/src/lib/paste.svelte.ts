// A LinkedIn profile page Tim copied in Chrome, all of it with ⌘A and
// ⌘C, and pasted with ⌘V while a person is open on People, becomes that
// person's profile, for someone Exa's index has no copy of. The app reads
// their name, headline and city from it and writes the first message from
// it. It never requests LinkedIn itself: Tim opened the page. A toast
// says it was kept, with Undo. The toast never names the person, since
// every toast goes into the app's log.

import { api, type Pasted } from './api'
import { errorText, fault, toast } from './toast.svelte'

// How Tim gives the app a profile Exa lacks, said under the pointer.
export const PASTE_HINT = 'Open them here, open their profile in Chrome, copy all of the page with ⌘A and ⌘C, and paste it here with ⌘V'

// A page is text over more than one line with no profile link in it. A
// search is one line, and links go into the search.
export function isPage(text: string): boolean {
  return text.includes('\n') && !/linkedin\.com\/in\//i.test(text)
}

class Pasting {
  // Counts every profile kept or put back, so the person and the list
  // read again.
  changed = $state(0)
  // The person whose page is being kept now, or null.
  busy = $state<number | null>(null)

  async paste(id: number, text: string): Promise<void> {
    this.busy = id
    try {
      const r = await api.pasteProfile(id, { text })
      this.changed++
      const undo = { label: 'Undo', run: () => void this.undo(id, r.was) }
      if (r.named) toast.show('Kept the page as their profile', 'plain', undo)
      else toast.show('Kept the page as their profile. It does not give their name, so check it is theirs', 'bad', undo)
    } catch (e) {
      toast.show(errorText(e), 'bad')
    } finally {
      this.busy = null
    }
  }

  private async undo(id: number, was: Pasted) {
    try {
      await api.pasteProfile(id, { as: was })
    } catch (e) {
      fault(e)
    } finally {
      this.changed++
    }
  }
}

export const pasting = new Pasting()
