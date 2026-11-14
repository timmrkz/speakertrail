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
