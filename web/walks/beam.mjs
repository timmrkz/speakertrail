// Walks over every control that carries the beam of work in hand, against
// the mock, in Chromium, the way walks/logs.mjs does: Settings, Updates,
// People, a person and Report a Problem. The light runs on the line round
// the control, not a pixel inside it, which Tim saw testing #24 on 17
// November 2026, issue #29. Ported from FrameFairy's beamsOff, its #795.
// Each walk holds the call the control waits on, so nothing waits on the
// clock. `make walks` runs them, and so does CI.
import { chromium } from 'playwright'
import { url, server } from './lib/mock.mjs'

// What is wrong with where the beam of work in hand lies, for every beam
// on screen, said in words, or nothing when every beam is right. The
// light runs on the line round the control, not a pixel inside it: on its
// border, going round its corners. The fill and the motes lie inside that
// line, with corners that are the inside of its corners. And nothing
// round the beam cuts its light off: a control that clips at the inside
// of its line would hide the light on it, which a box in the right place
// says nothing about. FrameFairy's beamsOff, for the controls this app
// has, which are all buttons with a border of their own.
function beamsOff(page) {
  return page.evaluate(() => {
    const off = []
    const px = (v) => parseFloat(v) || 0
    const near = (a, b) => Math.abs(a - b) < 0.05
    for (const beam of document.querySelectorAll('.beam')) {
      const ring = beam.querySelector(':scope > .ring')
      if (!ring) continue
      const host = beam.parentElement
      const h = host.getBoundingClientRect()
      const hs = getComputedStyle(host)
      const name = `${host.tagName.toLowerCase()} "${(host.getAttribute('aria-label') ?? host.textContent).trim().slice(0, 40)}"`
      const line = { left: h.left, right: h.right, top: h.top, bottom: h.bottom }
      const corners = [px(hs.borderTopLeftRadius), px(hs.borderBottomLeftRadius)]
      const width = px(hs.borderLeftWidth)
      const g = ring.getBoundingClientRect()
      const rs = getComputedStyle(ring)
      // Whatever clips what is in it, the control itself and everything
      // round it, clips at the inside of its border, and the line must
      // lie inside that.
      for (let el = host; el && el !== document.documentElement; el = el.parentElement) {
        const es = getComputedStyle(el)
        if (es.overflowX === 'visible' && es.overflowY === 'visible') continue
        const e = el.getBoundingClientRect()
        const clip = { left: e.left + px(es.borderLeftWidth), right: e.right - px(es.borderRightWidth), top: e.top + px(es.borderTopWidth), bottom: e.bottom - px(es.borderBottomWidth) }
        if (line.left < clip.left - 0.05 || line.right > clip.right + 0.05 || line.top < clip.top - 0.05 || line.bottom > clip.bottom + 0.05) {
          off.push(`the beam of ${name} is cut off by ${el === host ? 'the control itself' : `${el.tagName.toLowerCase()}.${[...el.classList].join('.')}`}, which clips at the inside of its line`)
          break
        }
      }
      if (!['left', 'right', 'top', 'bottom'].every((k) => near(g[k], line[k]))) {
        off.push(`the beam of ${name} runs at ${['left', 'top', 'right', 'bottom'].map((k) => g[k].toFixed(2)).join(' ')}, where its line is ${['left', 'top', 'right', 'bottom'].map((k) => line[k].toFixed(2)).join(' ')}`)
      } else if (!near(px(rs.borderTopLeftRadius), corners[0]) || !near(px(rs.borderBottomLeftRadius), corners[1])) {
        off.push(`the beam of ${name} turns its corners at ${rs.borderTopLeftRadius} and ${rs.borderBottomLeftRadius}, where its line turns at ${corners[0]}px and ${corners[1]}px`)
      } else if (!near(px(rs.paddingTop), Math.max(1, width))) {
        off.push(`the beam of ${name} is ${rs.paddingTop} thick on a line of ${width}px`)
      }
      // The inside of the line, and its corners, which the browser rounds
      // by the corner less the line.
      const within = { left: line.left + width, right: line.right - width, top: line.top + width, bottom: line.bottom - width }
      const corner = Math.max(0, corners[0] - width)
      for (const inside of beam.querySelectorAll(':scope > .fill, :scope > .motes')) {
        const f = inside.getBoundingClientRect()
        const fs = getComputedStyle(inside)
        const room = { left: f.left + px(fs.borderLeftWidth), right: f.right - px(fs.borderRightWidth), top: f.top + px(fs.borderTopWidth), bottom: f.bottom - px(fs.borderBottomWidth) }
        const turn = Math.max(0, px(fs.borderTopLeftRadius) - px(fs.borderLeftWidth))
        if (!['left', 'right', 'top', 'bottom'].every((k) => near(room[k], within[k])) || !near(turn, corner)) {
          off.push(`the ${inside.classList[0]} of ${name} fills ${['left', 'top', 'right', 'bottom'].map((k) => room[k].toFixed(2)).join(' ')} with corners of ${turn}px, where the inside of its line is ${['left', 'top', 'right', 'bottom'].map((k) => within[k].toFixed(2)).join(' ')} with corners of ${corner}px`)
        }
      }
    }
    return off
  })
}

