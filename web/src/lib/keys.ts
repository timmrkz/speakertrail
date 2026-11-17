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
