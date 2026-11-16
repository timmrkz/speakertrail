// One short message at the bottom of the screen, with an optional action
// like Undo.
//
// Every toast goes into the app's log. One about something that went
// wrong is a warning marked as a toast, so Settings can list it again
// once it is gone, see toastsIn in cmd/speakertrail-app/applog.go, the
// way every toast is in FrameFairy, whose toasts only ever say what went
// wrong. One that says what was done is an info line.

import { said } from './said'

export interface ToastAction {
  label: string
  run: () => void
}

class Toast {
  message = $state('')
  tone = $state<'plain' | 'bad'>('plain')
  action = $state<ToastAction | null>(null)
  private timer: ReturnType<typeof setTimeout> | undefined

  show(message: string, tone: 'plain' | 'bad' = 'plain', action: ToastAction | null = null) {
    if (this.message) said(`toast replaced: ${this.message}`, { level: 'info' })
    said(message, tone === 'bad' ? { level: 'warn', toast: true } : { level: 'info' })
    this.message = message
    this.tone = tone
    this.action = action
    clearTimeout(this.timer)
    // Time to reach the action before it goes.
    this.timer = setTimeout(() => this.hide('ran out'), tone === 'bad' || action ? 5000 : 2600)
  }

  // The person took its action, or its time ran out.
  hide(how: 'acted on' | 'ran out' = 'acted on') {
    if (this.message) said(`toast ${how}: ${this.message}`, { level: 'info' })
    clearTimeout(this.timer)
    this.message = ''
    this.action = null
  }
}

export const toast = new Toast()

export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : 'Something went wrong'
}
