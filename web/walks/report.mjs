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
    // the answer to the remove comes while the put back is on its way,
    // and Clear right after takes it, and Cmd+Z brings the words and the
    // picture back. The answer to the remove once took the picture away
    // again, and Clear then had nothing to bring back.
    name: 'A picture put back stays, and Undo after Clear brings it back',
    async run(page) {
      await openReport(page)
      await page.locator('textarea.telling').click()
      await page.keyboard.type('It froze.')
      await page.locator('button.attach').click()
      await page.locator('.pictures figure').waitFor()
      // The put back is done at once, and its answer held until the
      // remove is answered and Clear is pressed, so it comes last.
      const [cleared, clear] = later()
      await page.route('**/api/report/put-back', async (route) => {
        const answer = await route.fetch()
        await cleared
        await route.fulfill({ response: answer })
      })
      await page.locator('.tile').hover()
      const removed = page.waitForResponse((r) => r.url().endsWith('/api/report/remove'))
      await page.locator('.tile .bin').click()
      await page.getByRole('button', { name: 'Put it back' }).click()
      await removed
      await page.waitForFunction(() => document.querySelectorAll('.pictures figure').length === 1)
      await page.locator('button.clear').click()
      await page.locator('dialog[open] button.danger').click()
      await page.waitForResponse((r) => r.url().endsWith('/api/report/clear'))
      clear()
      await page.waitForResponse((r) => r.url().endsWith('/api/report/put-back'))
      await page.unroute('**/api/report/put-back')
      if ((await page.locator('.pictures figure').count()) !== 0) return 'Clear left a picture'
      const putBack = page.waitForResponse((r) => r.url().endsWith('/api/report/put-back'))
      await page.keyboard.press('ControlOrMeta+z')
      await putBack
      const text = await page.locator('textarea.telling').inputValue()
      const pictures = await page.locator('.pictures figure').count()
      if (text !== 'It froze.' || pictures !== 1) return `after Undo the field reads ${JSON.stringify(text)} with ${pictures} pictures, want the words and 1`
      const draft = await page.evaluate(() => fetch('/api/report').then((r) => r.json()))
      if (draft.pictures.length !== 1) return `the draft holds ${draft.pictures.length} pictures after Undo`
      return ''
    },
  },
  {
    // Report says Making while it makes the report, in the room it had,
    // and then the field is empty for the next one.
    name: 'Report keeps its width while it makes the report',
    async run(page) {
      await openReport(page)
      await page.locator('textarea.telling').click()
      await page.keyboard.type('It froze.')
      const button = page.locator('button.make')
      const before = (await button.boundingBox()).width
      const [made, make] = later()
      await page.route('**/api/report/send', async (route) => {
        await made
        await route.continue()
      })
      await button.click()
      await page.locator('button.make .beam').waitFor()
      const during = (await button.boundingBox()).width
      make()
      await page.waitForResponse((r) => r.url().endsWith('/api/report/send'))
      await page.waitForFunction(() => document.querySelector('textarea.telling').value === '')
      if (during !== before) return `Report is ${before} px wide and ${during} px while making`
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
