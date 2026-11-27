// Walks the setting new_days against the mock, in Chromium: it sits under
// People in Settings, and New on People holds and says as many days as it
// gives. Tim asked for it on 27 November 2026. `make walks` runs it, and
// so does CI.
import { chromium } from 'playwright'
import { url, server } from './lib/mock.mjs'

const newButton = (page) => page.locator('main .filters button').filter({ hasText: /^New/ })

// Types a number of days into the setting and leaves it, which saves it.
async function setDays(page, days) {
  await page.goto(`${url}/app/settings`)
  const field = page.locator('#set-new_days')
  await field.fill(String(days))
  await field.blur()
  await page.locator('.setting').filter({ has: field }).locator('.foot', { hasText: 'Saved' }).waitFor()
}

// What New shows on People, and what the app says it holds.
async function shown(page) {
  const answer = page.waitForResponse((r) => r.url().includes('/api/people?'))
  await page.goto(`${url}/app/people`)
  const body = await (await answer).json()
  await page.locator('main .prow').first().waitFor()
  const button = newButton(page)
  await button.locator('.n').filter({ hasText: String(body.counts.new) }).waitFor()
  return { title: await button.getAttribute('title'), count: body.counts.new }
}

const walks0 = [
  {
    name: 'How long someone stays under New is a setting under People in Settings, a week until it is changed',
    async run(page) {
      await page.goto(`${url}/app/settings`)
      const group = page.locator('section.group').filter({ has: page.locator('h2', { hasText: /^People$/ }) })
      await group.locator('#set-new_days').waitFor()
      const value = await group.locator('#set-new_days').inputValue()
      if (value !== '7') return `the setting starts at ${value}, not 7`
      if (await page.locator('section.group').filter({ has: page.locator('h2', { hasText: /^Searches$/ }) }).locator('#set-new_days').count()) {
        return 'the setting is under Searches too'
      }
      return ''
    },
  },
  {
    name: 'New holds and says as many days as the setting gives',
    async run(page) {
      const week = await shown(page)
      if (week.title !== 'Found or added in the last 7 days, newest first') return `with a week New says "${week.title}"`
      try {
        await setDays(page, 1)
        const day = await shown(page)
        if (day.title !== 'Found or added in the last day, newest first') return `with one day New says "${day.title}"`
        if (day.count >= week.count) return `New holds ${day.count} with one day and ${week.count} with 7`
        await setDays(page, 60)
        const longer = await shown(page)
        if (longer.title !== 'Found or added in the last 60 days, newest first') return `with 60 days New says "${longer.title}"`
        if (longer.count < week.count) return `New holds ${longer.count} with 60 days and ${week.count} with 7`
      } finally {
        // Put back as it was, for the walk after.
        await setDays(page, 7)
      }
      return ''
    },
  },
  {
    name: 'A part of a day is refused, and the setting keeps what it had',
    async run(page) {
      await page.goto(`${url}/app/settings`)
      const field = page.locator('#set-new_days')
      await field.fill('2.5')
      await field.blur()
      await page.locator('.setting').filter({ has: field }).locator('.field-error', { hasText: 'whole number' }).waitFor()
      const value = await field.inputValue()
      if (value !== '7') return `after 2.5 the field says ${value}`
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
