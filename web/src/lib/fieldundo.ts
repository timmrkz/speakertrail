import { edit } from './dictating'
import { Undo } from './undo'

// Undo and Redo of every text field, kept by the page. The browser's own
// undo knows nothing of text the page puts in, and WebKit keeps one undo
// for the whole page: Tim typed notes, took them back with Cmd+Z, clicked
// into the notes and pressed Cmd+Y, which did nothing, and more Cmd+Z and
// Cmd+Y put his words in two and three times (testing #87, 25 November
// 2026). Report a Problem's field learned the same, see lib/undo.ts. So
// the browser's undo is never asked: every field keeps its own steps,
// and a historyUndo or historyRedo the browser starts is turned away.
//
// Text the page put in since the last step is no step, and the steps
// before it name letters of another text, so they go. While a field
// shows the text the page put in, with no step of its own to take, it
// hands Cmd+Z and Cmd+Y on to the app, see lib/history.svelte.ts, so Cmd+Y
// in the notes does again what Cmd+Z took back while they were left.
// Once something is typed in it, the field keeps them, so the app never
// writes over words not saved yet.

type Field = HTMLTextAreaElement | HTMLInputElement

class Kept {
  private steps = new Undo()
  // The text as the last step left it, and as the page last put it in.
  private seen: string
  private put: string
  private alone = false

  constructor(private field: Field) {
    this.seen = this.put = field.value
  }

  // fresh forgets the steps when the page put other text in since.
  private fresh() {
    if (this.field.value === this.seen) return
    this.steps = new Undo()
    this.seen = this.put = this.field.value
  }

  // before comes before an edit, and when the focus comes in, since a
  // paste the page puts in with execCommand, see pastedlines.ts, comes
  // with no beforeinput.
  before(e?: InputEvent) {
    this.fresh()
    if (!e) return
    // A paste or a drop is a step of its own, as on a Mac.
    this.alone = e.inputType.startsWith('insertFrom')
  }

  input() {
    const was = this.seen
    const now = this.field.value
    if (now === was) return
    const [a, b, put] = edit(was, now, this.field.selectionEnd ?? now.length)
    this.steps.did(a, b, was.slice(a, b), put, this.alone)
    this.alone = false
    this.seen = now
  }

  // step takes the field's last step back, or does again the last one
  // taken back, with the cursor after it. It says whether the field
  // keeps the key: when it took a step, or holds typing of its own.
  step(how: 'undo' | 'redo'): boolean {
    this.fresh()
    const c = how === 'undo' ? this.steps.undo(this.field.value) : this.steps.redo(this.field.value)
    if (!c) return this.field.value !== this.put
    this.field.setRangeText(c[2], c[0], c[1], 'end')
    this.seen = this.field.value
    // The field's owner hears of it as of any edit.
    this.field.dispatchEvent(new Event('input', { bubbles: true }))
    return true
  }
}

const kept = new WeakMap<Element, Kept>()

// A field that takes text, with a cursor in it. A number or a date has
// none, and keeps no steps.
function textField(on: EventTarget | null): Field | null {
  if (!(on instanceof HTMLTextAreaElement || on instanceof HTMLInputElement)) return null
  try {
    return on.selectionStart === null ? null : on
  } catch {
    return null
  }
}

function keeper(field: Field): Kept {
  let k = kept.get(field)
  if (!k) {
    k = new Kept(field)
    kept.set(field, k)
  }
  return k
}

// keepUndo gives every text field in the page its own steps, from the
// first edit made in it. A historyUndo or historyRedo the browser starts,
// from its own menu or keys, goes to onkey, which asks the field in hand
// first, the way the Edit menu does. It gives a function that stops it.
export function keepUndo(onkey: (how: 'undo' | 'redo') => void): () => void {
  const before = (e: Event) => {
    const ev = e as InputEvent
    if (ev.inputType === 'historyUndo' || ev.inputType === 'historyRedo') {
      ev.preventDefault()
      onkey(ev.inputType === 'historyUndo' ? 'undo' : 'redo')
      return
    }
    const field = textField(ev.target)
    if (field) keeper(field).before(ev)
  }
  const input = (e: Event) => {
    const field = textField(e.target)
    if (field) kept.get(field)?.input()
  }
  const focus = (e: Event) => {
    const field = textField(e.target)
    if (field) keeper(field).before()
  }
  document.addEventListener('beforeinput', before, true)
  document.addEventListener('input', input, true)
  document.addEventListener('focusin', focus, true)
  return () => {
    document.removeEventListener('beforeinput', before, true)
    document.removeEventListener('input', input, true)
    document.removeEventListener('focusin', focus, true)
  }
}

// fieldStep takes Undo or Redo in the field being typed in, and says
// whether the field keeps the key. A field with nothing of its own to
// take passes it on.
export function fieldStep(on: Element | null, how: 'undo' | 'redo'): boolean {
  const field = textField(on)
  return field ? keeper(field).step(how) : false
}
