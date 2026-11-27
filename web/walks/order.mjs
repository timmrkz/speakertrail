// Walks what Tim's report of 27 November 2026 found, against the mock,
// in Chromium: New follows the order picked, Best fit first like every
// list, rather than always the newest first, and a search a run made from
// another says so on Searches. `make walks` runs it.
import { chromium } from 'playwright'
import { url, server } from './lib/mock.mjs'

let failed = 0
async function walk(name, fn) {
  const browser = await chromium.launch()
  try {
    await fn(browser)
    console.log(`ok    ${name}`)
  } catch (err) {
    failed++
    console.log(`FAIL  ${name}\n      ${err.message.split('\n').join('\n      ')}`)
  } finally {
    await browser.close()
  }
}

function expect(what, ok) {
  if (!ok) throw new Error(what)
}

// The rows' fits and names, top to bottom, once the list shows what
// was asked for.
const fits = (page) => page.locator('main .prow .fit').evaluateAll((f) => f.map((x) => Number(x.textContent.replace(/\D/g, ''))))
const names = (page) => page.locator('main .prow b').evaluateAll((b) => b.map((x) => x.textContent.trim()))

await walk('New is the best fit first, and the order picked changes it', async (browser) => {
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
  await page.goto(`${url}/app/people`)
  await page.locator('main .prow').first().waitFor()
  await page.locator('main .filters button', { hasText: 'New' }).click()
  await page.waitForFunction(() => document.querySelector('main .filters button[aria-pressed="true"]')?.textContent.startsWith('New'))
  await page.waitForFunction(() => document.querySelectorAll('main .prow .fit').length > 2)
  const byFit = await fits(page)
  expect(`New under Best fit was ${byFit}`, byFit.every((f, i) => i === 0 || byFit[i - 1] >= f))
  await page.locator('main .sort button.pick').click()
  await page.locator('.pick-row', { hasText: 'Name' }).click()
  await page.waitForFunction(() => {
    const n = [...document.querySelectorAll('main .prow b')].map((b) => b.textContent.trim())
    return n.length > 2 && n.every((x, i) => i === 0 || n[i - 1].localeCompare(x, 'de') <= 0)
  })
  const byName = await names(page)
  expect(`New by name was ${byName}`, byName.length === byFit.length)
})

await walk('A search a run made says which search it was made from', async (browser) => {
  const page = await browser.newPage({ viewport: { width: 390, height: 800 } })
  await page.goto(`${url}/app/searches`)
  const row = page.locator('li.search').filter({ has: page.locator('b:text-is("Gründerin direkt nach dem Studium Leverkusen")') })
  await row.waitFor()
  const sub = await row.locator('.sub').textContent()
  expect(`its line said "${sub}"`, sub.includes('Made from Gründerin direkt nach dem Studium Düsseldorf'))
  // One Tim added says nothing of the kind.
  const other = await page.locator('li.search').filter({ has: page.locator('b:text-is("Personal Trainer Köln")') }).locator('.sub').textContent()
  expect(`a search Tim added said "${other}"`, !other.includes('Made from'))
  const sideways = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(`Searches scrolls sideways by ${sideways} px`, sideways <= 0)
})

await server.close()
if (failed) {
  console.log(`${failed} walk${failed === 1 ? '' : 's'} failed`)
  process.exit(1)
}
