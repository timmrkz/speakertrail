// Walks the title bar against the mock, in Chromium, the way
// walks/sidebar.mjs does, and the way FrameFairy's "title bar" step in
// frontend/preview/walks/app/steps.mjs checks its own. `make walks` runs
// it, and so does CI. The mock answers /api/chrome with what macOS
// answered on Tim's Mac: a title bar 30 high, the buttons from 12 to 72.
import { chromium } from 'playwright'
import { createServer } from 'vite'
import { fileURLToPath } from 'node:url'
import { existsSync } from 'node:fs'

if (!existsSync(chromium.executablePath())) {
  console.error(`The walks need Playwright's Chromium, and it is not at ${chromium.executablePath()}. Install it with: cd web && npx playwright install chromium`)
  process.exit(1)
}

const web = fileURLToPath(new URL('..', import.meta.url))
process.env.MOCK = '1'
const server = await createServer({ root: web, configFile: `${web}/vite.config.ts`, logLevel: 'error', server: { port: 0, strictPort: false } })
await server.listen()
const url = server.resolvedUrls.local[0].replace(/\/$/, '')

// What the bar and the name are on screen, once the bar has taken the
// height macOS gave it.
async function seen(page) {
  // The page asks once it is up. A bar that never takes the answer is
  // measured as it stands, so the walk says how high it is.
  await page
    .waitForFunction(() => document.querySelector('header.titlebar')?.getAttribute('style')?.includes('--bar-h: 30px'), null, { timeout: 5000 })
    .catch(() => {})
  return page.evaluate(() => {
    const box = (el) => el?.getBoundingClientRect()
    const bar = box(document.querySelector('header.titlebar'))
    const name = box(document.querySelector('header.titlebar .what'))
    return { bar: [bar.top, bar.height], name: name && [name.left, name.top, name.height] }
  })
}

const walks = [
  {
    // Tim saw the bar taller than a Mac's, with the name under the
    // buttons, #18. The bar is as tall as the title bar macOS laid out,
    // and the name's capitals stand on the middle of what is seen of it:
    // under the window's edge over its top 2 points and over the line on
    // its last. A quarter of a pixel is the most they may be off, as in
    // FrameFairy's walk, because half a point is what Tim saw there.
    name: 'The title bar is as tall as macOS laid it out, the name on its middle after the buttons',
    async run(page) {
      await page.goto(`${url}/app/people`)
      const s = await seen(page)
      if (s.bar[1] !== 30) return `the title bar is ${s.bar[1]} px high, macOS laid it out 30 high`
      if (!s.name) return 'the title bar shows no name'
      const centre = s.bar[0] + 2 + (s.bar[1] - 3) / 2
      const off = s.name[1] + s.name[2] / 2 - centre
      if (Math.abs(off) > 0.25) return `the name's capitals stand ${Math.abs(off).toFixed(2)} px ${off < 0 ? 'above' : 'below'} the middle of the bar`
      if (Math.abs(s.name[0] - (72 + 12)) > 0.5) return `the name starts at ${s.name[0].toFixed(1)} px, one space after the buttons is 84`
      return ''
    },
  },
]

const browser = await chromium.launch()
let failed = 0
for (const w of walks) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 860 } })
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
