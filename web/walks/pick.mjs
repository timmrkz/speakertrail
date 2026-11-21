// Walks the list the language model is picked from in Settings, against
// the mock, in Chromium, the way walks/draft.mjs does. The list is the
// app's own, Pick.svelte, ported from FrameFairy's, so what it promises is
// walked here: the models largest first with whether each is downloaded,
// a pick that names the model at once, here or not, a trigger that keeps
// its width, a list that lets go of the keyboard as it closes, and a row
// that still fits a window 390 px wide. `make walks` runs it.
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

const trigger = (page) => page.locator('#language button.pick')
const shown = (page) => page.locator('#language button.pick .said > span:not(.room)').textContent()

async function settings(browser, width) {
  const page = await browser.newPage({ viewport: { width, height: 800 } })
  await page.goto(`${url}/app/settings#language`)
  // Until the app has said how the models stand, the list has nothing in it.
  await page.waitForFunction(() => !!document.querySelector('#language button.pick .said > span:not(.room)')?.textContent)
  return page
}

await walk('The list names every model, largest first, with whether it is downloaded', async (browser) => {
  const page = await settings(browser, 900)
  await trigger(page).click()
  await page.locator('.pick-list').waitFor()
  const rows = await page.locator('.pick-row').evaluateAll((rows) => rows.map((r) => ({
    name: r.querySelector('.what').textContent,
    size: r.querySelector('.detail').textContent,
    mark: r.querySelector('.tick').getAttribute('title'),
    chosen: r.hasAttribute('data-chosen'),
  })))
  expect(`the rows were ${JSON.stringify(rows)}`, JSON.stringify(rows.map((r) => r.name)) === JSON.stringify(['Gemma 4 26B A4B', 'Qwen3 14B', 'Gemma 4 12B', 'Ministral 3 8B']))
  expect(`the sizes were ${rows.map((r) => r.size)}`, rows[0].size === '14.4 GB' && rows[3].size === '5.2 GB')
  expect(`the marks were ${rows.map((r) => r.mark)}`, rows[0].mark === 'Downloaded' && rows.slice(1).every((r) => r.mark === 'Not downloaded yet'))
  expect('the one that drafts is not the one chosen', rows[0].chosen && rows.slice(1).every((r) => !r.chosen))
})

await walk('A model picked is named at once, and Download fetches it', async (browser) => {
  const page = await settings(browser, 900)
  const before = (await trigger(page).boundingBox()).width
  await trigger(page).click()
  await page.locator('.pick-row', { hasText: 'Qwen3 14B' }).click()
  await page.locator('.pick-list').waitFor({ state: 'detached' })
  await page.waitForFunction(() => document.querySelector('#language button.pick .said > span:not(.room)')?.textContent === 'Qwen3 14B', null, { timeout: 2000 })
  const line = await page.locator('#language .line').first().textContent()
  expect(`the line said "${line}"`, line === 'Not downloaded yet. 9.0 GB.')
  expect('Download is not there to fetch it', await page.locator('#language button.act', { hasText: 'Download' }).isVisible())
  expect('the list does not warn that it cannot draft yet', await trigger(page).evaluate((b) => b.classList.contains('warn')))
  const after = (await trigger(page).boundingBox()).width
  expect(`the list changed width from ${before} to ${after}`, Math.abs(before - after) < 0.5)
  // Picked again, it is the one Settings shows after a reload, because the
  // app keeps it.
  await page.reload()
  await page.waitForFunction(() => document.querySelector('#language button.pick .said > span:not(.room)')?.textContent === 'Qwen3 14B', null, { timeout: 2000 })
  // Back to the one downloaded, for the walks after this one.
  await trigger(page).click()
  await page.locator('.pick-row', { hasText: 'Gemma 4 26B A4B' }).click()
  await page.waitForFunction(() => document.querySelector('#language button.pick .said > span:not(.room)')?.textContent === 'Gemma 4 26B A4B', null, { timeout: 2000 })
  expect('Download stayed for a model that is here', !(await page.locator('#language button.act', { hasText: 'Download' }).count()))
})

await walk('The list lets go of the keyboard as Escape closes it', async (browser) => {
  const page = await settings(browser, 900)
  await trigger(page).focus()
  await page.keyboard.press('Enter')
  await page.locator('.pick-list').waitFor()
  await page.keyboard.press('Escape')
  await page.locator('.pick-list').waitFor({ state: 'detached' })
  await page.waitForTimeout(50)
  const focused = await page.evaluate(() => document.activeElement?.className ?? '')
  expect(`the keyboard stayed on "${focused}"`, !String(focused).includes('pick'))
  expect('Escape picked something', (await shown(page)) === 'Gemma 4 26B A4B')
})

await walk('Checking FrameFairy’s copy says how far it is, on Cancel', async (browser) => {
  const page = await browser.newPage({ viewport: { width: 900, height: 800 } })
  await page.route('**/api/language', async (route) => {
    const st = await (await route.fetch()).json()
    st.models = st.models.map((m) => ({ ...m, installed: false }))
    st.job = { id: 'language-9', kind: 'model', title: 'Gemma 4 26B A4B', state: 'running', progress: { text: 'copying', fraction: 0.4, remaining: 4 } }
    await route.fulfill({ json: st })
  })
  await page.goto(`${url}/app/settings#language`)
  await page.waitForFunction(() => /^Checking FrameFairy/.test(document.querySelector('#language .line')?.textContent ?? ''))
  const line = await page.locator('#language .line').first().textContent()
  expect(`the line said "${line}"`, /^Checking FrameFairy’s copy, 5\.8 GB of 14\.4 GB, .+/.test(line))
  const fill = await page.locator('#language button.act .fill i').evaluate((i) => getComputedStyle(i).transform)
  expect(`Cancel is not filled to 40 %: ${fill}`, fill !== 'none')
})

await walk('At 390 px the row fits, with the list under the words', async (browser) => {
  const page = await settings(browser, 390)
  const sideways = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(`the page scrolls sideways by ${sideways} px`, sideways <= 0)
  const words = await page.locator('#language .info').first().boundingBox()
  const list = await trigger(page).boundingBox()
  expect(`the list ${JSON.stringify(list)} lies over the words ${JSON.stringify(words)}`, list.y >= words.y + words.height)
  await trigger(page).click()
  const open = await page.locator('.pick-list').boundingBox()
  expect(`the open list runs off the window: ${JSON.stringify(open)}`, open.x >= 0 && open.x + open.width <= 390)
})

await server.close()
if (failed) {
  console.log(`${failed} walk${failed === 1 ? '' : 's'} failed`)
  process.exit(1)
}
