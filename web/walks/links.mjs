// Walks adding people from their LinkedIn links on People, against the
// mock, in Chromium, the way walks/pick.mjs does. Tim agreed the way it
// works on 22 November 2026, issue #43, and on 23 November how a profile
// Exa lacks gets in, pasted from Chrome: links pasted into the search, or
// anywhere on People, turn Run into Add, Add 2 or Open, Enter does the
// same, links dropped on the window light all of it and are added too,
// and the people added show under New at once, each row filled while
// Exa reads its profile, with New counting how far the run is. Every
// person and link here is invented. `make walks` runs it.
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

const search = (page) => page.locator('main .search input')
const start = (page) => page.locator('main button.start')
const said = async (page) => (await start(page).textContent()).trim()
const pressed = (page) => page.locator('main .filters button[aria-pressed="true"]')

async function people(browser, width = 1280) {
  const page = await browser.newPage({ viewport: { width, height: 800 } })
  await page.goto(`${url}/app/people`)
  await page.locator('main .prow').first().waitFor()
  return page
}

// The run the walk started ends, so the next finds the mock at rest.
async function done(page) {
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Run', null, { timeout: 30000 })
}

await walk('Links in the search turn Run into Add 2, Open, or a greyed Add, and the list answers them', async (browser) => {
  const page = await people(browser)
  await search(page).fill('https://www.linkedin.com/in/greta-probe-71 www.linkedin.com/in/hugo-muster-72/')
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Add 2')
  const empty = await page.locator('main .rows-wrap').textContent()
  expect(`the list said "${empty}"`, empty.includes('Not in the app yet'))
  // One who is here already opens.
  await search(page).fill('https://www.linkedin.com/in/example-1000')
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Open')
  expect('the list does not show the one person the link leads to', (await page.locator('main .prow').count()) === 1)
  // A company page adds nobody.
  await search(page).fill('https://www.linkedin.com/company/probe-gmbh')
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Add')
  expect('Add is not greyed out for a company page', await start(page).isDisabled())
  const none = await page.locator('main .rows-wrap').textContent()
  expect(`the list said "${none}"`, none.includes("Not a person's profile"))
  // Words again, and it is Run.
  await search(page).fill('Probe')
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Run')
})

await walk('Enter adds the people, shows them under New on top, fills each row while its profile is read, and New counts how far', async (browser) => {
  const page = await people(browser)
  await search(page).fill('https://www.linkedin.com/in/greta-probe-71 https://www.linkedin.com/in/neu-probe-73')
  await search(page).press('Enter')
  await page.waitForFunction(() => document.querySelector('main .filters button[aria-pressed="true"]')?.textContent.startsWith('New'))
  expect('the search was not emptied', (await search(page).inputValue()) === '')
  // The two come first, newest on top, named by their handles.
  await page.locator('main .prow b', { hasText: 'greta-probe-71' }).waitFor()
  const first = await page.locator('main .prow b').evaluateAll((b) => b.slice(0, 2).map((x) => x.textContent))
  expect(`the list began with ${first}`, first.includes('greta-probe-71') && first.includes('neu-probe-73'))
  // Each row fills while its profile is read, several at once.
  await page.locator('main .prow.reading .fill i').first().waitFor()
  const reading = await page.locator('main .prow.reading').count()
  expect(`${reading} rows were filled`, reading === 2)
  const line = await page.locator('main .prow.reading .sub').first().textContent()
  expect(`a row being read said "${line}"`, /^Reading the profile from Exa · about \d+ s left$/.test(line))
  const count = await pressed(page).locator('.n').textContent()
  expect(`New counted "${count}"`, /^\d+ of 2$/.test(count))
  expect(`Run said "${await said(page)}" while the run went`, (await said(page)) === 'Stop')
  await done(page)
  // Read, the person has their name, and the one Exa has not indexed
  // says so in red.
  await page.locator('main .prow b', { hasText: 'Greta Probe' }).waitFor()
  const missing = page.locator('main .prow', { hasText: 'neu-probe-73' }).locator('.sub')
  expect(`the one not indexed said "${await missing.textContent()}"`, (await missing.textContent()) === 'Paste their LinkedIn page')
  expect('the one not indexed is not in red', await missing.evaluate((e) => e.classList.contains('bad')))
  const settled = await pressed(page).locator('.n').textContent()
  expect(`New counted "${settled}" once the run was done`, /^\d+$/.test(settled))
})

