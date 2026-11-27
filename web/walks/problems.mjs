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
