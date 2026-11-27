// Walks through the app's log against the mock, in Chromium, the way
// walks/person.mjs does: what the window tells the log, and Settings,
// Logs. `make walks` runs them, and so does CI.
import { chromium } from 'playwright'
import { url, server } from './lib/mock.mjs'

// What the mock heard from the window for the app's log.
const heard = (page) => page.evaluate(() => fetch('/api/log/heard').then((r) => r.json()))
// Waits until the log heard what is asked, since the window sends its
// lines within a quarter of a second. The wait for the window's next
// line starts before the mock is asked, never after: a line sent while
// the answer was on its way would otherwise be missed, and the wait
// would last until the walk timed out, as it did in CI on #98 with the
// walks run side by side.
async function until(page, ok) {
  for (;;) {
    const next = page.waitForResponse((r) => r.url().includes('/api/log/') && !r.url().endsWith('/api/log/heard'))
    // A wait no longer needed ends with the page, and says nothing.
    next.catch(() => {})
    const h = await heard(page)
    if (ok(h)) return h
    await next
  }
}

const walks = [
  {
    // Every click is an act, named by the control it was on. A row of
    // People, the name in the person's view and the sheet's label would
    // name a person, so the log names them by their kind alone.
    name: 'Acts name the control and never a person',
    async run(page) {
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      const names = await page.evaluate(() => fetch('/api/people').then((r) => r.json()).then((d) => d.people.flatMap((p) => [p.name, p.headline ?? ''].filter(Boolean))))
      await page.locator('.prow').nth(1).click()
      await page.locator('.detail-pane .person').waitFor()
      await page.locator('.detail-pane .person-name').click()
      await page.locator('.connect').click()
      const h = await until(page, (h) => h.acts.length >= 3)
      const all = JSON.stringify(h)
      const named = names.find((n) => all.includes(n))
      if (named) return `the log names a person: ${named}`
      const whats = h.acts.map((a) => a.what)
      if (!whats.some((w) => /^pressed a\.prow$/.test(w))) return `no act for the row, as "pressed a.prow": ${whats.join(' | ')}`
      if (!whats.some((w) => /^pressed button\.btn "(Connect|Connected)"$/.test(w))) return `no act for Connect by its words: ${whats.join(' | ')}`
      if (new Set(h.acts.map((a) => a.id)).size !== h.acts.length) return `two acts share an id: ${h.acts.map((a) => a.id).join(', ')}`
      if (!h.said.some((l) => l.text.startsWith('window: started'))) return 'the window did not say it started'
      return ''
    },
  },
  {
    // #104: Stop said under the pointer whose profile the run read, and
    // the log named the button by that, so pressing it wrote the person's
    // name into the log. Stop is named by its own words now.
    name: 'Pressing Stop while a profile is read names no one in the log',
    async run(page) {
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      // A link added is read in a run straight away, named by its handle
      // until its profile is read.
      await page.locator('main .search input').fill('https://www.linkedin.com/in/rike-probe-91')
      await page.locator('main .search input').press('Enter')
      const reading = page.locator('button.start[title^="Reading the profile of"]')
      await reading.waitFor()
      const title = await reading.getAttribute('title')
      if (!title.startsWith('Reading the profile of rike-probe-91')) return `Stop said "${title}", want the person read`
      await reading.click()
      const h = await until(page, (h) => h.acts.some((a) => /Stop/.test(a.what)))
      const stop = h.acts.find((a) => /Stop/.test(a.what)).what
      if (stop !== 'pressed button.btn "Stop"') return `pressing Stop was logged as ${stop}`
      if (JSON.stringify(h).includes('rike-probe-91')) return 'the log names the person read'
      return ''
    },
  },
  {
    // In a narrow window the person opens in a sheet, labelled with their
    // name. A click and a key there are acts, and neither names them.
    name: 'Acts in the narrow window\'s person sheet never name a person',
    async run(page) {
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      const names = await page.evaluate(() => fetch('/api/people').then((r) => r.json()).then((d) => d.people.flatMap((p) => [p.name, p.headline ?? ''].filter(Boolean))))
      await page.locator('.prow').nth(1).click()
      const sheet = page.locator('.sheet[role=dialog]')
      await sheet.waitFor()
      await sheet.locator('h1, h2').first().click()
      await page.keyboard.press('ArrowDown')
      const h = await until(page, (h) => h.acts.some((a) => a.what.startsWith('ArrowDown')))
      const all = JSON.stringify(h)
      const named = names.find((n) => all.includes(n))
      if (named) return `the log names a person: ${named}`
      const key = h.acts.find((a) => a.what.startsWith('ArrowDown')).what
      if (!/^ArrowDown on \S+$/.test(key)) return `the key in the sheet is "${key}", want it named by its kind`
      return ''
    },
  },
  {
    // An error the window meets goes into the log at error, with the act
    // the person was at.
    name: 'An error in the window reaches the log',
    async run(page) {
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      await page.locator('.prow').first().click()
      await page.evaluate(() => setTimeout(() => { throw new Error('a walk threw this') }))
      const h = await until(page, (h) => h.said.some((l) => l.text.includes('a walk threw this')))
      const line = h.said.find((l) => l.text.includes('a walk threw this'))
      if (line.level !== 'error' || !line.text.startsWith('uncaught:') || !line.act) return `the line is ${JSON.stringify(line)}`
      return ''
    },
  },
  {
    // Settings, Logs, at the narrowest window: where the log is, its
    // files, size and lines, and the toast that ran out, with nothing
    // wider than the window. Removing asks first, Cancel where the
    // keyboard starts, and Remove them leaves the new log of one line.
    name: 'Settings shows the log and removes it after asking',
    async run(page) {
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`${url}/app/settings`)
      const head = page.locator('section.group:has(h2:text-is("Logs")) .desc')
      await head.filter({ hasText: 'lines' }).waitFor()
      const text = (await head.innerText()).trim()
      if (text !== '2 files, 2 MB, 9,812 lines') return `the log says ${text}`
      if ((await page.locator('.logged .text').innerText()).trim() !== 'Could not reach the app. Try again.') return 'the toast that ran out is not listed'
      const wide = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)
      if (wide > 0) return `the page is ${wide} px wider than the window`
      const show = page.waitForRequest((r) => r.url().endsWith('/api/log/show') && r.method() === 'POST')
      await page.getByRole('button', { name: "Show the app's log in folder" }).click()
      await show
      await page.getByRole('button', { name: "Remove the app's log" }).click()
      const box = page.locator('dialog[open]')
      await box.waitFor()
      if ((await page.evaluate(() => document.activeElement?.textContent?.trim())) !== 'Cancel') return 'the keyboard does not start on Cancel'
      await box.getByRole('button', { name: 'Remove them' }).click()
      await page.locator('dialog[open]').waitFor({ state: 'detached' })
      await head.filter({ hasText: '1 line' }).waitFor()
      const after = (await head.innerText()).trim()
      if (after !== '1 file, 1 KB, 1 line') return `after removing, the log says ${after}`
      if ((await page.locator('.logged').count()) !== 0) return 'the toasts are still listed'
      return ''
    },
  },
]

const browser = await chromium.launch()
let failed = 0
for (const w of walks) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 860 } })
  const errors = []
  page.on('pageerror', (e) => { if (!e.message.includes('a walk threw this')) errors.push(e.message) })
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