await walk('Enter on a greyed Add adds nobody and marks nobody connected', async (browser) => {
  const page = await people(browser)
  await search(page).fill('https://www.linkedin.com/in/example-1002 https://www.linkedin.com/in/example-1003')
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Add')
  expect('Add is not greyed out for people here already', await start(page).isDisabled())
  const posts = []
  page.on('request', (r) => r.method() === 'POST' && r.url().endsWith('/api/people') && posts.push(r.url()))
  await search(page).press('Enter')
  // Whatever Enter sent has gone by the time the search's next answer is in.
  await search(page).fill('Probe')
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Run')
  expect(`Enter sent ${posts.length} adds`, posts.length === 0)
  expect(`the filter moved to ${await pressed(page).textContent()}`, (await pressed(page).textContent()).startsWith('All'))
})

await walk('Undo takes whom adding brought in out again', async (browser) => {
  const page = await people(browser)
  await search(page).fill('https://www.linkedin.com/in/olaf-probe-78')
  await search(page).press('Enter')
  await page.locator('main .prow b', { hasText: 'olaf-probe-78' }).waitFor()
  await page.locator('.toast-region button', { hasText: 'Undo' }).click()
  await page.locator('main .prow b', { hasText: 'olaf-probe-78' }).waitFor({ state: 'detached' })
  await done(page)
})

await walk('Links pasted on People, not into a field, go into the search', async (browser) => {
  const page = await people(browser)
  await page.evaluate(() => {
    document.activeElement?.blur()
    const data = new DataTransfer()
    data.setData('text/plain', 'https://www.linkedin.com/in/ida-beispiel-74')
    document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true }))
  })
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Add')
  expect('the search does not hold the link', (await search(page).inputValue()) === 'https://www.linkedin.com/in/ida-beispiel-74')
})

await walk('A LinkedIn page pasted on someone Exa lacks becomes their profile, and Undo puts back their link name', async (browser) => {
  const page = await people(browser)
  await search(page).fill('https://www.linkedin.com/in/neu-probe-79')
  await search(page).press('Enter')
  await done(page)
  // The toast of the run's end comes first, so it does not take the place
  // of the paste's.
  await page.locator('.toast-region', { hasText: 'Run done' }).waitFor()
  const row = page.locator('main .prow', { hasText: 'neu-probe-79' })
  await row.locator('.sub', { hasText: 'Paste their LinkedIn page' }).waitFor()
  expect('the row does not say how to give the app the profile', (await row.locator('.sub').getAttribute('title')).includes('⌘V'))
  await row.click()
  await page.waitForURL(/\/app\/people\/\d+$/)
  // Open, they say how the app gets their profile, until it has it.
  const hint = page.locator('main .paste-hint')
  expect(`open, they said "${await hint.textContent()}"`, (await hint.textContent()).includes('⌘A and ⌘C'))
  await page.evaluate(() => {
    document.activeElement?.blur()
    const data = new DataTransfer()
    data.setData('text/plain', 'Startseite\nNeu Probe\nGründerin | Probe Keramik\nDortmund, Nordrhein-Westfalen, Deutschland\nInfo\nIch töpfere.')
    document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true }))
  })
  await page.locator('main .prow b', { hasText: 'Neu Probe' }).waitFor()
  await hint.waitFor({ state: 'detached' })
  const line = await page.locator('main .prow', { hasText: 'Neu Probe' }).locator('.sub').textContent()
  expect(`the row said "${line}"`, line.includes('Gründerin | Probe Keramik') && line.includes('Dortmund'))
  expect('the page went into the search', (await search(page).inputValue()) === '')
  await page.locator('.toast-region', { hasText: 'Kept the page' }).waitFor()
  const said = await page.locator('.toast-region').textContent()
  expect(`the toast said "${said}"`, said.includes('Kept the page as their profile') && !said.includes('Neu Probe'))
  await page.locator('.toast-region button', { hasText: 'Undo' }).click()
  await page.locator('main .prow b', { hasText: 'neu-probe-79' }).waitFor()
})

