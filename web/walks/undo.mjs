// Walks Undo and Redo in the Edit menu for searches, exclusions and
// settings against the mock, in Chromium: each change is one step, the
// page it changed opens, what came back shows, and no toast or link
// offers an Undo of its own. Tim, 22 November 2026, #71. A person's
// changes are walked in walks/person.mjs, the template's in
// walks/template.mjs. `make walks` runs them, and so does CI.
import { chromium } from 'playwright'
import { url, server } from './lib/mock.mjs'

// Undo or Redo from the Edit menu, which the app hands the page as the
// event undo, see menu.go.
const menu = (page, how) => page.evaluate((how) => window._wails.dispatchWailsEvent({ name: 'undo', data: how }), how)
const anyUndo = (page) => page.locator('.toast button, button.link').filter({ hasText: 'Undo' })
const row = (page, query) => page.locator('li.search').filter({ has: page.locator(`b:text-is("${query}")`) })
const chip = (page, phrase) => page.locator('li.phrase').filter({ has: page.locator(`span:text-is("${phrase}")`) })

async function searches(page) {
  await page.goto(`${url}/app/searches`)
  await page.locator('li.search').first().waitFor()
}

const walks0 = [
  {
    name: 'Pausing a search is taken back by Undo in the Edit menu, which opens Searches, and Redo pauses it again',
    async run(page) {
      await searches(page)
      const first = page.locator('li.search').first()
      const query = await first.locator('b').innerText()
      if ((await first.locator('button.toggle').innerText()) !== 'Pause') return 'the first search is paused from the start'
      await first.locator('button.toggle').click()
      await row(page, query).locator('button.toggle', { hasText: 'Resume' }).waitFor()
      await page.goto(`${url}/app/people`)
      await page.locator('main').waitFor()
      await menu(page, 'undo')
      await page.waitForURL('**/app/searches')
      await row(page, query).locator('button.toggle', { hasText: 'Pause' }).waitFor()
      await menu(page, 'redo')
      await row(page, query).locator('button.toggle', { hasText: 'Resume' }).waitFor()
      // Put back as it was, for the walks after.
      await menu(page, 'undo')
      await row(page, query).locator('button.toggle', { hasText: 'Pause' }).waitFor()
      return ''
    },
  },
  {
    name: 'A search added is taken out by Undo, and Redo puts it back',
    async run(page) {
      await searches(page)
      await page.locator('#new-search').fill('Floristin Bonn')
      await page.locator('#new-search').press('Enter')
      await row(page, 'Floristin Bonn').waitFor()
      await page.locator('#new-search').blur()
      await menu(page, 'undo')
      await row(page, 'Floristin Bonn').waitFor({ state: 'detached' })
      await menu(page, 'redo')
      await row(page, 'Floristin Bonn').waitFor()
      return ''
    },
  },
  {
    name: 'An exclusion removed comes back with Undo in the Edit menu, with the list open to show it, and no toast offers Undo',
    async run(page) {
      await searches(page)
      const list = page.locator('details.exclusions')
      await list.locator('summary').click()
      await chip(page, 'Sichtbarkeit').waitFor()
      await chip(page, 'Sichtbarkeit').locator('button.remove').click()
      await chip(page, 'Sichtbarkeit').waitFor({ state: 'detached' })
      if (await anyUndo(page).count()) return 'something offers Undo, which the Edit menu has now'
      await list.locator('summary').click()
      if (await list.evaluate((d) => d.open)) return 'the exclusions did not close'
      await menu(page, 'undo')
      await chip(page, 'Sichtbarkeit').waitFor()
      if (!(await list.evaluate((d) => d.open))) return 'the exclusions stayed closed, so the one that came back is not seen'
      await menu(page, 'redo')
      await chip(page, 'Sichtbarkeit').waitFor({ state: 'detached' })
      await menu(page, 'undo')
      await chip(page, 'Sichtbarkeit').waitFor()
      return ''
    },
  },
  {
    name: 'A setting changed is put back by Undo in the Edit menu, which opens Settings, and the row has no Undo of its own',
    async run(page) {
      await page.goto(`${url}/app/settings`)
      const field = page.locator('#set-profile_searches_per_run')
      await field.waitFor()
      const was = await field.inputValue()
      await field.fill('12')
      await field.press('Enter')
      await page.locator('.setting').filter({ has: field }).locator('.foot', { hasText: 'Saved' }).waitFor()
      if (await anyUndo(page).count()) return 'something offers Undo, which the Edit menu has now'
      await field.blur()
      await page.goto(`${url}/app/searches`)
      await page.locator('main').waitFor()
      await menu(page, 'undo')
      await page.waitForURL('**/app/settings')
      await page.waitForFunction((was) => document.querySelector('#set-profile_searches_per_run')?.value === was, was)
      // Redo on the page itself, where the row shows it at once.
      await menu(page, 'redo')
      await page.waitForFunction(() => document.querySelector('#set-profile_searches_per_run')?.value === '12')
      await menu(page, 'undo')
      await page.waitForFunction((was) => document.querySelector('#set-profile_searches_per_run')?.value === was, was)
      return ''
    },
  },
  {
    name: 'The browser profile LinkedIn opens in is put back by Undo in the Edit menu',
    async run(page) {
      await page.goto(`${url}/app/settings`)
      const state = page.locator('#linkedin .state')
      await state.filter({ hasText: /\S/ }).waitFor()
      const was = await state.innerText()
      await page.evaluate(() => fetch('/api/linkedin', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ app: 'Google Chrome', folder: 'Profile 1' }) }))
      await page.reload()
      await state.filter({ hasText: 'Arbeit' }).waitFor()
      await menu(page, 'undo')
      await state.filter({ hasText: was }).waitFor()
      if ((await state.innerText()) !== was) return `after Undo it says ${await state.innerText()}, it said ${was}`
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
