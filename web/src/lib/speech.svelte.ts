// The speech models, as the app says they stand: which are here, and the
// latest download with how far it has come. The app sends the event
// speech whenever that changes, the way it sends updates, and this is
// asked once when a screen that shows it opens and after each press. One
// watch for Settings and Report a Problem alike, so neither shows a state
// the other has left. See cmd/speakertrail-app/speechmodels.go.
import { Events } from '@wailsio/runtime'
import { api, type SpeechModel, type SpeechState } from './api'

class SpeechWatch {
  state = $state<SpeechState>({ models: [], folder: '' })
  // Whether the app has answered once, so nothing is said about the
  // models before it is known.
  loaded = $state(false)
  // The model Dictate hears with, as the app marks it.
  readonly inUse = $derived<SpeechModel | undefined>(this.state.models.find((m) => m.inUse))
  private users = 0
  private off: (() => void) | undefined

  async read() {
    try {
      this.state = await api.speech()
      this.loaded = true
    } catch {
      // Settings says what went wrong when it asks itself.
    }
  }

  // Follows the event while a screen that shows it is open. Returns what
  // stops following, for the screen's effect to call.
  watch(): () => void {
    if (this.users++ === 0) {
      this.off = Events.On('speech', (ev) => {
        this.state = ev.data as SpeechState
        this.loaded = true
      })
    }
    this.read()
    return () => {
      if (--this.users === 0) {
        this.off?.()
        this.off = undefined
      }
    }
  }
}

export const speechWatch = new SpeechWatch()