await walk('A LinkedIn page pasted on another page, or into the search, opens whose it is and drafts their first message', async (browser) => {
  const page = await people(browser)
  await search(page).fill('https://www.linkedin.com/in/eva-s-neu-80')
  await search(page).press('Enter')
  await done(page)
  await page.locator('main .prow', { hasText: 'eva-s-neu-80' }).locator('.sub', { hasText: 'Paste their LinkedIn page' }).waitFor()
  // Tim went to Settings, and pastes the page there, with the links
  // Chrome copies with it, his own among them.
  await page.goto(`${url}/app/settings`)
  await page.locator('main').waitFor()
  const text = 'Home\nMy Network\n\nEva Neu\n· 1st\n\nGründerin | Probe Keramik\nDortmund, Nordrhein-Westfalen, Deutschland\nAbout\nIch töpfere.'
  const html = '<a href="/in/tim-beobachter/">Me</a><p>Eva Neu</p><a href="https://www.linkedin.com/in/eva-s-neu-80/overlay/contact-info/">Contact info</a><a href="/in/eva-s-neu-80/recent-activity/all/">Activity</a>'
  await page.evaluate(([text, html]) => {
    document.activeElement?.blur()
    const data = new DataTransfer()
    data.setData('text/plain', text)
    data.setData('text/html', html)
    document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true }))
  }, [text, html])
  await page.waitForURL(/\/app\/people\/\d+$/)
  const opened = page.url()
  await page.locator('main article.person[aria-label="Eva Neu"]').waitFor()
  await page.locator('.toast-region', { hasText: 'Kept the page' }).waitFor()
  const toast = await page.locator('.toast-region').textContent()
  expect(`the toast said "${toast}"`, toast.includes('drafting the first message') && !toast.includes('not the one') && !toast.includes('Eva'))
  // The first message is being written, on the button that started it.
  await page.locator('main .first-message .draft-progress').waitFor()
  expect('the paste went into the search', (await search(page).inputValue()) === '')

  // Pasted again into the search field, by its text alone, with nobody
  // open, the page finds her by the name on it.
  await page.goto(`${url}/app/people`)
  await search(page).focus()
  await page.evaluate((text) => {
    const data = new DataTransfer()
    data.setData('text/plain', text)
    document.activeElement.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true }))
  }, text)
  await page.waitForURL(opened)
  expect('the page went into the search', (await search(page).inputValue()) === '')

  // A page of someone the app does not have says so, and opens nobody.
  await page.goto(`${url}/app/people`)
  await page.evaluate(() => {
    document.activeElement?.blur()
    const data = new DataTransfer()
    data.setData('text/plain', 'Ida Unbekannt\n· 2nd\nCoach\nAbout\nIch coache.')
    document.dispatchEvent(new ClipboardEvent('paste', { clipboardData: data, bubbles: true }))
  })
  await page.locator('.toast-region', { hasText: 'Nobody in the app has this profile' }).waitFor()
  expect('a person opened', !/\/app\/people\/\d+$/.test(page.url()))
})

await walk('Links dropped on the window light all of it and are added, from any page', async (browser) => {
  const page = await people(browser)
  await page.goto(`${url}/app/searches`)
  await page.locator('main').waitFor()
  await page.evaluate(() => {
    const data = new DataTransfer()
    data.setData('text/uri-list', 'https://www.linkedin.com/in/jule-probe-75')
    const at = document.querySelector('main')
    at.dispatchEvent(new DragEvent('dragenter', { dataTransfer: data, bubbles: true }))
    at.dispatchEvent(new DragEvent('dragover', { dataTransfer: data, bubbles: true, cancelable: true }))
  })
  // It fades in, so it is read once it has.
  await page.waitForFunction(() => getComputedStyle(document.querySelector('.landing')).opacity === '1', null, { timeout: 2000 })
  await page.evaluate(() => {
    const data = new DataTransfer()
    data.setData('text/uri-list', 'https://www.linkedin.com/in/jule-probe-75')
    document.querySelector('main').dispatchEvent(new DragEvent('drop', { dataTransfer: data, bubbles: true, cancelable: true }))
  })
  await page.waitForURL(/\/app\/people$/)
  await page.locator('main .prow b', { hasText: 'jule-probe-75' }).waitFor()
  expect('New is not chosen', (await pressed(page).textContent()).startsWith('New'))
  await page.waitForFunction(() => getComputedStyle(document.querySelector('.landing')).opacity === '0')
  await done(page)
})

await walk('At 390 px the search, Add 2 and the filters fit with no sideways scrolling', async (browser) => {
  const page = await people(browser, 390)
  await search(page).fill('https://www.linkedin.com/in/kai-probe-76 https://www.linkedin.com/in/lou-probe-77')
  await page.waitForFunction(() => document.querySelector('main button.start')?.textContent.trim() === 'Add 2')
  const over = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(`the page scrolls ${over} px sideways`, over <= 0)
})

await server.close()
process.exit(failed ? 1 : 0)
