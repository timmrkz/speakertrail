// What went wrong, said at the control it went wrong at, never in a
// toast over the window (Tim, testing #60 on 22 November 2026, #72). Each
// place that can fail keeps one Problem, and ProblemNote.svelte draws it
// where the click was: under a row of buttons or a field, or over the top
// of a text box. It stays until the place is used again, so it is read,
// and it goes into the app's log the way a toast about something that
// went wrong did, so Settings, Logs lists it once it is gone, see
// toastsIn in cmd/speakertrail-app/applog.go.

import { said } from './said'

export class Problem {
  text = $state('')
  // Whether it offers Report: a failure the person cannot put right by
  // themselves, as FrameFairy marks one, its issue #311. A key that is
  // missing or a budget that is spent is theirs to put right, a file the
  // app could not write is not.
  report = $state(false)

  // say puts what went wrong here, in the app's words or the ones given.
  say(e: unknown) {
    this.put(typeof e === 'string' ? e : errorText(e), false)
  }

  // failed puts a failure the person cannot put right here, with Report
  // beside it.
  failed(e: unknown) {
    this.put(errorText(e), true)
  }

  // needs says what the person has to do first: what a field typed wrong
  // needs, like a search with no words or one that is there already, or a
  // choice still to make in Settings. Nothing went wrong, so Settings,
  // Logs leaves it out.
  needs(text: string) {
    said(`field: ${text}`, { level: 'info' })
    this.text = text
    this.report = false
  }

  // clear takes it away, when the place is used again.
  clear() {
    this.text = ''
    this.report = false
  }

  private put(text: string, report: boolean) {
    said(text, { level: 'warn', toast: true })
    this.text = text
    this.report = report
  }
}

export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : 'Something went wrong'
}
