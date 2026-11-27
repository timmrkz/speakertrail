// Who a key belongs to, ported from FrameFairy's lib/keys.ts, the part
// the report window needs.

// A field being typed in holds the keyboard.
export function typing(): boolean {
  const on = document.activeElement as HTMLElement | null
  const tag = on?.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || !!on?.isContentEditable
}

// Escape in a field of something open, like a person's notes, steps out
// one level: the field lets go of the keyboard, which saves it as leaving
// it always does, and the keyboard goes to the sheet the field is in, or
// else to the row the thing stands on in its list, so the arrow keys go
// on from there. The next Escape puts the thing away, from there. One
// Escape never does both, since on a Mac Escape also ends Dictation, and
// an Escape meant for that would close what was being dictated into.
// What was typed always stays. Issue #85.
export function stepOut(e: KeyboardEvent, row: string) {
  if (e.key !== 'Escape' || e.defaultPrevented || e.isComposing || e.metaKey || e.ctrlKey || e.altKey) return
  const field = e.currentTarget as HTMLElement
  e.preventDefault()
  const to = field.closest<HTMLElement>('[role="dialog"]') ?? document.getElementById(row)
  if (to) keyFocus(to)
  if (document.activeElement === field) field.blur()
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
