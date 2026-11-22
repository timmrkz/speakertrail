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
      // Someone connected is only under Connected.
      await page.locator('main .filters button', { hasText: 'Connected' }).click()
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
    name: `In ${theme}, every button and list to pick from is as wide as what it says now, at 1280 and at 390 px`,
    async run(page) {
      // Tim found Run, Add, Check and the LinkedIn profile list wider than
      // their words, twice, testing #46. A button is as wide as what it
      // says now, and a list as wide as what is picked, up to --pick-max.
      const wrong = []
      // What each needs is added up from what it shows: the words, set
      // alone in its font, each icon, the gaps between them, its padding
      // and its line. Nothing hidden counts, so room kept for words it
      // does not say now is caught.
      const measure = () => [...document.querySelectorAll('main .btn:not(.icon), main .pick, .report .btn:not(.icon)')]
        .filter((e) => e.getClientRects().length && getComputedStyle(e).visibility !== 'hidden')
        .map((e) => {
          const said = (n) => {
            if (n.nodeType === Node.TEXT_NODE) return n.textContent
            if (n.nodeType !== Node.ELEMENT_NODE || n.getAttribute('aria-hidden') === 'true') return ''
            return [...n.childNodes].map(said).join('')
          }
          const words = (text) => {
            const probe = document.createElement('span')
            probe.textContent = text.trim()
            probe.style.cssText = 'position: absolute; visibility: hidden; white-space: nowrap'
            e.append(probe)
            const w = probe.getBoundingClientRect().width
            probe.remove()
            return w
          }
          const shows = (n) => n.nodeType === Node.TEXT_NODE ? n.textContent.trim() !== ''
            : n.nodeType === Node.ELEMENT_NODE && n.getClientRects().length > 0 && getComputedStyle(n).position !== 'absolute'
          const parts = [...e.childNodes].filter(shows).map((n) => {
            const text = n.nodeType === Node.TEXT_NODE ? n.textContent : n.matches('svg') ? '' : said(n)
            return text.trim() ? words(text) : n.getBoundingClientRect().width
          })
          const s = getComputedStyle(e)
          let own = parts.reduce((a, b) => a + b, 0) + (parseFloat(s.columnGap) || 0) * Math.max(0, parts.length - 1)
            + parseFloat(s.paddingLeft) + parseFloat(s.paddingRight) + parseFloat(s.borderLeftWidth) + parseFloat(s.borderRightWidth)
          if (e.classList.contains('pick')) {
            const probe = document.createElement('i')
            probe.style.cssText = 'position: absolute; display: block; width: var(--pick-max)'
            e.append(probe)
            own = Math.min(own, probe.getBoundingClientRect().width)
            probe.remove()
          }
          return { what: (e.getAttribute('aria-label') || said(e)).trim().replace(/\s+/g, ' '), now: e.getBoundingClientRect().width, own }
        })
        .filter((b) => b.now > b.own + 1)
        .map((b) => `${b.what} is ${Math.round(b.now)} px wide, where what it shows needs ${Math.round(b.own)} px`)
      for (const size of [{ width: 1280, height: 860 }, { width: 390, height: 844 }]) {
        await page.setViewportSize(size)
        for (const [where, ready] of [
          ['/app/people', 'main button.start'],
          ['/app/searches', 'main button.btn.primary'],
          ['/app/settings', '#linkedin-profile:enabled'],
          ['/app/updates', 'main button.act'],
          ['/report', '.report button.btn.primary'],
        ]) {
          await page.goto(`${url}${where}`)
          await page.locator(ready).first().waitFor()
          if (where === '/app/people') {
            await page.locator('.prow').first().click()
            await page.locator('.person .connect').waitFor()
          }
          for (const w of await page.evaluate(measure)) wrong.push(`at ${size.width} px on ${where}: ${w}`)
        }
      }
      return wrong.join('\n\t')
    },
  })
  walks.push({
    theme,
    name: `In ${theme}, every button in focus wears a ring in its own colour`,
    async run(page) {
      // Tim found Clear, red, with the accent's ring of Report beside it,
      // and plain buttons in the accent too, testing #46. A ring is the
      // colour of the button's words, or of its fill when the words sit
      // on the accent: a primary button and a switch that is on.
      const wrong = []
      const rings = () => [...document.querySelectorAll('main :is(.btn, .chip, .switch, .pick), .report :is(.btn, .pick)')]
        .filter((e) => e.checkVisibility() && !e.disabled && !e.querySelector(':scope > .beam') && getComputedStyle(e).visibility !== 'hidden')
        .map((e) => {
          e.focus({ focusVisible: true })
          const s = getComputedStyle(e)
          const off = e.getAttribute('aria-disabled') === 'true'
          const filled = !off && (e.matches('.btn.primary') || e.matches('.switch[aria-checked="true"]'))
          const want = filled ? s.backgroundColor : s.color
          const what = (e.getAttribute('aria-label') || e.textContent).trim().replace(/\s+/g, ' ')
          const got = s.outlineStyle === 'none' ? 'no ring' : s.outlineColor
          e.blur()
          return got === want ? '' : `${what} wears ${got}, its own colour is ${want}`
        })
        .filter(Boolean)
      for (const [where, ready] of [
        ['/app/people', 'main button.start'],
        ['/app/searches', 'main button.btn.primary'],
        ['/app/settings', '#linkedin-profile:enabled'],
        ['/app/updates', 'main button.act'],
        ['/report', '.report button.make'],
      ]) {
        await page.goto(`${url}${where}`)
        await page.locator(ready).first().waitFor()
        if (where === '/app/people') {
          await page.locator('.prow').first().click()
          await page.locator('.person .connect').waitFor()
        }
        for (const w of await page.evaluate(rings)) wrong.push(`on ${where}: ${w}`)
        if (where === '/report') {
          await page.locator('textarea.telling').fill('It froze.')
          for (const w of await page.evaluate(rings)) wrong.push(`on ${where} with words: ${w}`)
        }
      }
      return wrong.join('\n\t')
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

// The gaps are the same in both themes, so this walk runs once.
walks.push({
  theme: 'light',
  name: 'Two controls side by side or one over the other stand --gap-control apart, and the items sharing one ground --gap-inset, at 1280 and at 390 px',
  async run(page) {
    // Tim found the page under the pointer in the sidebar touching the
    // page chosen, the People filter's segments touching, and buttons in
    // rows 6, 8 or 10 px apart, testing #46, issue #63. Every row of
    // controls on a page is measured: the space between each two
    // neighbours, on one line or one over the other, as drawn. A margin
    // or a gap of its own shows as well as a wrong gap.
    const gaps = () => {
      const px = (t) => {
        const probe = document.createElement('i')
        probe.style.cssText = `position: absolute; width: var(${t})`
        document.body.append(probe)
        const w = probe.getBoundingClientRect().width
        probe.remove()
        return w
      }
      const control = px('--gap-control')
      const inset = px('--gap-inset')
      // A control, or a wrapper round one alone. An info mark is help,
      // not a control, and Tab passes it by.
      const is = 'button:not([tabindex="-1"]), a.btn, a.nav, input, textarea, .pick, .search'
      const shown = (e) => e.checkVisibility({ visibilityProperty: true }) && getComputedStyle(e).position !== 'absolute'
      const name = (e) => (e.getAttribute('aria-label') || e.textContent || e.getAttribute('placeholder') || e.tagName).trim().replace(/\s+/g, ' ').slice(0, 30)
      const wrong = []
      for (const box of document.querySelectorAll('main *, aside *, .report *, dialog[open] *, .sheet *')) {
        const d = getComputedStyle(box).display
        if (!/flex|grid/.test(d) || !shown(box)) continue
        // Only two controls with nothing between them are neighbours. The
        // words between Clear and Attach in Report take the room there.
        const kids = [...box.children].filter(shown)
        // A row of controls counts as one too, like the search, the order
        // and Run over the filter on People, so two rows of controls stand
        // apart as two controls do. The sidebar's parts are measured below.
        const one = (k) => k.matches(is) || (k.children.length === 1 && k.firstElementChild.matches(is))
        const row = (k) => !k.closest('aside') && /flex|grid/.test(getComputedStyle(k).display)
          && [...k.children].filter(shown).length > 1 && [...k.children].filter(shown).every(one)
        const ctl = (k) => one(k) || row(k)
        const want = box.matches('.segmented') || box.closest('aside') ? inset : control
        for (let i = 0; i + 1 < kids.length; i++) {
          if (!ctl(kids[i]) || !ctl(kids[i + 1])) continue
          const a = kids[i].getBoundingClientRect()
          const b = kids[i + 1].getBoundingClientRect()
          const [first, second] = getComputedStyle(box).flexDirection === 'row-reverse' ? [b, a] : [a, b]
          const across = Math.min(first.bottom, second.bottom) - Math.max(first.top, second.top) > 0
          const got = across ? second.left - first.right : second.top - first.bottom
          if (Math.abs(got - want) > 0.5) wrong.push(`${name(kids[i])} and ${name(kids[i + 1])} stand ${got.toFixed(1)} px apart, not ${want} px`)
        }
      }
      // The button that opens the sidebar stands over the first page in
      // it as the pages stand over each other.
      const head = document.querySelector('aside .head .glyph')?.getBoundingClientRect()
      const first = document.querySelector('aside .nav')?.getBoundingClientRect()
      if (head && first && Math.abs(first.top - head.bottom - inset) > 0.5) {
        wrong.push(`the sidebar's button stands ${(first.top - head.bottom).toFixed(1)} px over People, not ${inset} px`)
      }
      return wrong
    }
    const wrong = []
    for (const size of [{ width: 1280, height: 860 }, { width: 390, height: 844 }]) {
      await page.setViewportSize(size)
      for (const [where, ready] of [
        ['/app/people', 'main button.start'],
        ['/app/searches', 'main button.btn.primary'],
        ['/app/settings', '#linkedin-profile:enabled'],
        ['/app/updates', 'main button.act'],
        ['/report', '.report button.btn.primary'],
      ]) {
        await page.goto(`${url}${where}`)
        await page.locator(ready).first().waitFor()
        // The template comes after the rest of Settings, with its first
        // part open.
        if (where === '/app/settings') await page.locator('.variation').first().waitFor()
        for (const w of await page.evaluate(gaps)) wrong.push(`at ${size.width} px on ${where}: ${w}`)
        // What opens on a page has rows of controls too: the exclusions
        // on Searches, and the question before removing the log.
        // A person open, in the pane at 1280 px and in the sheet at 390,
        // with Connect, Draft, LinkedIn and Not this person in one row.
        if (where === '/app/people') {
          await page.locator('.prow').first().click()
          await page.locator('.person .connect').waitFor()
          for (const w of await page.evaluate(gaps)) wrong.push(`at ${size.width} px on ${where} with a person open: ${w}`)
        }
        if (where === '/app/searches') {
          await page.locator('main summary', { hasText: 'Exclusions' }).click()
          await page.locator('main .exclusions .add').waitFor()
          for (const w of await page.evaluate(gaps)) wrong.push(`at ${size.width} px on ${where} with the exclusions open: ${w}`)
        }
        if (where === '/app/settings') {
          await page.locator('main button[aria-label="Remove the app\'s log"]').click()
          await page.locator('dialog[open]').waitFor()
          for (const w of await page.evaluate(gaps)) wrong.push(`at ${size.width} px on ${where} asking before removing: ${w}`)
        }
      }
    }
    return wrong.join('\n\t')
  },
})

// The insets are the same in both themes, so this walk runs once.
walks.push({
  theme: 'light',
  name: 'Every page stands --gap-page in from its edges and between its sections, and a person lines up with the list beside it',
  async run(page) {
    // Tim marked them on a screenshot testing #60, 21 November 2026: the
    // space over the search field was not the space between the filter
    // and the list, the person stood further in from its edge than the
    // list did, and the person's buttons sat lower than the filter.
    const token = (t) => page.evaluate((t) => {
      const probe = document.createElement('i')
      probe.style.cssText = `position: absolute; width: var(${t})`
      document.body.append(probe)
      const w = probe.getBoundingClientRect().width
      probe.remove()
      return w
    }, t)
    const wrong = []
    const near = (what, got, want) => { if (Math.abs(got - want) > 0.5) wrong.push(`${what} is ${got.toFixed(1)} px, not ${want} px`) }
    for (const size of [{ width: 1280, height: 860 }, { width: 390, height: 844 }]) {
      await page.setViewportSize(size)
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      const gap = await token('--gap-page')
      const pad = await token('--pad')
      const control = await token('--gap-control')
      const box = (sel) => page.locator(sel).first().evaluate((e) => { const r = e.getBoundingClientRect(); const b = parseFloat(getComputedStyle(e).borderLeftWidth); return { top: r.top, bottom: r.bottom, left: r.left + b, right: r.right, height: r.height } })
      const at = `at ${size.width} px`
      const pane = await box('.list-pane')
      const search = await box('.controls > :first-child')
      const filter = await box('.controls > :nth-child(2)')
      const list = await box('.rows-wrap .list')
      near(`${at}, the space over the search field`, search.top - pane.top, gap)
      near(`${at}, the space between the filter and the list`, list.top - filter.bottom, gap)
      if (size.width === 1280) {
        await page.locator('.prow').first().click()
        await page.locator('.person .connect').waitFor()
        // With a person open the list is narrower than its column, so
        // it stands --pad in, as the person does.
        const narrowed = await box('.list-pane')
        const listNow = await box('.rows-wrap .list')
        near(`${at}, the list's inset on the right with a person open`, narrowed.right - listNow.right, pad)
        const detail = await box('.detail-pane')
        const name = await box('.detail-top .person-name')
        const row = await box('.person .actions')
        const next = await box('.person > :nth-child(2)')
        near(`${at}, the person's inset on the left`, row.left - detail.left, pad)
        near(`${at}, the name's middle against the search field's`, (name.top + name.bottom) / 2, (search.top + search.bottom) / 2)
        near(`${at}, the top of the person's buttons against the filter's`, row.top, filter.top)
        near(`${at}, the height of the person's buttons against the filter's`, row.height, filter.height)
        near(`${at}, the top of what follows the buttons against the list's`, next.top, list.top)
      } else {
        await page.locator('.prow').first().click()
        await page.locator('.sheet .person .connect').waitFor()
        const sheet = await box('.sheet')
        const bar = await box('.sheet-bar')
        const row = await box('.sheet .person .actions')
        near(`${at}, the space between the name and the person's buttons in the sheet`, row.top - bar.bottom, control)
        near(`${at}, the person's inset in the sheet`, row.left - sheet.left, pad)
      }
      for (const [where, ready] of [['/app/searches', 'main button.btn.primary'], ['/app/settings', '#linkedin-profile:enabled'], ['/app/updates', 'main button.act']]) {
        await page.goto(`${url}${where}`)
        await page.locator(ready).first().waitFor()
        const m = await page.evaluate(() => {
          const main = document.querySelector('main').getBoundingClientRect()
          const kids = [...document.querySelector('.page-body').children].filter((k) => k.getBoundingClientRect().height > 0).map((k) => k.getBoundingClientRect())
          return { top: kids[0].top - main.top, left: kids[0].left - main.left, gaps: kids.slice(1).map((k, i) => k.top - kids[i].bottom) }
        })
        near(`${at} on ${where}, the space over the first control`, m.top, gap)
        if (size.width === 390) near(`${at} on ${where}, the inset on the left`, m.left, pad)
        m.gaps.forEach((g, i) => near(`${at} on ${where}, the space over section ${i + 2}`, g, gap))
      }
    }
    return wrong.join('\n\t')
  },
})

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
