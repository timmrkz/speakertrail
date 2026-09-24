// One short message at the bottom of the screen.

class Toast {
  message = $state('')
  tone = $state<'plain' | 'bad'>('plain')
  private timer: ReturnType<typeof setTimeout> | undefined

  show(message: string, tone: 'plain' | 'bad' = 'plain') {
    this.message = message
    this.tone = tone
    clearTimeout(this.timer)
    this.timer = setTimeout(() => (this.message = ''), tone === 'bad' ? 5000 : 2600)
  }
}

export const toast = new Toast()

export function errorText(e: unknown): string {
  return e instanceof Error ? e.message : 'Something went wrong'
}
