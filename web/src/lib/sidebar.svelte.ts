// The sidebar, the way FrameFairy has it: a rail of icons until the
// pointer reaches it, open over the page while the pointer is on it, and
// kept open by the button at its top. Ported from FrameFairy's App.svelte,
// with the reasons it gives, so the two apps answer the hand the same way.
// What is kept open is a view preference, so it lives in the webview, not
// in the app's file.

function read(): boolean {
  try {
    return localStorage.getItem('sidebar') === 'open'
  } catch {
    return false
  }
}

// A field being typed in has Escape for itself.
function typing(): boolean {
  const el = document.activeElement
  return !!el && (el.matches('input, textarea, select, [contenteditable]') || (el as HTMLElement).isContentEditable)
}

export class Sidebar {
  pinned = $state(read())
  // Open because the hand or the keyboard went to it.
  near = $state(false)
  open = $derived(this.pinned || this.near)
  aside: HTMLElement | undefined = undefined

  // What the sidebar knew of the pointer is out of date once the app's
  // window has been away, so it asks the next move where the pointer is.
  private asking = false
  // Where the pointer was at its last real move, so the sidebar opens on
  // the way in and not again while it stays, and closes on the way out.
  private inside = false
  private moved = false
  private lastAt = [0, 0]

  private reach(to: boolean) {
    this.near = to
  }

  // The sidebar opens when the hand moves the pointer onto it, and for
  // nothing else. WebKit hands on every mouse entered and exited macOS
  // sends as a move of the pointer, and a sidebar that opens and closes
  // under a still pointer can have the system send another, so it would
  // answer itself, open and closed over and over. Those moves move
  // nothing: they land where the move before them did. A move is the
  // hand's when it lands somewhere else. Only the very first move is
  // taken as it comes. And the sidebar only answers the pointer while the
  // app is in front, the way the Mac's own windows do.
  pointerMoved(e: PointerEvent) {
    if (this.moved && e.clientX === this.lastAt[0] && e.clientY === this.lastAt[1]) return
    if (!document.hasFocus()) return
    this.moved = true
    this.lastAt = [e.clientX, e.clientY]
    const now = !!this.aside?.contains(e.target as Node)
    const came = now && !this.inside
    const went = !now && this.inside
    this.inside = now
    if (this.asking) {
      this.asking = false
      this.reach(now)
    } else if (came) this.reach(true)
    else if (went) this.reach(false)
  }

  // The sidebar closes by the rule that opens it: a move of the hand that
  // ends outside it, or the pointer leaving the app altogether, which is a
  // leave to nothing. A leave to somewhere in the app with no move is no
  // hand: WebKit sends one when what lies under a still pointer changes.
  pointerLeft(e: PointerEvent) {
    const out = e.clientX < 0 || e.clientY < 0 || e.clientX >= innerWidth || e.clientY >= innerHeight
    if (e.relatedTarget && !out) return
    this.inside = false
    this.reach(false)
  }

  // Only the keyboard going into the sidebar opens it. Focus that comes
  // back to the button last clicked is not the person reaching for it.
  focusIn(e: FocusEvent) {
    if ((e.target as Element).matches?.(':focus-visible')) this.reach(true)
  }

  focusOut(e: FocusEvent) {
    if (!this.aside?.contains(e.relatedTarget as Node)) this.reach(false)
  }

  // Another window taking the focus takes the pointer with it, and the
  // app's window hears nothing of the pointer going. So the sidebar
  // closes, and the next move, once the app is back, says where it is.
  away() {
    this.reach(false)
    this.asking = true
  }

  // Escape closes one thing at a time: the sidebar the hover opened goes
  // first, and the person behind it only on the next press. Heard before
  // the page hears it, and marked as used so the page lets it be.
  escape(e: KeyboardEvent) {
    if (e.key !== 'Escape' || this.pinned || !this.near || typing()) return
    e.preventDefault()
    this.reach(false)
    this.asking = false
  }

  // The button keeps the sidebar open or lets it go, the one thing hover
  // cannot do, so it acts on the pin and not on whether the sidebar is
  // open. Not kept, a click keeps it, whether hovering opened it or not.
  // Kept, a click closes it at once, even with the pointer still on it,
  // and hover opens it again once the pointer has left and come back.
  toggle() {
    const keep = !this.pinned
    this.pinned = keep
    if (!keep) this.near = false
    try {
      localStorage.setItem('sidebar', keep ? 'open' : 'closed')
    } catch {
      // A webview without storage simply forgets the choice.
    }
  }
}

export const sidebar = new Sidebar()
