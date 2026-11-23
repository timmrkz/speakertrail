// A LinkedIn profile page Tim copied in Chrome, all of it with ⌘A and
// ⌘C, and pasted with ⌘V while a person is open on People, becomes that
// person's profile, for someone Exa's index has no copy of. The app reads
// their name, headline and city from it and writes the first message from
// it. It never requests LinkedIn itself: Tim opened the page. A toast
// says it was kept, with Undo. The toast never names the person, since
// every toast goes into the app's log.

import { api, type Pasted } from './api'
import { errorText, fault, toast } from './toast.svelte'

// What someone Exa has no copy of says, on their row and when they are
// open: what to do, not why (Tim, 23 November 2026).
export const PASTE_LINE = 'Paste their LinkedIn page'
export const PASTE_HINT = 'Exa has no copy of their profile. Open it in Chrome, copy all of the page with ⌘A and ⌘C, and paste it here with ⌘V.'

// A page is text over more than one line with no profile link in it. A
// search is one line, and links go into the search.
export function isPage(text: string): boolean {
  return text.includes('\n') && !/linkedin\.com\/in\//i.test(text)
}

class Pasting {
  // Counts every profile kept or put back, so the person and the list
  // read again.
  changed = $state(0)
  async paste(id: number, text: string): Promise<void> {
    try {
      const r = await api.pasteProfile(id, { text })
      this.changed++
      const undo = { label: 'Undo', run: () => void this.undo(id, r.was) }
      if (r.named) toast.show('Kept the page as their profile', 'plain', undo)
      else toast.show('Kept the page as their profile. Its name is not the one in their link, so check it is theirs', 'bad', undo)
    } catch (e) {
      toast.show(errorText(e), 'bad')
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
