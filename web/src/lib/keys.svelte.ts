// Whether the Exa key is saved and how much of its budget is used, as the
// app says. Asked when Settings or the sidebar opens and after the key is
// saved or removed, so the dot on Settings in the sidebar and the row in
// Settings say the same. The key itself never comes back to the window.
import { api, type SearchUse } from './api'

class KeyWatch {
  exa = $state<SearchUse | null>(null)
  loaded = $state(false)

  // Asks the app. Throws what went wrong, for Settings to say.
  async read() {
    const all = await api.keys()
    this.exa = all.find((x) => x.provider === 'exa') ?? null
    this.loaded = true
  }
}

export const keyWatch = new KeyWatch()