// One promise and the hand that keeps it.
function later() {
  let keep
  const p = new Promise((r) => (keep = r))
  return [p, keep]
}

// Holds the call to path until the hand it gives back lets it go.
async function hold(page, path) {
  const [held, letGo] = later()
  await page.route(`**${path}`, async (route) => {
    await held
    await route.continue().catch(() => {})
  })
  return letGo
}

// Waits for the beam in the control, then says what is wrong with every
// beam on screen.
async function measured(page, control, what) {
  await control.locator(':scope > .beam > .ring').waitFor()
  const off = await beamsOff(page)
  if (off.length === 0 && (await page.locator('.beam > .ring').count()) === 0) return `${what} shows no beam`
  return off.length ? `while ${what} works, ${off.join(', and ')}` : ''
}

const walks = [
  {
    // The speech model's download carries the beam and the fill in Cancel.
    name: 'Settings: the beam of a speech model downloading runs on the line round Cancel',
    viewport: { width: 1280, height: 900 },
    async run(page) {
      await page.route('**/api/speech', async (route) => {
        const res = await route.fetch()
        const body = await res.json()
        body.models = body.models.map((m) => ({ ...m, installed: false, inUse: false }))
        body.job = { id: 'speech-1', kind: 'model', title: body.models[0].title, state: 'running', progress: { text: 'fetching', fraction: 0.4, remaining: 30 } }
        await route.fulfill({ response: res, json: body })
      })
      await page.goto(`${url}/app/settings`)
      const cancel = page.locator('.speech button.act')
      await cancel.locator(':scope > .beam > .fill').waitFor()
      return measured(page, cancel, 'the speech model downloads')
    },
  },
  {
    // Download carries the beam while it waits for the download to start.
    name: 'Settings: the beam of Download starting runs on its line',
    viewport: { width: 1280, height: 900 },
    async run(page) {
      await page.route('**/api/speech', async (route) => {
        const res = await route.fetch()
        const body = await res.json()
        body.models = body.models.map((m) => ({ ...m, installed: false, inUse: false }))
        body.job = null
        await route.fulfill({ response: res, json: body })
      })
      const letGo = await hold(page, '/api/speech/install')
      await page.goto(`${url}/app/settings`)
      // Found by its place, since its words change to Starting.
      const download = page.locator('.speech button.act')
      await download.filter({ hasText: 'Download' }).waitFor()
      await download.click()
      const wrong = await measured(page, download, 'Download')
      letGo()
      return wrong
    },
  },
  {
    name: 'Updates: the beam of Check runs on its line',
    viewport: { width: 1280, height: 900 },
    async run(page) {
      const letGo = await hold(page, '/api/updates/check')
      await page.goto(`${url}/app/updates`)
      // Found by its place, since its words change to Checking.
      const check = page.locator('main button.act')
      await check.filter({ hasText: 'Check' }).waitFor()
      await check.click()
      const wrong = await measured(page, check, 'Check')
      letGo()
      return wrong
    },
  },
  {
    // The run carries the beam and its fill in Stop.
    name: 'People: the beam of a run runs on the line round Stop',
    viewport: { width: 390, height: 844 },
    async run(page) {
      await page.goto(`${url}/app/people`)
      const start = page.locator('button.start')
      await start.click()
      await start.locator(':scope > .beam > .fill').waitFor()
      const wrong = await measured(page, start, 'the run')
      await start.click()
      return wrong
    },
  },
  {
    name: 'A person: the beam of LinkedIn opening runs on its line',
    viewport: { width: 1280, height: 900 },
    async run(page) {
      const letGo = await hold(page, '/api/people/1001/linkedin')
      await page.goto(`${url}/app/people/1001`)
      // Found by its link, since its words change to Opening.
      const open = page.locator('a.btn[href*="linkedin.com"]')
      await open.click()
      const wrong = await measured(page, open, 'LinkedIn')
      letGo()
      return wrong
    },
  },
  {
    // Dictate, Attach and Report each carry the beam while they work.
    name: 'Report a Problem: the beams of Dictate, Attach and Report run on their lines',
    viewport: { width: 560, height: 640 },
    async run(page) {
      await page.goto(`${url}/report`)
      await page.evaluate(() => fetch('/api/report/send', { method: 'POST', body: '{}' }))
      await page.reload()
      await page.locator('textarea.telling').click()
      await page.keyboard.type('It froze.')

      const dictate = page.locator('button.dictate')
      await dictate.click()
      let wrong = await measured(page, dictate, 'Dictate')
      await page.keyboard.press('Escape')
      await page.waitForFunction(() => document.querySelector('button.dictate')?.getAttribute('aria-label') === 'Dictate')
      if (wrong) return wrong

      let letGo = await hold(page, '/api/report/pick')
      const attach = page.locator('button.attach')
      await attach.click()
      wrong = await measured(page, attach, 'Attach')
      letGo()
      await page.locator('.pictures figure').waitFor()
      if (wrong) return wrong

      letGo = await hold(page, '/api/report/send')
      const make = page.locator('button.make')
      await make.click()
      wrong = await measured(page, make, 'Report')
      letGo()
      return wrong
    },
  },
]

const browser = await chromium.launch()
let failed = 0
for (const w of walks) {
  const page = await browser.newPage({ viewport: w.viewport })
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
