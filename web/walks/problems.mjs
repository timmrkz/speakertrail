// Walks what goes wrong against the mock, in Chromium: each control that
// fails says so at itself, in words that say what to do, never in a toast
// over the window (Tim, testing #60 on 22 November 2026, #72). The app
// refuses with page.route, as it does when it cannot write its file. A
// person's row of buttons is walked in walks/person.mjs, the first
// message in walks/draft.mjs. `make walks` runs them, and so does CI.
import { chromium } from 'playwright'
import { url, server } from './lib/mock.mjs'

const refuse = (page, glob, method, error = 'The file could not be written.') =>
  page.route(glob, (route) => route.request().method() === method
    ? route.fulfill({ status: 500, json: { error } })
    : route.fallback())

// What a line says, the Report beside it included.
const words = async (line) => (await line.innerText()).trim()

const walks0 = [
  {
    name: 'Pausing a search the app refuses says so on its row, with Report and no toast, and goes with the next click',
    async run(page) {
      await refuse(page, '**/api/sources/*', 'PATCH')
      await page.goto(`${url}/app/searches`)
      await page.locator('li.search').first().waitFor()
      // The row by its words, since a paused search moves down the list.
      const query = await page.locator('li.search').first().locator('b').innerText()
      const first = page.locator('li.search').filter({ has: page.locator(`b:text-is("${query}")`) })
      await first.locator('button.toggle').click()
      const line = first.locator('.problem')
      await line.waitFor()
      if ((await words(line)) !== 'The file could not be written. Report') return `the row says ${JSON.stringify(await words(line))}`
      if (await page.locator('.toast').count()) return 'a toast shows too'
      if ((await first.locator('button.toggle').innerText()) !== 'Pause') return 'the search shows as paused'
      if ((await page.locator('.problem').count()) !== 1) return 'it shows somewhere else too'
      await page.unroute('**/api/sources/*')
      await first.locator('button.toggle').click()
      await first.locator('button.toggle', { hasText: 'Resume' }).waitFor()
      if (await line.count()) return 'the line stays after pausing worked'
      // Put back as it was, for the walks after.
      await first.locator('button.toggle').click()
      await first.locator('button.toggle', { hasText: 'Pause' }).waitFor()
      return ''
    },
  },
  {
    name: 'Removing an exclusion the app refuses says so under its field, and a search typed with no words says what it needs there',
    async run(page) {
      await refuse(page, '**/api/exclusions/*', 'DELETE')
      await page.goto(`${url}/app/searches`)
      await page.locator('details.exclusions summary').click()
      const chip = page.locator('li.phrase').first()
      await chip.waitFor()
      await chip.locator('button.remove').click()
      const line = page.locator('details.exclusions .problem')
      await line.waitFor()
      if ((await words(line)) !== 'The file could not be written. Report') return `the line says ${JSON.stringify(await words(line))}`
      if (await page.locator('.toast').count()) return 'a toast shows too'
      if (!(await chip.isVisible())) return 'the exclusion went'
      await page.locator('form.add button[type="submit"]').first().click()
      const needs = page.locator('#search-err')
      await needs.waitFor()
      if ((await words(needs)) !== 'Enter a kind of person and a place, like Yogalehrerin Köln') return `the search says ${JSON.stringify(await words(needs))}`
      // A search there already is not something that went wrong, so it
      // offers no Report.
      const query = await page.locator('li.search b').first().innerText()
      await page.locator('#new-search').fill(query)
      await page.locator('form.add button[type="submit"]').first().click()
      await needs.filter({ hasText: 'there already' }).waitFor()
      if (await needs.getByRole('button', { name: 'Report' }).count()) return 'a search there already offers Report'
      return ''
    },
  },
  {
    name: 'Run the app refuses says why under Run, with no toast, and goes once a run starts',
    async run(page) {
      await page.route('**/api/runs', (route) => route.request().method() === 'POST'
        ? route.fulfill({ status: 409, json: { error: 'Paste the Exa key in Settings first.' } })
        : route.fallback())
      await page.goto(`${url}/app/people`)
      const run = page.locator('.controls button.start', { hasText: 'Run' })
      await run.click()
      const line = page.locator('.controls .problem')
      await line.waitFor()
      // Theirs to put right, so no Report.
      if ((await words(line)) !== 'Paste the Exa key in Settings first.') return `the line says ${JSON.stringify(await words(line))}`
      if (await page.locator('.toast').count()) return 'a toast shows too'
      if ((await run.innerText()).trim() !== 'Run') return `the button says ${await run.innerText()}`
      await page.unroute('**/api/runs')
      await run.click()
      await line.waitFor({ state: 'detached' })
      await page.locator('.controls button.start', { hasText: 'Stop' }).click()
      await page.locator('.controls button.start', { hasText: 'Run' }).waitFor()
      return ''
    },
  },
  {
    // Undo in the Edit menu has no control of its own, so what went wrong
    // with it is a toast, as Tim chose on 27 November 2026.
    name: 'Undo in the Edit menu the app refuses says so in a toast',
    async run(page) {
      await page.route('**/api/undo', (route) => route.fulfill({ status: 500, json: { error: 'The file could not be written.' } }))
      await page.goto(`${url}/app/searches`)
      await page.locator('li.search').first().waitFor()
      await page.evaluate(() => window._wails.dispatchWailsEvent({ name: 'undo', data: 'undo' }))
      await page.locator('.toast.bad', { hasText: 'The file could not be written.' }).waitFor()
      if (await page.locator('.problem').count()) return 'it shows at a control too'
      return ''
    },
  },
]

const walks = walks0.filter((w) => !process.env.ONLY || w.name.includes(process.env.ONLY))
const browser = await chromium.launch()
let failed = 0
for (const w of walks) {
  const context = await browser.newContext({ viewport: w.viewport ?? { width: 1440, height: 860 } })
  const page = await context.newPage()
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
  await context.close()
}
await browser.close()
await server.close()
process.exit(failed ? 1 : 0)
