// One short message at the bottom of the screen, with an optional action
// like Undo.

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
    this.message = message
    this.tone = tone
    this.action = action
    clearTimeout(this.timer)
    // Time to reach the action before it goes.
    this.timer = setTimeout(() => this.hide(), tone === 'bad' || action ? 5000 : 2600)
  }

  hide() {
    clearTimeout(this.timer)
    this.message = ''
    this.action = null
  }
}

export const toast = new Toast()

export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : 'Something went wrong'
}
