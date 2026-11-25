// Walks the Updates page, against the mock, in Chromium, the way
// walks/pick.mjs does. The page is FrameFairy's, ported as it is, so what
// Tim found different is walked here: a channel named by what it is,
// never by its pull request's title, a list only as wide as that name,
// and a row that still fits a window 390 px wide. `make walks` runs it.
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


const trigger = (page) => page.locator('#channel')

async function updates(browser, width, failed = false) {
  const page = await browser.newPage({ viewport: { width, height: 800 } })
  if (failed)
    await page.route('**/api/updates', async (route) => {
      const st = await (await route.fetch()).json()
      await route.fulfill({
        json: { ...st, phase: 'failed', problem: 'GitHub did not answer.', detail: 'Get "https://github.com/example/releases/channels.json": dial tcp: lookup github.com: no such host' },
      })
    })
  await page.goto(`${url}/app/updates`)
  // Until the app has said which channel it follows, the list names none.
  await page.waitForFunction(() => /PR #9/.test(document.querySelector('#channel')?.textContent ?? ''))
  return page
}

await walk('The list names each channel by what it is, never by the title of its pull request', async (browser) => {
  const page = await updates(browser, 1280)
  await trigger(page).click()
  await page.locator('.pick-list').waitFor()
  const rows = await page.locator('.pick-row').evaluateAll((rows) => rows.map((r) => r.textContent.trim()))
  expect(`the rows say ${JSON.stringify(rows)}`, JSON.stringify(rows) === JSON.stringify(['Branch main', 'PR #9', 'PR #12, conflicts']))
})

await walk('The list is as wide as the channel it names, with How updates work beside it', async (browser) => {
  const page = await updates(browser, 1280)
  const list = await trigger(page).boundingBox()
  expect(`the list is ${list.width} px wide for PR #9`, list.width < 140)
  const mark = await page.locator('.build .info').boundingBox()
  expect(`the info mark ${JSON.stringify(mark)} is not just left of the list ${JSON.stringify(list)}`, mark.x + mark.width <= list.x && list.x - (mark.x + mark.width) <= 16)
})

await walk('At 390 px the build fits on one line, and the list stays in its card', async (browser) => {
  const page = await updates(browser, 390)
  const sideways = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(`the page scrolls sideways by ${sideways} px`, sideways <= 0)
  const version = await page.locator('.version').evaluate((v) => v.getClientRects().length === 1 && v.getBoundingClientRect().height < 30)
  expect('the version breaks over lines', version)
  const card = await page.locator('.list').boundingBox()
  const list = await trigger(page).boundingBox()
  expect(`the list ${JSON.stringify(list)} runs out of its card ${JSON.stringify(card)}`, list.x >= card.x && list.x + list.width <= card.x + card.width)
})

await walk('What the system said is a field under the words, with Try again beside it', async (browser) => {
  for (const width of [1280, 390]) {
    const page = await updates(browser, width, true)
    await page.locator('.detail code').waitFor()
    const words = await page.locator('.status .words').boundingBox()
    const field = await page.locator('.detail').boundingBox()
    const again = await page.getByRole('button', { name: 'Try again' }).boundingBox()
    expect(`at ${width} px the field ${JSON.stringify(field)} is not under the words ${JSON.stringify(words)}`, field.y >= words.y + words.height && Math.abs(field.x - words.x) < 1)
    expect(`at ${width} px Try again ${JSON.stringify(again)} is not beside the field ${JSON.stringify(field)}`, Math.abs(again.y + again.height / 2 - (field.y + field.height / 2)) < 1 && again.x > field.x + field.width)
    const sideways = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    expect(`at ${width} px the page scrolls sideways by ${sideways} px`, sideways <= 0)
    await page.close()
  }
})

// Tim followed PR #46 after it was merged, and the list and the line under
// the build called it closed. A merged pull request is said to be merged,
// in the colour of a warning, and one closed without a merge closed, in
// the colour of an error, on the list's edge and the line's mark alike.
await walk('A pull request followed that was merged says merged in amber, one closed says closed in red', async (browser) => {
  for (const [ended, word, colour] of [['merged', 'merged', '--warn'], ['closed', 'closed', '--bad']]) {
    const page = await browser.newPage({ viewport: { width: 390, height: 800 } })
    await page.route('**/api/updates', async (route) => {
      const st = await (await route.fetch()).json()
      await route.fulfill({ json: { ...st, picked: 'pr-46', follows: '', gone: 'pr-46', ended, phase: 'gone' } })
    })
    await page.goto(`${url}/app/updates`)
    await page.waitForFunction(() => /PR #46/.test(document.querySelector('#channel')?.textContent ?? ''))
    const label = (await trigger(page).textContent()).trim()
    expect(`the list says "${label}", want "PR #46, ${word}"`, label === `PR #46, ${word}`)
    const head = (await page.locator('.status .words').textContent()).replace(/\s+/g, ' ')
    expect(`the line says "${head}", want PR #46 is ${word}`, head.includes(`PR #46 is ${word}`))
    const seen = await page.evaluate((name) => {
      const probe = document.createElement('span')
      probe.style.color = `var(${name})`
      document.body.append(probe)
      const want = getComputedStyle(probe).color
      probe.remove()
      return { want, edge: getComputedStyle(document.querySelector('#channel')).borderTopColor, mark: getComputedStyle(document.querySelector('.status .mark')).color }
    }, colour)
    expect(`${ended}: the list's edge is ${seen.edge}, want ${colour}, ${seen.want}`, seen.edge === seen.want)
    expect(`${ended}: the line's mark is ${seen.mark}, want ${colour}, ${seen.want}`, seen.mark === seen.want)
    await page.close()
  }
})

await server.close()
if (failed) {
  console.log(`${failed} walk${failed === 1 ? '' : 's'} failed`)
  process.exit(1)
}
