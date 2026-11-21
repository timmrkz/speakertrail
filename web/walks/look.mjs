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
    // The two colours a line may be drawn in, as the browser writes them.
    const probe = document.createElement('i')
    document.body.append(probe)
    const lines = ['--line', '--line-strong'].map((t) => {
      probe.style.color = `var(${t})`
      return getComputedStyle(probe).color
    })
    probe.remove()
    for (const el of document.querySelectorAll('body *')) {
      if (el.closest('.beam')) continue
      for (const pseudo of [null, '::before', '::after']) {
        const s = getComputedStyle(el, pseudo)
        const what = []
        // A hairline drawn inside an edge, so it takes no room, is a line,
        // not a shadow: inset, one pixel, with no blur and no spread, in
        // the colour of a line. A bright edge on top is not one.
        const line = (x) => lines.some((c) => [`${c} 1px 0px 0px 0px inset`, `${c} -1px 0px 0px 0px inset`, `${c} 0px 1px 0px 0px inset`, `${c} 0px -1px 0px 0px inset`].includes(x))
        if (s.boxShadow !== 'none' && !s.boxShadow.split(/,(?![^(]*\))/).every((x) => line(x.trim()))) what.push(`a shadow ${s.boxShadow}`)
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
      // A person in a sheet, in a narrow window.
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().click()
      await page.locator('.sheet').waitFor()
      found.push(...(await deep(page)).map((f) => `a person in a sheet: ${f}`))
      await page.setViewportSize({ width: 1280, height: 860 })
      // A person open, with the sidebar kept open beside it, and the list
      // of an order open over it.
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().click()
      await page.locator('aside .glyph').click()
      await page.locator('main .pick').first().click()
      await page.locator('.pick-list').waitFor()
      found.push(...(await deep(page)).map((f) => `a person open: ${f}`))
      await page.keyboard.press('Escape')
      // Every other layer that lies over a page: a toast, plain and bad,
      // an info bubble, the question asked before something is removed,
      // and a button with work running in it.
      await page.goto(`${url}/app/settings`)
      await page.locator('main button', { hasText: 'System' }).waitFor()
      await page.evaluate(async () => {
        const { toast } = await import('/src/lib/toast.svelte.ts')
        toast.show('Saved', 'plain', { label: 'Undo', run: () => {} })
      })
      await page.locator('.toast').waitFor()
      found.push(...(await deep(page)).map((f) => `a toast: ${f}`))
      await page.evaluate(async () => {
        const { toast } = await import('/src/lib/toast.svelte.ts')
        toast.show('The file could not be written', 'bad')
      })
      await page.locator('.toast.bad').waitFor()
      found.push(...(await deep(page)).map((f) => `a toast that says what went wrong: ${f}`))
      await page.locator('main .info .mark').first().hover()
      await page.locator('.bubble.placed').waitFor()
      found.push(...(await deep(page)).map((f) => `an info bubble: ${f}`))
      await page.mouse.move(1, 1)
      await page.locator('main button[aria-label="Remove the app\'s log"]').click()
      await page.locator('dialog[open]').waitFor()
      found.push(...(await deep(page)).map((f) => `the question before removing: ${f}`))
      await page.keyboard.press('Escape')
      const download = page.locator('.speech button.act')
      await download.click()
      await page.locator('.speech button.act > .beam').waitFor()
      found.push(...(await deep(page)).map((f) => `a download running: ${f}`))
      // Cancelled again, so the walks after this one find it not started.
      await download.click()
      await page.locator('.speech button.act', { hasText: /^\s*Download\s*$/ }).waitFor()
      return found.slice(0, 8).join('\n\t')
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, every control in People reads in every state, and chosen looks the same everywhere`,
    async run(page) {
      await page.goto(`${url}/app/people`)
      // No run going, so Run is the button it is at rest.
      await page.locator('main button.start', { hasText: /^\s*Run\s*$/ }).waitFor()
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
        ['Connect', page.locator('.detail-pane button.connect')],
        ['Draft', page.locator('.detail-pane button.btn', { hasText: 'Draft' })],
        ['the one who is not a fit', page.locator('.detail-pane button.btn:not(.primary)').last()],
      ])))
      // A toggle that is on, the one connected, in the colour it says.
      await page.locator('.prow', { hasText: 'Connected' }).first().click()
      await page.locator('.detail-pane button.connect[aria-pressed="true"]').waitFor()
      wrong.push(...(await states(page, 'Connected', page.locator('.detail-pane button.connect'))))
      // A row of a list to pick from, under the pointer and pressed.
      await page.locator('main .pick').first().click()
      const row = page.locator('.pick-row:not([data-chosen])').first()
      await row.hover()
      const lit = await row.evaluate((e) => window.look.reads(e))
      const box = await row.boundingBox()
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
      await page.mouse.down()
      const pressed = await row.evaluate((e) => window.look.reads(e))
      await page.keyboard.press('Escape')
      await page.mouse.up()
      if (pressed.bg === lit.bg) wrong.push('a row of the order does not change when pressed')
      for (const r of [lit, pressed]) if (r.ink < 4.5) wrong.push(`a row of the order reads at ${r.ink.toFixed(2)} to 1`)
      return wrong.join('\n\t')
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, the controls of Settings and Searches read in every state, and chosen is the same tint as in People`,
    async run(page) {
      await page.goto(`${url}/app/settings`)
      await page.locator('main button', { hasText: 'System' }).waitFor()
      await page.waitForLoadState('networkidle')
      await page.evaluate(helpers)
      const wrong = []
      const controls = [
        ['Download', page.locator('.speech button.act')],
        ['Remove', page.locator('main button.btn', { hasText: /^Remove$/ })],
        ['the theme chosen', page.locator('main .segmented button[aria-pressed="true"]')],
        ['a theme not chosen', page.locator('main .segmented button[aria-pressed="false"]').first()],
        ['the LinkedIn profile', page.locator('main .pick').first()],
        ['Downloaded models', page.locator('main .disclose')],
      ]
      for (const [name, control] of controls) wrong.push(...(await states(page, name, control)))
      await page.mouse.move(1, 1)
      const theme = await page.evaluate(() => getComputedStyle(document.querySelector('main .segmented button[aria-pressed="true"]')).backgroundColor)
      const page2 = await page.evaluate(() => getComputedStyle(document.querySelector('aside .nav[aria-current="page"]')).backgroundColor)
      if (theme !== page2) wrong.push(`the theme chosen is ${theme}, Settings in the sidebar ${page2}`)
      await page.goto(`${url}/app/searches`)
      await page.locator('main button.toggle').first().waitFor()
      await page.evaluate(helpers)
      for (const [name, control] of [
        ['Add', page.locator('main button.btn.primary', { hasText: 'Add' })],
        ['Pause', page.locator('main button.toggle').first()],
      ]) wrong.push(...(await states(page, name, control)))
      // A quiet button has no ground to grey, so only its words are read.
      wrong.push(...(await greyed(page, [['Add', page.locator('main button.btn.primary', { hasText: 'Add' })]])))
      wrong.push(...(await greyed(page, [['Pause', page.locator('main button.toggle').first()]])))
      return wrong.join('\n\t')
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, a list to pick from is no wider than its cap, and a button no wider than its words`,
    async run(page) {
      const wrong = []
      // Tim found the LinkedIn profile list as wide as its longest name,
      // and Check on Updates far wider than its word, testing #46.
      await page.goto(`${url}/app/settings`)
      // Enabled once the browser profiles are known, so it is as wide as
      // their longest name.
      const pick = page.locator('#linkedin-profile:enabled')
      await pick.waitFor()
      const p = await pick.evaluate((e) => {
        const probe = document.createElement('i')
        probe.style.cssText = 'position: absolute; display: block; width: var(--pick-max)'
        e.append(probe)
        const cap = probe.getBoundingClientRect().width
        probe.remove()
        return { width: e.getBoundingClientRect().width, cap }
      })
      if (p.width > p.cap + 0.5) wrong.push(`the LinkedIn profile list is ${Math.round(p.width)} px wide, over its cap of ${Math.round(p.cap)} px`)
      await page.goto(`${url}/app/updates`)
      const check = page.locator('main button.act', { hasText: 'Check' })
      await check.waitFor()
      const c = await check.evaluate((e) => {
        // The widest word it may say as the answer to a click, set in its
        // own font, with its padding and its line.
        const s = getComputedStyle(e)
        const probe = document.createElement('span')
        probe.textContent = 'Checking'
        probe.style.cssText = 'position: absolute; visibility: hidden; white-space: nowrap'
        e.append(probe)
        const words = probe.getBoundingClientRect().width
        probe.remove()
        const frame = parseFloat(s.paddingLeft) + parseFloat(s.paddingRight) + parseFloat(s.borderLeftWidth) + parseFloat(s.borderRightWidth)
        return { width: e.getBoundingClientRect().width, most: words + frame }
      })
      if (c.width > c.most + 1) wrong.push(`Check is ${Math.round(c.width)} px wide, where its longest answer needs ${Math.round(c.most)} px`)
      return wrong.join(', ')
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, the fill of a run stands out from its track, the band over it shows, and the band stays on the fill`,
    async run(page) {
      await page.goto(`${url}/app/people`)
      const start = page.locator('main button.start')
      await start.click()
      const fill = page.locator('.progress > i').first()
      await fill.waitFor()
      await page.evaluate(helpers)
      const m = await fill.evaluate((i) => {
        const { bytes, over, ground, contrast } = window.look
        const track = ground(i.parentElement)
        const done = over(bytes(getComputedStyle(i).backgroundColor), track)
        // The strongest point of the band that travels over the fill is
        // the middle stop of its gradient.
        const stops = getComputedStyle(i, '::after').backgroundImage.match(/(rgba?|color)\([^)]*\)/g) ?? []
        const band = stops.length >= 3 ? over(bytes(stops[1]), done) : done
        return { fill: contrast(done, track), band: contrast(band, done), bandTrack: contrast(band, track) }
      })
      // The run is stopped again, so the walks after this one find the
      // mock at rest.
      await start.click()
      await page.locator('main button.start', { hasText: /^\s*Run\s*$/ }).waitFor()
      const wrong = []
      if (m.fill < 3) wrong.push(`the fill stands out from its track at ${m.fill.toFixed(2)} to 1`)
      if (m.band < 1.5) wrong.push(`the band shows on the fill at ${m.band.toFixed(2)} to 1`)
      if (m.bandTrack < 3) wrong.push(`under the band the fill stands out from its track at ${m.bandTrack.toFixed(2)} to 1`)
      // The band travels over what is done and never on past it onto the
      // track. A bar a third done is laid on the page where nothing else
      // moves, every animation on it is held at points along the band's
      // way, and the track just past the fill is read off the screen at
      // each.
      await page.evaluate(() => {
        const box = document.createElement('div')
        box.id = 'look-third'
        box.style.cssText = 'position: fixed; left: 40px; top: 40px; width: 300px; padding: 16px; background: var(--surface); z-index: 1000'
        box.innerHTML = '<div class="progress"><i style="width: 33%"></i></div>'
        document.body.append(box)
        return new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(() => done())))
      })
      const own = page.locator('#look-third .progress > i')
      for (const at of [0.5, 0.7, 0.9]) {
        const strip = await own.evaluate((i, at) => {
          for (const a of i.getAnimations({ subtree: true })) {
            a.pause()
            const d = a.effect?.getComputedTiming().duration
            if (typeof d === 'number' && Number.isFinite(d)) a.currentTime = d * at
          }
          const f = i.getBoundingClientRect()
          const t = i.parentElement.getBoundingClientRect()
          return { x: Math.ceil(f.right) + 2, y: Math.round(t.top + t.height / 2), width: 40, track: window.look.ground(i.parentElement).slice(0, 3) }
        }, at)
        const png = await page.screenshot({ clip: { x: strip.x, y: strip.y, width: strip.width, height: 1 }, caret: 'hide' })
        const lit = await page.evaluate(async ({ b64, track }) => {
          const bitmap = await createImageBitmap(new Blob([Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))], { type: 'image/png' }))
          const c = new OffscreenCanvas(bitmap.width, bitmap.height).getContext('2d')
          c.drawImage(bitmap, 0, 0)
          const d = c.getImageData(0, 0, bitmap.width, bitmap.height).data
          let most = 0
          for (let k = 0; k < d.length; k += 4) most = Math.max(most, ...[0, 1, 2].map((n) => Math.abs(d[k + n] - track[n])))
          return most
        }, { b64: png.toString('base64'), track: strip.track })
        if (lit > 4) {
          wrong.push(`the band runs on past the fill onto the track, ${Math.round(at * 100)} % of the way through it changes the track by ${lit} of 255`)
          break
        }
      }
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
