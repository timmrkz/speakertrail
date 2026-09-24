// A tiny path router on the History API. The Go server answers index.html
// for every path outside /api, so every path here is a real URL.

type Params = Record<string, string>

class Router {
  path = $state(location.pathname)
  search = $state(location.search)

  constructor() {
    window.addEventListener('popstate', () => this.sync())
    // Same-origin links navigate in place. New tabs, downloads, /api and
    // modified clicks keep the browser default.
    document.addEventListener('click', (e) => {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
      const a = (e.target as Element | null)?.closest?.('a')
      if (!a || a.target || a.hasAttribute('download') || a.origin !== location.origin) return
      if (a.pathname.startsWith('/api/')) return
      e.preventDefault()
      this.go(a.pathname + a.search)
    })
  }

  private sync() {
    this.path = location.pathname
    this.search = location.search
  }

  go(to: string, opts: { replace?: boolean } = {}) {
    if (to === location.pathname + location.search) return
    if (opts.replace) history.replaceState(null, '', to)
    else history.pushState(null, '', to)
    const prev = this.path
    this.sync()
    const next = this.path
    // Opening or closing a sheet (/app/people <-> /app/people/7) keeps the scroll.
    const nested = next.startsWith(`${prev}/`) || prev.startsWith(`${next}/`)
    if (prev !== next && !nested && !opts.replace) window.scrollTo({ top: 0 })
  }

  // Returns the params when `pattern` matches the current path, like
  // match('/app/people/:id') gives { id: '7' } on /app/people/7.
  match(pattern: string, path = this.path): Params | null {
    const a = pattern.split('/').filter(Boolean)
    const b = path.split('/').filter(Boolean)
    if (a.length !== b.length) return null
    const params: Params = {}
    for (let i = 0; i < a.length; i++) {
      if (a[i].startsWith(':')) params[a[i].slice(1)] = decodeURIComponent(b[i])
      else if (a[i] !== b[i]) return null
    }
    return params
  }

  // True when the current path is `prefix` or below it.
  under(prefix: string): boolean {
    return this.path === prefix || this.path.startsWith(`${prefix}/`)
  }
}

export const router = new Router()

export function navigate(to: string, opts: { replace?: boolean } = {}) {
  router.go(to, opts)
}
