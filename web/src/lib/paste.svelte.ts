// A LinkedIn profile page Tim copied in Chrome, all of it with ⌘A and
// ⌘C, and pasted with ⌘V anywhere in the window outside a text box,
// becomes the profile of the person it belongs to, for someone Exa's
// index has no copy of. The app finds whose it is by the links Chrome
// copies with the page, else by the name on it, and the window opens
// them, so Tim never has to find them first (Tim, 24 November 2026,
// #89). The app reads their name, headline and city from it and, for
// someone who is waiting for a first message, starts it (#88). It never
// requests LinkedIn itself: Tim opened the page. A toast says it was
// kept, with Undo. The toast never names the person, since every toast
// goes into the app's log.

import { api, type Pasted } from './api'
import { draftWatch } from './drafts.svelte'
import { navigate, router } from './router.svelte'
import { errorText, fault, toast } from './toast.svelte'

// What someone Exa has no copy of says, on their row and when they are
// open: what to do, not why (Tim, 23 November 2026).
export const PASTE_LINE = 'Paste their LinkedIn page'
export const PASTE_HINT = 'Exa has no copy of their profile. Open it in Chrome, copy all of the page with ⌘A and ⌘C, and paste it here with ⌘V.'

// A page is text over more than one line, most of them without a
// profile link. Links one to a line are links, and go into the search.
// A page may hold a link in its About, and is a page all the same.
export function isPage(text: string): boolean {
  const lines = text.split('\n').map((l) => l.trim()).filter(Boolean)
  const linked = lines.filter((l) => /linkedin\.com\/in\//i.test(l)).length
  return lines.length > 1 && linked * 2 < lines.length
}

// The links in the HTML Chrome copies with a page's text. The app tells
// whose page it is by them, see engine.Whose. Only their addresses go,
// never the HTML.
export function pageLinks(html: string): string[] {
  if (!html) return []
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const out: string[] = []
  for (const a of doc.querySelectorAll('a[href]')) {
    const href = a.getAttribute('href') ?? ''
    if (href.includes('/in/')) out.push(href)
    if (out.length >= 5000) break
  }
  return out
}

// The person open on People, or 0.
function openPerson(): number {
  const m = router.match('/app/people/:id')
  return m ? Number(m.id) : 0
}

class Pasting {
  // Counts every profile kept or put back, so the person and the list
  // read again.
  changed = $state(0)

  // Takes a paste from anywhere in the window. A page in a text box,
  // like the notes, stays there as text, and so does what is no page.
  // It says whether it took it.
  take(e: ClipboardEvent, inField: boolean): boolean {
    if (inField) return false
    const text = e.clipboardData?.getData('text/plain')?.trim() ?? ''
    if (!isPage(text)) return false
    e.preventDefault()
    e.stopPropagation()
    void this.page(text, e.clipboardData?.getData('text/html') ?? '')
    return true
  }

  async page(text: string, html: string): Promise<void> {
    try {
      const r = await api.pastePage({ text, links: pageLinks(html), open: openPerson() })
      const id = r.person.id
      this.changed++
      if (openPerson() !== id) navigate(`/app/people/${id}`)
      // The draft it started shows at once on the person, before the
      // app's first word of it.
      if (r.drafting) void draftWatch.read()
      const undo = { label: 'Undo', run: () => void this.undo(id, r.was, r.drafting) }
      const drafting = r.drafting ? ', and drafting the first message' : ''
      if (r.named) toast.show(`Kept the page as their profile${drafting}`, 'plain', undo)
      else toast.show(`Kept the page as their profile${drafting}. Its name is not the one in their link, so check it is theirs`, 'bad', undo)
    } catch (e) {
      toast.show(errorText(e), 'bad')
    }
  }

  // Undo puts back what they were, and stops the draft the paste
  // started, if it still runs.
  private async undo(id: number, was: Pasted, drafting: boolean) {
    try {
      if (drafting) await api.cancelDraft(id).catch(() => {})
      await api.pasteProfile(id, { as: was })
    } catch (e) {
      fault(e)
    } finally {
      this.changed++
    }
  }
}

export const pasting = new Pasting()
