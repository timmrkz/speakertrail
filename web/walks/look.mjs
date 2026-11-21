// Walks over the look of the app against the mock, in Chromium, the way
// walks/focus.mjs does, in light and in dark. Tim asked on 21 November
// 2026 for a flat interface, issue #45: no shadows and no frosted glass,
// solid colours, and the states of every control, at rest, under the
// pointer, pressed, chosen and greyed out, readable and the same
// everywhere. The light of work in hand, the beam of Busy.svelte, glows
// on purpose and is left out. `make walks` runs them, and so does CI.
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

// The helpers the page needs, put on window once it has loaded: a colour
// in any CSS syntax as the four bytes a canvas paints it with, the solid
// ground under an element with every translucent layer under it laid on
// the one below, and WCAG's contrast between two colours.
function helpers() {
  const g = new OffscreenCanvas(1, 1).getContext('2d', { willReadFrequently: true })
  const bytes = (css) => {
    g.clearRect(0, 0, 1, 1)
    g.fillStyle = 'rgba(0, 0, 0, 0)'
    g.fillStyle = css
    g.fillRect(0, 0, 1, 1)
    const [r, gr, b, a] = g.getImageData(0, 0, 1, 1).data
    return [r, gr, b, a / 255]
  }
  const over = (top, under) => {
    const a = top[3]
    return [0, 1, 2].map((i) => top[i] * a + under[i] * (1 - a)).concat(1)
  }
  const ground = (el) => {
    const layers = []
    for (let e = el; e; e = e.parentElement) layers.push(bytes(getComputedStyle(e).backgroundColor))
    let out = [255, 255, 255, 1]
    for (const l of layers.reverse()) out = over(l, out)
    return out
  }
  const lum = (c) => {
    const [r, gr, b] = c.slice(0, 3).map((v) => {
      v /= 255
      return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4
    })
    return 0.2126 * r + 0.7152 * gr + 0.0722 * b
  }
  const contrast = (a, b) => {
    const [x, y] = [lum(a), lum(b)]
    return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
  }
  // How an element reads: its words on its ground, and the ground itself.
  const reads = (el) => {
    const bg = ground(el)
    const ink = over(bytes(getComputedStyle(el).color), bg)
    return { bg: bg.map(Math.round).join(','), ink: contrast(ink, bg) }
  }
  window.look = { bytes, over, ground, contrast, reads }
}

// Every element on screen that casts a shadow or blurs what is behind it,
// leaving out the light of work in hand.
function deep(page) {
  return page.evaluate(() => {
    const found = []
    for (const el of document.querySelectorAll('body *')) {
      if (el.closest('.beam')) continue
      for (const pseudo of [null, '::before', '::after']) {
        const s = getComputedStyle(el, pseudo)
        const what = []
        // A hairline drawn inside an edge, so it takes no room, is a line,
        // not a shadow: inset, one pixel, with no blur and no spread.
        const line = /^\S+(?: \S+)*? (-?1px 0px|0px -?1px) 0px 0px inset$/
        if (s.boxShadow !== 'none' && !s.boxShadow.split(/,(?![^(]*\))/).every((x) => line.test(x.trim()))) what.push(`a shadow ${s.boxShadow}`)
        if (s.backdropFilter && s.backdropFilter !== 'none') what.push(`a backdrop ${s.backdropFilter}`)
        if (s.filter && s.filter !== 'none') what.push(`a filter ${s.filter}`)
        if (what.length) found.push(`${el.tagName.toLowerCase()}.${[...el.classList].join('.')}${pseudo ?? ''} has ${what.join(' and ')}`)
      }
    }
    return found
  })
}

// The words of a control against its ground at rest, under the pointer
// and pressed. Each state is a ground of its own, pressed differs from
// under the pointer, and the words read at 4.5 to 1 in every one.
async function states(page, name, control) {
  const wrong = []
  await control.scrollIntoViewIfNeeded()
  await page.mouse.move(1, 1)
  const rest = await control.evaluate((e) => window.look.reads(e))
  await control.hover()
  const hover = await control.evaluate((e) => window.look.reads(e))
  const box = await control.boundingBox()
  await page.mouse.down()
  const press = await control.evaluate((e) => window.look.reads(e))
  // Let go away from it, so the press clicks nothing.
  await page.mouse.move(1, 1)
  await page.mouse.up()
  if (!box) wrong.push(`${name} is not on screen`)
  if (hover.bg === rest.bg) wrong.push(`${name} does not change under the pointer`)
  if (press.bg === hover.bg) wrong.push(`${name} does not change when pressed`)
  for (const [state, r] of [['at rest', rest], ['under the pointer', hover], ['pressed', press]]) {
    if (r.ink < 4.5) wrong.push(`${name} ${state} reads at ${r.ink.toFixed(2)} to 1`)
  }
  return wrong
}

