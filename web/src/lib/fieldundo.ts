import { edit } from './dictating'
import { Undo } from './undo'

// Undo and Redo of a text field, kept by the page, for every field whose
// text the app also puts in, like the notes after Undo in the Edit menu
// took them back. The browser's own undo knows nothing of text the page
// puts in: Tim typed notes, took them back with Cmd+Z, clicked into the
// notes and pressed Cmd+Y, which did nothing, and more Cmd+Z and Cmd+Y
// put his words in two and three times (testing #87, 25 November 2026).
// Report a Problem's field learned the same, see lib/undo.ts.
//
// Text the page put in since the last step is no step, and the steps
// before it name letters of another text, so they go. A field with no
// step to take hands Cmd+Z and Cmd+Y on to the app, see
// lib/history.svelte.ts, so Cmd+Y in the notes does again what Cmd+Z
// took back while they were left.

type Field = HTMLTextAreaElement | HTMLInputElement

class Kept {
  private steps = new Undo()
  // The text as the last step left it.
  private seen: string
  private alone = false

  constructor(private field: Field) {
    this.seen = field.value
  }

  // fresh forgets the steps when the page put other text in since.
  private fresh() {
    if (this.field.value === this.seen) return
    this.steps = new Undo()
    this.seen = this.field.value
  }

  before(e: InputEvent) {
    if (e.inputType === 'historyUndo' || e.inputType === 'historyRedo') {
      e.preventDefault()
      this.step(e.inputType === 'historyUndo' ? 'undo' : 'redo')
      return
    }
    this.fresh()
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
  // taken back, with the cursor after it. It says whether there was one.
  step(how: 'undo' | 'redo'): boolean {
    this.fresh()
    const c = how === 'undo' ? this.steps.undo(this.field.value) : this.steps.redo(this.field.value)
    if (!c) return false
    this.field.setRangeText(c[2], c[0], c[1], 'end')
    this.seen = this.field.value
    // The field's owner hears of it as of any edit.
    this.field.dispatchEvent(new Event('input', { bubbles: true }))
    return true
  }
}

const kept = new WeakMap<Element, Kept>()

// keepsUndo is the action that gives a field its own undo.
export function keepsUndo(field: Field) {
  const k = new Kept(field)
  kept.set(field, k)
  const before = (e: Event) => k.before(e as InputEvent)
  const input = () => k.input()
  field.addEventListener('beforeinput', before)
  field.addEventListener('input', input)
  return {
    destroy() {
      kept.delete(field)
      field.removeEventListener('beforeinput', before)
      field.removeEventListener('input', input)
    },
  }
}

// fieldStep takes Undo or Redo in the field being typed in, and says
// whether it took a step there. A field without its own undo gets the
// browser's, as before, and has taken one when its text changed.
export function fieldStep(on: Element | null, how: 'undo' | 'redo'): boolean {
  if (!on) return false
  const k = kept.get(on)
  if (k) return k.step(how)
  const field = on as Field
  const was = field.value
  document.execCommand(how)
  return field.value !== was
}
