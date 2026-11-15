// Walks the question Cmd+Q asks against the mock, in Chromium, the way
// walks/titlebar.mjs does. `make walks` runs it, and so does CI. The Go
// side, cmd/speakertrail-app/quit.go, tells the page through the Wails
// event called quit, and the walk sends it the way the app does, through
// window._wails.dispatchWailsEvent. FrameFairy has stub hooks for this
// and no walk that uses them, issue #17.
import { chromium } from 'playwright'
import { createServer } from 'vite'
import { fileURLToPath } from 'node:url'
import { existsSync, readFileSync } from 'node:fs'

if (!existsSync(chromium.executablePath())) {
  console.error(`The walks need Playwright's Chromium, and it is not at ${chromium.executablePath()}. Install it with: cd web && npx playwright install chromium`)
  process.exit(1)
}

const web = fileURLToPath(new URL('..', import.meta.url))
process.env.MOCK = '1'
const server = await createServer({ root: web, configFile: `${web}/vite.config.ts`, logLevel: 'error', server: { port: 0, strictPort: false } })
await server.listen()
const url = server.resolvedUrls.local[0].replace(/\/$/, '')

// How long the question stays, from the one place the interface keeps it.
const quitAgain = Number(readFileSync(`${web}/src/lib/api.ts`, 'utf8').match(/^export const QUIT_AGAIN_MS = (\d+)$/m)?.[1])

// The app's window on Tim's Mac, so the key reads ⌘Q.
const mac = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)'

// The page open on People, with the clock the walk moves itself, and every
// POST /api/quit/stay counted.
async function open(page) {
  const stays = { n: 0 }
  page.on('request', (r) => {
    if (r.method() === 'POST' && new URL(r.url()).pathname === '/api/quit/stay') stays.n++
  })
  await page.clock.install()
  await page.goto(`${url}/app/people`)
  await page.waitForSelector('header.titlebar')
  return stays
}

// What the Go side says, the way it says it.
const say = (page, what) => page.evaluate((what) => window._wails.dispatchWailsEvent({ name: 'quit', data: what }), what)

// What the layer shows, or null when there is none.
const shown = (page) =>
  page.evaluate(() => {
    const layer = document.querySelector('.leaving')
    if (!layer) return null
    const box = layer.getBoundingClientRect()
    const p = layer.querySelector('p')
    const text = document.createRange()
    text.selectNodeContents(p)
    return {
      words: p.textContent,
      role: layer.getAttribute('role'),
      covers: box.left === 0 && box.top === 0 && box.width === innerWidth && box.height === innerHeight,
      oneLine: text.getClientRects().length === 1,
      wide: document.documentElement.scrollWidth > innerWidth,
    }
  })

// Waits for what the stay request needs to leave the page, so a count of
// none is not read before it was sent.
const settled = (page) => page.evaluate(() => fetch('/api/chrome').then(() => {}))

const walks = [
  {
    name: 'The first Cmd+Q asks over the whole app, and Escape takes the question away',
    async run(page) {
      const stays = await open(page)
      await say(page, 'ask')
      const s = await shown(page)
      if (!s) return 'Cmd+Q heard and nothing on screen'
      if (s.words !== 'Press ⌘Q again to quit') return `the question reads "${s.words}"`
      if (s.role !== 'status') return `the layer has role ${s.role}, not status`
      if (!s.covers) return 'the layer does not cover the whole app'
      await page.keyboard.press('Escape')
      if (await shown(page)) return 'Escape left the question on screen'
      await settled(page)
      if (stays.n !== 1) return `Escape asked POST /api/quit/stay ${stays.n} times, not once`
      return ''
    },
  },
  {
    name: 'A click takes the question away and tells the app',
    async run(page) {
      const stays = await open(page)
      await say(page, 'ask')
      await page.mouse.click(20, 400)
      if (await shown(page)) return 'a click left the question on screen'
      await settled(page)
      if (stays.n !== 1) return `the click asked POST /api/quit/stay ${stays.n} times, not once`
      return ''
    },
  },
  {
    name: 'The question goes by itself when the second press may no longer come',
    async run(page) {
      if (!(quitAgain > 0)) return 'web/src/lib/api.ts says no QUIT_AGAIN_MS'
      const stays = await open(page)
      await say(page, 'ask')
      await page.clock.runFor(quitAgain - 1)
      if (!(await shown(page))) return `the question went after ${quitAgain - 1} ms, before the second press may no longer come`
      await page.clock.runFor(1)
      if (await shown(page)) return `the question is still there after ${quitAgain} ms`
      await settled(page)
      if (stays.n !== 0) return 'the question going by itself asked POST /api/quit/stay'
      return ''
    },
  },
  {
    name: 'The second Cmd+Q says Quitting, and nothing takes it away',
    async run(page) {
      const stays = await open(page)
      await say(page, 'ask')
      await page.clock.runFor(quitAgain - 1)
      await say(page, 'going')
      let s = await shown(page)
      if (s?.words !== 'Quitting') return `the second press shows ${s ? `"${s.words}"` : 'nothing'}`
      // The question's own time ran out here, had it been left to run.
      await page.clock.runFor(quitAgain)
      await page.keyboard.press('Escape')
      await page.mouse.click(20, 400)
      s = await shown(page)
      if (s?.words !== 'Quitting') return `Quitting went from the screen, it shows ${s ? `"${s.words}"` : 'nothing'}`
      await settled(page)
      if (stays.n !== 0) return `Quitting asked POST /api/quit/stay ${stays.n} times`
      return ''
    },
  },
  {
    name: 'The question fits on one line in the narrowest window',
    viewport: { width: 390, height: 844 },
    async run(page) {
      await open(page)
      await say(page, 'ask')
      const s = await shown(page)
      if (!s) return 'Cmd+Q heard and nothing on screen'
      if (!s.oneLine) return 'the question wraps'
      if (s.wide) return 'the page scrolls sideways'
      return ''
    },
  },
]

const browser = await chromium.launch()
let failed = 0
for (const w of walks) {
  const page = await browser.newPage({ viewport: w.viewport ?? { width: 1280, height: 900 }, userAgent: mac })
  const errors = []
  page.on('pageerror', (e) => errors.push(e.message))
  let wrong
  try {
    wrong = await w.run(page)
  } catch (e) {
    wrong = String(e).split('\n')[0]
  }
  if (!wrong && errors.length) wrong = `the page threw: ${errors.join(', ')}`
  console.log(`${wrong ? 'FAIL' : 'ok  '}\t${w.name}${wrong ? `\n\t${wrong}` : ''}`)
  if (wrong) failed++
  await page.close()
}
await browser.close()
await server.close()
process.exit(failed ? 1 : 0)
