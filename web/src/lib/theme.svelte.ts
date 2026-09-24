// Light, dark or follow the system. Stored per browser only.

export type ThemeChoice = 'system' | 'light' | 'dark'

function read(): ThemeChoice {
  try {
    const t = localStorage.getItem('st-theme')
    if (t === 'light' || t === 'dark') return t
  } catch {
    // Storage can be blocked. Follow the system then.
  }
  return 'system'
}

class Theme {
  choice = $state<ThemeChoice>(read())

  set(choice: ThemeChoice) {
    this.choice = choice
    const root = document.documentElement
    if (choice === 'system') delete root.dataset.theme
    else root.dataset.theme = choice
    try {
      if (choice === 'system') localStorage.removeItem('st-theme')
      else localStorage.setItem('st-theme', choice)
    } catch {
      // Not stored. The choice still holds until the page reloads.
    }
  }

  cycle() {
    this.set(this.choice === 'system' ? 'dark' : this.choice === 'dark' ? 'light' : 'system')
  }

  get label(): string {
    return { system: 'Theme follows system', dark: 'Dark theme', light: 'Light theme' }[this.choice]
  }
}

export const theme = new Theme()
