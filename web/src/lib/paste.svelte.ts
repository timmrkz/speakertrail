// A LinkedIn profile page Tim copied in Chrome, all of it with ⌘A and
// ⌘C, and pasted with ⌘V anywhere in the window outside a text box,
// becomes the profile of the person it belongs to, for someone Exa's
// index has no copy of. The app finds whose it is by the links Chrome
// copies with the page, else by the name on it, and the window opens
// them, so Tim never has to find them first (Tim, 24 November 2026,
// #89). Someone nobody here has is added from it, as a link adds them
// (25 November). The app reads their name, headline and city from it and, for
// someone who is waiting for a first message, starts it (#88). It never
// requests LinkedIn itself: Tim opened the page. A toast says it was
// kept, and Undo in the Edit menu takes it back, see history.svelte.ts,
// which also stops the draft it started, see draft.Drafter.Undone.
// Someone the page added is taken out by the toast's Undo, as for a
// link, until adding moves into the history too, see issue #71. The
// toast never names the person, since every toast goes into the app's
// log.

import { api } from './api'
import { draftWatch } from './drafts.svelte'
import { navigate, router } from './router.svelte'
import { said } from './said'
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
  // Counts every profile kept, so the person and the list read again.
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
    const links = pageLinks(html)
    // How the paste came, never what it said, so a report shows whether
    // Chrome's HTML came with it.
    said(`paste: a page of ${text.length} characters, ${html ? `with HTML and ${links.length} profile links` : 'as text alone'}`)
    try {
      const r = await api.pastePage({ text, links, open: openPerson() })
      const id = r.person.id
      this.changed++
      if (openPerson() !== id) navigate(`/app/people/${id}`)
      // The draft it started shows at once on the person, before the
      // app's first word of it.
      if (r.drafting) void draftWatch.read()
      const drafting = r.drafting ? ', and drafting the first message' : ''
      if (r.added) {
        // Someone new, whom Undo takes out again, as for a link.
        toast.show(`Added them from the page${drafting}`, 'plain', { label: 'Undo', run: () => void this.unadd(id, r.drafting) })
        return
      }
      if (r.named) toast.show(`Kept the page as their profile${drafting}`)
      else toast.show(`Kept the page as their profile${drafting}. Its name is not the one in their link, so check it is theirs`, 'bad')
    } catch (e) {
      toast.show(errorText(e), 'bad')
    }
  }

  // Undo of someone the page added takes them out, the draft first.
  private async unadd(id: number, drafting: boolean) {
    try {
      if (drafting) await api.cancelDraft(id).catch(() => {})
      await api.removePerson(id)
      if (openPerson() === id) navigate('/app/people')
    } catch (e) {
      fault(e)
    } finally {
      this.changed++
    }
  }
}

export const pasting = new Pasting()
