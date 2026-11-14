// Ported from FrameFairy's lib/chrome.svelte.ts at 57bca0c. Speaker Trail
// has one window, so it follows that one, and it asks over /api/chrome
// rather than through a Wails binding, the way lib/updates.svelte.ts asks.
import { Events } from '@wailsio/runtime'
import { api, type Chrome } from './api'

// Where macOS put the title bar and its buttons. All zeros away from
// macOS, where the system draws its own title bar, and then the bar is
// an ordinary header and the stylesheet keeps its own numbers.
//
// Nothing here works a size out from a measurement of the page. This is
// the one thing the stylesheet cannot know, asked for once and told
// again when it changes, and handed over as three numbers it lays itself
// out from, see .titlebar in PrivateApp.svelte.
class TitleBar {
  chrome = $state<Chrome>({ bar: 0, left: 0, right: 0 })
  // The height is given whenever there is one. Where the buttons are is
  // given only when there are buttons, so that in native fullscreen, where
  // macOS takes them away, the name moves to the left edge rather than
  // keeping room for buttons that are not there.
  style = $derived.by(() => {
    const c = this.chrome
    const parts: string[] = []
    if (c.bar > 0) parts.push(`--bar-h: ${c.bar}px`)
    if (c.right > 0) {
      parts.push(`--lights-l: ${c.left}px`)
      parts.push(`--lights-r: ${c.right}px`)
    }
    return parts.join('; ')
  })

  // Asks once for the window and follows every change after, and gives
  // what stops it. The window is told again whenever the answer changes:
  // a resize, either way through fullscreen, another display, another
  // scale, light or dark, or back from the Dock, see watchChrome in
  // chrome.go.
  follow(): () => void {
    api
      .chrome()
      .then((c) => (this.chrome = c))
      .catch(() => {})
    return Events.On('chrome', (ev) => (this.chrome = ev.data as Chrome))
  }
}

export const titleBar = new TitleBar()
