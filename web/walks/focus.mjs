// Walks through focus against the mock, in Chromium, the way
// walks/logs.mjs does: a control reached with the keyboard has to be
// found at a glance, in light and in dark. Tim could not tell whether
// Report had focus, testing #24 on 17 November 2026. `make walks` runs
// them, and so does CI.
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

// The pixels of a control's box, painted.
async function pixels(page, box) {
  const png = await page.screenshot({ clip: box, animations: 'disabled', caret: 'hide' })
  return page.evaluate(async (b64) => {
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
    const bitmap = await createImageBitmap(new Blob([bytes], { type: 'image/png' }))
    const canvas = new OffscreenCanvas(bitmap.width, bitmap.height)
    const g = canvas.getContext('2d')
    g.drawImage(bitmap, 0, 0)
    return { w: bitmap.width, h: bitmap.height, data: Array.from(g.getImageData(0, 0, bitmap.width, bitmap.height).data) }
  }, png.toString('base64'))
}

// WCAG's relative luminance and the contrast between two of them.
const lum = (d, i) => {
  const c = [d[i], d[i + 1], d[i + 2]].map((v) => {
    v /= 255
    return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]
}
const contrast = (a, b) => (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05)

// How much of the mark of focus changes, with focus and without it, by a
// contrast of 3 to 1 or more, the least WCAG asks of what marks focus. A
// button wears a ring round it and a row an edge inside it, see
// :focus-visible in app.css, so the mark is measured where the control
// draws it: the band its outline covers, out from its edge or in from
// it. At each point along the band, the most any of its device pixels
// changed counts, so a box that starts between two pixels is measured
// the same. The round corners are left out.
async function edgeChange(page, control) {
  await page.mouse.move(0, 0)
  await page.keyboard.press('Shift')
  await control.focus()
  if (!(await control.evaluate((e) => e.matches(':focus-visible')))) return { error: 'it took focus without showing it as the keyboard\'s' }
  const { corner, out } = await control.evaluate((e) => {
    const s = getComputedStyle(e)
    // How far out from the edge the band ends: the ring's offset and
    // width for a ring, nothing for an edge drawn inside.
    const offset = parseFloat(s.outlineOffset) || 0
    return { corner: parseFloat(s.borderTopLeftRadius) || 0, out: offset > 0 ? offset + (parseFloat(s.outlineWidth) || 0) : 0 }
  })
  // Each state is taken where the control is then, since focus can
  // scroll it into view.
  const grow = (r) => ({ x: r.x - out, y: r.y - out, width: r.width + 2 * out, height: r.height + 2 * out })
  const around = (r) => ({ x: Math.floor(r.x) - 1, y: Math.floor(r.y) - 1, width: Math.ceil(r.width) + 3, height: Math.ceil(r.height) + 3 })
  const r = grow(await control.boundingBox())
  const box = around(r)
  const lit = await pixels(page, box)
  await control.evaluate((e) => e.blur())
  const rest = await pixels(page, around(grow(await control.boundingBox())))
  const scale = rest.w / box.width
  // The control's own box in device pixels, inside the screenshot.
  const left = Math.ceil((r.x - box.x) * scale)
  const top = Math.ceil((r.y - box.y) * scale)
  const right = Math.floor((r.x + r.width - box.x) * scale) - 1
  const bottom = Math.floor((r.y + r.height - box.y) * scale) - 1
  const depth = Math.ceil(scale) + 1
  const skip = Math.ceil((corner + out) * scale)
  const change = (x, y) => {
    const i = (y * rest.w + x) * 4
    return contrast(lum(rest.data, i), lum(lit.data, i))
  }
  let changed = 0
  let all = 0
  const along = (points) => {
    for (const p of points) {
      all++
      if (Math.max(...p) >= 3) changed++
    }
  }
  const run = (from, to) => Array.from({ length: Math.max(0, to - from + 1) }, (_, k) => from + k)
  const d = run(0, depth - 1)
  along(run(left + skip, right - skip).map((x) => d.map((k) => change(x, top + k))))
  along(run(left + skip, right - skip).map((x) => d.map((k) => change(x, bottom - k))))
  along(run(top + skip, bottom - skip).map((y) => d.map((k) => change(left + k, y))))
  along(run(top + skip, bottom - skip).map((y) => d.map((k) => change(right - k, y))))
  return { share: all ? changed / all : 0 }
}

const report = [
  ['Report', (p) => p.locator('button.make')],
  ['Clear', (p) => p.locator('button.clear')],
  ['Dictate', (p) => p.locator('button.dictate')],
  ['Attach', (p) => p.locator('button.attach')],
]
const settings = [
  ['Download', (p) => p.locator('.speech button.act')],
  ['Remove', (p) => p.locator('main button.btn', { hasText: /^Remove$/ })],
  ['System', (p) => p.locator('main button', { hasText: 'System' })],
  ['Light', (p) => p.locator('main button', { hasText: 'Light' })],
]

const people = [
  ['Run', (p) => p.locator('main button.start')],
  ['the filter shown', (p) => p.locator('main .filters button[aria-pressed="true"]')],
  ['a filter not shown', (p) => p.locator('main .filters button[aria-pressed="false"]').first()],
]

const walks = []
for (const theme of ['light', 'dark']) {
  walks.push({
    theme,
    // Report is where Tim lost it. Tab goes through the window in the
    // order it is drawn, see report.mjs.
    name: `In ${theme}, every button of Report a Problem shows it has focus on its edge`,
    async run(page) {
      await page.setViewportSize({ width: 560, height: 640 })
      await page.goto(`${url}/report`)
      const field = page.locator('textarea.telling')
      await field.waitFor()
      await field.click()
      await page.keyboard.type('It froze.')
      return measure(page, report)
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, the buttons in Settings show they have focus on their edge`,
    async run(page) {
      await page.goto(`${url}/app/settings`)
      await page.locator('main button', { hasText: 'System' }).waitFor()
      return measure(page, settings)
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, People shows focus on its buttons, and nothing more on the open person`,
    async run(page) {
      await page.goto(`${url}/app/people`)
      const row = page.locator('.prow').first()
      await row.waitFor()
      const weak = await measure(page, people)
      if (weak) return weak
      // The open person's row is marked already. Arrow keys move it, and
      // the keyboard on it adds no edge, see the interface skill.
      await row.click()
      await page.keyboard.press('ArrowDown')
      await page.waitForFunction(() => {
        const e = document.activeElement
        return e?.matches('.prow[aria-current="true"]') && e !== document.querySelector('.prow')
      })
      const mark = await page.evaluate(() => ({ focus: document.activeElement.matches(':focus-visible'), outline: getComputedStyle(document.activeElement).outlineStyle }))
      if (!mark.focus) return 'the open person\'s row took focus without showing it as the keyboard\'s'
      if (mark.outline !== 'none') return `the open person's row wears a ${mark.outline} edge`
      return ''
    },
  })
}

async function measure(page, controls) {
  const weak = []
  for (const [name, find] of controls) {
    const m = await edgeChange(page, find(page))
    if (m.error) weak.push(`${name}: ${m.error}`)
    else if (m.share < 0.95) weak.push(`${name}: ${Math.round(m.share * 100)} % of its edge changes`)
  }
  return weak.join(', ')
}

const browser = await chromium.launch()
let failed = 0
for (const w of walks) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 860 }, colorScheme: w.theme, deviceScaleFactor: 2 })
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