// A button that cannot be pressed is grey, not a paler copy of itself:
// the same ground in every kind, and its words still read at 3 to 1.
async function greyed(page, controls) {
  const wrong = []
  const grounds = new Set()
  for (const [name, control] of controls) {
    const r = await control.evaluate((e) => {
      const was = e.disabled
      e.disabled = true
      const r = window.look.reads(e)
      e.disabled = was
      return r
    })
    grounds.add(r.bg)
    if (r.ink < 3) wrong.push(`${name} greyed out reads at ${r.ink.toFixed(2)} to 1`)
  }
  if (grounds.size > 1) wrong.push(`buttons greyed out have ${grounds.size} different grounds`)
  return wrong
}

const walks = []
for (const theme of ['light', 'dark']) {
  walks.push({
    theme,
    name: `In ${theme}, no screen casts a shadow or blurs what is behind it`,
    async run(page) {
      const found = []
      for (const path of ['/app/people', '/app/searches', '/app/settings', '/app/updates', '/report']) {
        await page.goto(`${url}${path}`)
        await page.locator('main, .report').first().waitFor()
        await page.waitForLoadState('networkidle')
        found.push(...(await deep(page)).map((f) => `${path}: ${f}`))
      }
      // A person open, with the sidebar kept open beside it, and the list
      // of an order open over it.
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().click()
      await page.locator('aside .glyph').click()
      await page.locator('main .pick').first().click()
      await page.locator('.pick-list').waitFor()
      found.push(...(await deep(page)).map((f) => `a person open: ${f}`))
      return found.slice(0, 6).join('\n\t')
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, every control in People reads in every state, and chosen looks the same everywhere`,
    async run(page) {
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').nth(1).click()
      await page.locator('.detail-pane .btn').first().waitFor()
      await page.evaluate(helpers)
      const wrong = []
      const controls = [
        ['Run', page.locator('main button.start')],
        ['LinkedIn', page.locator('.detail-pane a.btn', { hasText: 'LinkedIn' })],
        ['the order', page.locator('main .pick').first()],
        ['People in the sidebar', page.locator('aside .nav[aria-current="page"]')],
        ['Searches in the sidebar', page.locator('aside .nav', { hasText: 'Searches' })],
        ['the filter shown', page.locator('main .filters button[aria-pressed="true"]')],
        ['a filter not shown', page.locator('main .filters button[aria-pressed="false"]').first()],
        ['the person open', page.locator('.prow[aria-current="true"]')],
        ['a person not open', page.locator('.prow:not([aria-current])').first()],
      ]
      for (const [name, control] of controls) wrong.push(...(await states(page, name, control)))
      // What is chosen wears one tint, whatever it lies on.
      await page.mouse.move(1, 1)
      const tint = await page.evaluate(() => {
        const of = (sel) => getComputedStyle(document.querySelector(sel)).backgroundColor
        return [of('aside .nav[aria-current="page"]'), of('main .filters button[aria-pressed="true"]'), of('.prow[aria-current="true"]')]
      })
      if (new Set(tint).size > 1) wrong.push(`what is chosen is drawn in ${new Set(tint).size} tints: ${tint.join(', ')}`)
      wrong.push(...(await greyed(page, [
        ['Run', page.locator('main button.start')],
        ['Draft', page.locator('.detail-pane button.btn.primary').first()],
        ['the one who is not a fit', page.locator('.detail-pane button.btn:not(.primary)').last()],
      ])))
      return wrong.join('\n\t')
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, the fill of a run stands out from its track, its light too`,
    async run(page) {
      await page.goto(`${url}/app/people`)
      await page.locator('main button.start').click()
      const fill = page.locator('.progress > i').first()
      await fill.waitFor()
      await page.evaluate(helpers)
      const m = await fill.evaluate((i) => {
        const { bytes, over, ground, contrast } = window.look
        const track = ground(i.parentElement)
        const done = over(bytes(getComputedStyle(i).backgroundColor), track)
        // The brightest point of the light that travels over the fill is
        // the middle stop of its gradient.
        const stops = getComputedStyle(i, '::after').backgroundImage.match(/(rgba?|color)\([^)]*\)/g) ?? []
        const lit = stops.length >= 3 ? over(bytes(stops[1]), done) : done
        return { fill: contrast(done, track), lit: contrast(lit, track) }
      })
      const wrong = []
      if (m.fill < 3) wrong.push(`the fill stands out from its track at ${m.fill.toFixed(2)} to 1`)
      if (m.lit < 3) wrong.push(`the light on the fill stands out from its track at ${m.lit.toFixed(2)} to 1`)
      return wrong.join(', ')
    },
  })
}

const browser = await chromium.launch()
let failed = 0
for (const w of walks) {
  const page = await browser.newPage({ viewport: { width: 1280, height: 860 }, colorScheme: w.theme })
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
