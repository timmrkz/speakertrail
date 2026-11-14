// Walks through the person view against the mock, in Chromium, the way
// walks/sidebar.mjs does. `make walks` runs them, and so does CI.
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

const review = (page, id) => page.evaluate((id) => fetch(`/api/people/${id}`).then((r) => r.json()).then((p) => p.profiles[0].review), id)

const walks = [
  {
    // Tim saw the list reshuffle as a person came in: rows went from one
    // line to two, the toolbar broke into lines and the list jumped.
    name: 'Opening a person narrows the list and moves nothing in it',
    async run(page) {
      const look = () => page.evaluate(() => {
        const rows = [...document.querySelectorAll('.prow')]
        return JSON.stringify({
          rows: rows.map((r) => Math.round(r.getBoundingClientRect().top)),
          areas: getComputedStyle(rows[0]).gridTemplateAreas,
          controls: [...document.querySelectorAll('.controls > *, .controls .toolbar > *')].map((c) => Math.round(c.getBoundingClientRect().top)),
        })
      })
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      const closed = await look()
      await page.locator('.prow').nth(2).click()
      await page.locator('.detail-pane').waitFor()
      await page.waitForTimeout(0)
      await page.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {}))))
      const open = await look()
      if (open !== closed) return `the list changed: closed ${closed}, open ${open}`
      return ''
    },
  },
  {
    // A search can find someone else's LinkedIn profile. Not this person
    // sets it aside, and the toast's Undo brings it back as it was.
    name: 'Not this person sets the LinkedIn profile aside, and Undo brings it back',
    async run(page) {
      await page.goto(`${url}/app/people/1001`)
      const linkedin = page.locator('.actions a.btn')
      await linkedin.waitFor()
      const before = await review(page, 1001)
      await page.getByRole('button', { name: 'Not this person' }).click()
      await linkedin.waitFor({ state: 'detached' })
      if ((await review(page, 1001)) !== 'rejected') return 'the profile was not set aside'
      await page.locator('.toast').getByRole('button', { name: 'Undo' }).click()
      await linkedin.waitFor()
      const after = await review(page, 1001)
      if (after !== before) return `Undo left the profile ${after}, it was ${before}`
      // Once the toast is gone, the line under the buttons undoes it.
      await page.getByRole('button', { name: 'Not this person' }).click()
      await linkedin.waitFor({ state: 'detached' })
      await page.locator('.set-aside').getByRole('button', { name: 'Undo' }).click()
      await linkedin.waitFor()
      if ((await review(page, 1001)) === 'rejected') return 'the line\'s Undo left the profile set aside'
      return ''
    },
  },
  {
    // Without a choice of where LinkedIn opens, the app says so with a
    // field, and the line leads to Settings. The button is itself again
    // as soon as the answer is in.
    name: 'LinkedIn with nothing chosen leads to Settings',
    async run(page) {
      await page.goto(`${url}/app/people/1002`)
      const linkedin = page.locator('.actions a.btn')
      await linkedin.click()
      const line = page.locator('.open-problem')
      await line.waitFor()
      if ((await line.getByRole('link', { name: 'Settings' }).count()) !== 1) return `the line says ${await line.innerText()}`
      const text = (await linkedin.innerText()).trim()
      if (text !== 'LinkedIn') return `the button says ${text} once the answer is in`
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
