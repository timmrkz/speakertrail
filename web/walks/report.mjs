// Walks through Report a Problem against the mock, in Chromium, the way
// walks/logs.mjs does: the report window at /report, Dictate, the
// pictures, Clear and its undo, and Report on a toast in the app's
// window. The mock hears nothing, so Stop gives one invented sentence, as
// the app gives what the speech model wrote down. `make walks` runs
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

// The report window, at its own size, with the draft the last walk left
// sent off, so every walk starts from an empty report.
async function openReport(page) {
  await page.setViewportSize({ width: 560, height: 640 })
  await page.goto(`${url}/report`)
  await page.evaluate(() => fetch('/api/report/send', { method: 'POST', body: '{}' }))
  await page.reload()
  await page.locator('textarea.telling').waitFor()
}

// One promise and the hand that keeps it.
function later() {
  let keep
  const p = new Promise((r) => (keep = r))
  return [p, keep]
}

const walks = [
  {
    // What is said goes where the cursor is, after what was typed, and
    // Enter keeps it, the way the button does.
    name: 'Dictate writes what was said into the field and Enter keeps it',
    async run(page) {
      await openReport(page)
      const field = page.locator('textarea.telling')
      await field.click()
      await page.keyboard.type('It froze.')
      const dictate = page.locator('button.dictate')
      const listening = page.waitForResponse((r) => r.url().endsWith('/api/listen'))
      await dictate.click()
      if ((await dictate.getAttribute('aria-label')) !== 'Stop') return `Dictate says ${await dictate.getAttribute('aria-label')} while it listens`
      await listening
      await page.keyboard.press('Enter')
      await page.waitForResponse((r) => r.url().endsWith('/api/listen/stop'))
      await page.waitForFunction(() => document.querySelector('button.dictate')?.getAttribute('aria-label') === 'Dictate')
      const text = await field.inputValue()
      if (text !== 'It froze. The run stopped at the third search.') return `the field reads ${JSON.stringify(text)}`
      const wide = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)
      if (wide > 0) return `the page is ${wide} px wider than the window`
      return ''
    },
  },
  {
    // A picture put back a moment after it was removed stays, even when
    // the answer to the remove comes after the put back was shown, and
    // Clear after it takes it, and Cmd+Z brings the words and the
    // picture back. The answer to the remove once took the picture away
    // again, and Clear then had nothing to bring back. The put back is
    // asked only once the remove is answered, so the app does the two in
    // the order they were pressed.
    name: 'A picture put back stays, and Undo after Clear brings it back',
    async run(page) {
      await openReport(page)
      await page.locator('textarea.telling').click()
      await page.keyboard.type('It froze.')
      await page.locator('button.attach').click()
      await page.locator('.pictures figure').waitFor()
      // The remove's answer is held until Put it back is pressed, and the
      // put back's until the page was looked at in between.
      const [pressed, press] = later()
      const [looked, look] = later()
      let putBackAsked = false
      await page.route('**/api/report/remove', async (route) => {
        const answer = await route.fetch()
        await pressed
        await route.fulfill({ response: answer })
      })
      await page.route('**/api/report/put-back', async (route) => {
        putBackAsked = true
        const answer = await route.fetch()
        await looked
        await route.fulfill({ response: answer })
      })
      await page.locator('.tile').hover()
      // The bin is FrameFairy's: 24 px, with no box of its own until the
      // pointer is on it.
      const bin = await page.locator('.tile .bin').evaluate((b) => ({ w: b.offsetWidth, h: b.offsetHeight, bg: getComputedStyle(b).backgroundColor }))
      if (bin.w !== 24 || bin.h !== 24 || bin.bg !== 'rgba(0, 0, 0, 0)') return `the bin is ${bin.w} by ${bin.h} px on ${bin.bg}`
      const removed = page.waitForResponse((r) => r.url().endsWith('/api/report/remove'))
      await page.locator('.tile .bin').click()
      await page.getByRole('button', { name: 'Put it back' }).click()
      await page.waitForTimeout(0)
      if (putBackAsked) return 'the put back was asked before the remove was answered'
      const asked = page.waitForRequest((r) => r.url().endsWith('/api/report/put-back'))
      press()
      await removed
      await asked
      const between = await page.locator('.pictures figure').count()
      const putBack = page.waitForResponse((r) => r.url().endsWith('/api/report/put-back'))
      look()
      await putBack
      await page.unroute('**/api/report/remove')
      await page.unroute('**/api/report/put-back')
      if (between !== 1) return `the answer to the remove left ${between} pictures while the put back was on its way`
      if ((await page.locator('.pictures figure').count()) !== 1) return 'the picture put back is gone'
      await page.locator('button.clear').click()
      await page.locator('dialog[open] button.danger').click()
      await page.waitForResponse((r) => r.url().endsWith('/api/report/clear'))
      if ((await page.locator('.pictures figure').count()) !== 0) return 'Clear left a picture'
      const back = page.waitForResponse((r) => r.url().endsWith('/api/report/put-back'))
      await page.keyboard.press('ControlOrMeta+z')
      await back
      const text = await page.locator('textarea.telling').inputValue()
      const pictures = await page.locator('.pictures figure').count()
      if (text !== 'It froze.' || pictures !== 1) return `after Undo the field reads ${JSON.stringify(text)} with ${pictures} pictures, want the words and 1`
      const draft = await page.evaluate(() => fetch('/api/report').then((r) => r.json()))
      if (draft.pictures.length !== 1) return `the draft holds ${draft.pictures.length} pictures after Undo`
      return ''
    },
  },
  {
    // Report says Making while it makes the report, as wide as those
    // words, and then the field is empty for the next one. A button is as
    // wide as what it says now, Tim asked testing #46.
    name: 'Report says Making while it makes the report',
    async run(page) {
      await openReport(page)
      await page.locator('textarea.telling').click()
      await page.keyboard.type('It froze.')
      const button = page.locator('button.make')
      const [made, make] = later()
      await page.route('**/api/report/send', async (route) => {
        await made
        await route.continue()
      })
      await button.click()
      await page.locator('button.make .beam').waitFor()
      const during = (await button.textContent()).trim()
      make()
      await page.waitForResponse((r) => r.url().endsWith('/api/report/send'))
      await page.waitForFunction(() => document.querySelector('textarea.telling').value === '')
      if (during !== 'Making…') return `Report says ${JSON.stringify(during)} while making`
      return ''
    },
  },
  {
    // Tab takes the same way whatever is in the field: from the field to
    // Report, so Enter, Tab, Enter still reports after dictating, then
    // the buttons from right to left, greyed out or not, past the info
    // mark, which is help and not a control, then the pictures, and back
    // round to the field. Tim set this order testing
    // #46, and found it changed with an empty field, because greyed out
    // buttons dropped out of it.
    name: 'Tab takes the same way through Report a Problem, empty, filled and after dictating',
    async run(page) {
      const where = () => page.evaluate(() => {
        const e = document.activeElement
        if (e?.matches('textarea.telling')) return 'the field'
        if (e?.matches('figure')) return 'a picture'
        return e?.getAttribute('aria-label') ?? e?.textContent?.trim() ?? 'nothing'
      })
      const walk = async (key, n) => {
        const went = []
        for (let i = 0; i < n; i++) {
          await page.keyboard.press(key)
          went.push(await where())
        }
        return went.join(', ')
      }
      const row = ['Report', 'Clear', 'Take People along', 'Attach screenshots', 'Dictate']
      await openReport(page)
      const field = page.locator('textarea.telling')
      await field.click()
      let want = [...row, 'the field'].join(', ')
      let went = await walk('Tab', row.length + 1)
      if (went !== want) return `with an empty field Tab went ${went}, want ${want}`
      // The way back is the same way.
      want = [...row].reverse().concat('the field').join(', ')
      went = await walk('Shift+Tab', row.length + 1)
      if (went !== want) return `with an empty field Shift+Tab went ${went}, want ${want}`
      await field.click()
      await page.keyboard.type('It froze.')
      await page.locator('button.attach').click()
      await page.locator('.pictures figure').waitFor()
      await field.click()
      want = [...row, 'a picture', 'the field'].join(', ')
      went = await walk('Tab', row.length + 2)
      if (went !== want) return `with words and a picture Tab went ${went}, want ${want}`
      // Dictate, Enter keeps what was said, Tab goes to Report.
      const listening = page.waitForResponse((r) => r.url().endsWith('/api/listen'))
      await page.locator('button.dictate').click()
      await listening
      await page.keyboard.press('Enter')
      await page.waitForResponse((r) => r.url().endsWith('/api/listen/stop'))
      await page.waitForFunction(() => document.querySelector('button.dictate')?.getAttribute('aria-label') === 'Dictate')
      await page.keyboard.press('Tab')
      if ((await where()) !== 'Report') return `after dictating Tab went to ${await where()}, want Report`
      return ''
    },
  },
  {
    // People goes on with one click and off with the next, kept with the
    // draft, and a report of People alone can be made. Report waits for
    // the last press, and the next report starts with People off. Tim asked for a way
    // to hand Claude his People on 22 November 2026, issue #73.
    name: 'People goes along with a report only when it is on',
    async run(page) {
      await openReport(page)
      const people = page.locator('button.people')
      if ((await people.getAttribute('aria-pressed')) !== 'false') return 'People is on in a new report'
      if ((await page.locator('button.make').getAttribute('aria-disabled')) !== 'true') return 'Report can be pressed with nothing to report'
      const kept = page.waitForResponse((r) => r.url().endsWith('/api/report/people'))
      await people.click()
      if ((await people.getAttribute('aria-pressed')) !== 'true') return 'People did not show on at once'
      await kept
      const draft = await page.evaluate(() => fetch('/api/report').then((r) => r.json()))
      if (draft.people !== true) return `the draft holds People ${draft.people}`
      await page.reload()
      await page.waitForFunction(() => document.querySelector('button.people')?.getAttribute('aria-pressed') === 'true')
      if ((await page.locator('button.make').getAttribute('aria-disabled')) !== 'false') return 'Report cannot be pressed with People on'
      const off = page.waitForResponse((r) => r.url().endsWith('/api/report/people'))
      await people.click()
      if (JSON.parse((await off).request().postData() ?? '{}').people !== false) return 'a second click did not take People off'
      // The draft says whether People goes, so Report waits for a press
      // of People still on its way, held here until Report is making.
      const [held, hold] = later()
      await page.route('**/api/report/people', async (route) => {
        await held
        await route.continue()
      })
      const order = []
      page.on('request', (r) => { if (r.url().endsWith('/api/report/send')) order.push('send') })
      page.on('response', (r) => { if (r.url().endsWith('/api/report/people')) order.push('kept') })
      await people.click()
      await page.locator('button.make').click()
      await page.locator('button.make .beam').waitFor()
      hold()
      await page.waitForResponse((r) => r.url().endsWith('/api/report/send'))
      if (order.join(',') !== 'kept,send') return `Report did not wait for People: ${order.join(',')}`
      await page.waitForFunction(() => document.querySelector('button.people')?.getAttribute('aria-pressed') === 'false')
      return ''
    },
  },
  {
    // Report and Clear stay greyed out and do nothing with nothing to
    // report, though Tab reaches them.
    name: 'Report and Clear do nothing with nothing to report, though Tab reaches them',
    async run(page) {
      let made = false
      await openReport(page)
      page.on('request', (r) => { if (r.url().endsWith('/api/report/send')) made = true })
      await page.locator('button.make').click({ force: true })
      await page.locator('button.clear').click({ force: true })
      await page.waitForTimeout(100)
      if (made) return 'Report made a report of nothing'
      if (await page.locator('dialog[open]').count()) return 'Clear asked to clear nothing'
      const greyed = await page.locator('button.make').evaluate((b) => getComputedStyle(b).backgroundColor === getComputedStyle(document.querySelector('button.clear')).backgroundColor)
      if (!greyed) return 'Report and Clear are not the same grey'
      return ''
    },
  },
  {
    // What went wrong in the app's window says so in a toast with Report,
    // which opens Report a Problem with the toast's words in it.
    name: 'A toast that says what went wrong offers Report',
    async run(page) {
      await page.setViewportSize({ width: 390, height: 844 })
      await page.route('**/api/log/show', (route) => route.fulfill({ status: 409, contentType: 'application/json', body: '{"error":"The log could not be shown"}' }))
      await page.goto(`${url}/app/settings`)
      await page.getByRole('button', { name: "Show the app's log in folder" }).click()
      const report = page.locator('.toast.bad button', { hasText: 'Report' })
      await report.waitFor()
      const about = page.waitForRequest((r) => r.url().endsWith('/api/report/about'))
      await report.click()
      const body = JSON.parse((await about).postData() ?? '{}')
      if (!String(body.text).includes('The log could not be shown')) return `Report sent ${JSON.stringify(body)}`
      return ''
    },
  },
  {
    // A toast that ran out unread is in Settings, Logs, and Report beside
    // it opens Report a Problem with its words, as the toast's own does.
    name: 'A toast in Settings, Logs offers Report',
    async run(page) {
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`${url}/app/settings`)
      const row = page.locator('.logged').first()
      await row.waitFor()
      await row.hover()
      const report = row.getByRole('button', { name: 'Report' })
      if ((await report.evaluate((b) => getComputedStyle(b).opacity)) !== '1') return 'Report does not show on the toast in hand'
      const about = page.waitForRequest((r) => r.url().endsWith('/api/report/about'))
      await report.click()
      const body = JSON.parse((await about).postData() ?? '{}')
      if (body.text !== 'Could not reach the app. Try again.') return `Report sent ${JSON.stringify(body)}`
      const wide = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)
      if (wide > 0) return `the page is ${wide} px wider than the window`
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
