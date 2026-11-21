// Who a key belongs to, ported from FrameFairy's lib/keys.ts, the part
// the report window needs.

// A field being typed in holds the keyboard.
export function typing(): boolean {
  const on = document.activeElement as HTMLElement | null
  const tag = on?.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || !!on?.isContentEditable
}

// The box over the page is asking something, see Confirm.svelte.
export function asking(): boolean {
  return !!document.querySelector('dialog[open], [role="alertdialog"]')
}

// A key that moves the focus shows where it went. Where macOS would not
// move it, Tab to a button, the page moves it itself, and WebKit draws no
// ring on a focus a script moved when the focus before it came from a
// click. Tim clicked into the report, pressed Tab, and Report had the
// focus with nothing to show it. The move asks for it.
export function keyFocus(to: HTMLElement) {
  to.focus({ focusVisible: true } as FocusOptions)
}

// What Tab reaches inside root, in the order the page is written. A
// button greyed out with aria-disabled stays in it, one with disabled or
// tabindex -1 does not, nor anything not drawn, like the inside of a
// closed details.
export function tabStops(root: ParentNode): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>('a[href], button, input, textarea, select, [tabindex]')].filter(
    (e) => e.tabIndex >= 0 && !(e as HTMLButtonElement).disabled && e.checkVisibility(),
  )
}

// Tab and Shift+Tab inside root, round from the last stop to the first.
// macOS only tabs between buttons when full keyboard access is on, so a
// window, a sheet or a question moves the focus itself, and this is the
// one place that does it. It gives where the focus went, or nothing when
// root has no stop.
export function moveTab(root: ParentNode, e: KeyboardEvent): HTMLElement | null {
  const stops = tabStops(root)
  if (!stops.length) return null
  e.preventDefault()
  const here = stops.indexOf(document.activeElement as HTMLElement)
  const step = e.shiftKey ? -1 : 1
  const next = here < 0 ? stops[step > 0 ? 0 : stops.length - 1] : stops[(here + step + stops.length) % stops.length]
  keyFocus(next)
  return next
}
