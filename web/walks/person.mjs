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
    // Tim saw the list reshuffle as a person came in: rows went from one
    // line to two, the toolbar broke into lines and the list jumped.
    name: 'Opening a person narrows the list and moves nothing in it',
    async run(page) {
      const look = () => page.evaluate(() => {
        const rows = [...document.querySelectorAll('.prow')]
        return JSON.stringify({
          rows: rows.map((r) => Math.round(r.getBoundingClientRect().top)),
          controls: [...document.querySelectorAll('.controls > *, .controls .toolbar > *')].map((c) => Math.round(c.getBoundingClientRect().top)),
        })
      })
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      const closed = await look()
      await page.locator('.prow').nth(2).click()
      await page.locator('.detail-pane').waitFor()
      await page.waitForTimeout(0)
      await page.evaluate(() => Promise.all(document.getAnimations().map((a) => a.finished.catch(() => {}))))
      const open = await look()
      if (open !== closed) return `the list changed: closed ${closed}, open ${open}`
      return ''
    },
  },
  {
    // Tim saw the pane slide in while the list jumped to its new width at
    // once. An animation goes the full way, so the list narrows over the
    // frames the pane slides in on, and widens again as it leaves, with
    // every row as tall as before on every frame.
    name: 'The list narrows and widens with the person, frame by frame',
    async run(page) {
      // Reads the list's width and the rows' heights on every frame,
      // starting before the click, until the given time has passed.
      const frames = (ms) => page.evaluate((ms) => new Promise((done) => {
        const out = []
        const end = performance.now() + ms
        const read = () => {
          const list = document.querySelector('.list-pane')
          out.push({
            w: Math.round(list.getBoundingClientRect().width),
            h: [...document.querySelectorAll('.prow')].map((r) => Math.round(r.getBoundingClientRect().height)).join(),
          })
          if (performance.now() < end) requestAnimationFrame(read)
          else done(out)
        }
        requestAnimationFrame(read)
      }), ms)
      const judge = (what, fs, narrower) => {
        const first = fs[0].w
        const last = fs[fs.length - 1].w
        if (narrower ? last >= first : last <= first) return `the list did not ${narrower ? 'narrow' : 'widen'}: ${first} to ${last}`
        const between = new Set(fs.map((f) => f.w).filter((w) => w !== first && w !== last))
        if (between.size < 3) return `the list ${what} in ${between.size} steps between ${first} and ${last}, it jumps`
        for (let i = 1; i < fs.length; i++) {
          if (narrower ? fs[i].w > fs[i - 1].w : fs[i].w < fs[i - 1].w) return `the list ${what} and then went back, ${fs.map((f) => f.w).join(' ')}`
        }
        const heights = new Set(fs.map((f) => f.h))
        if (heights.size !== 1) return `rows changed height while the list ${what}: ${[...heights].join(' | ')}`
        return ''
      }
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      const opening = frames(600)
      await page.locator('.prow').nth(2).click()
      const opened = judge('narrowed', await opening, true)
      if (opened) return opened
      const closing = frames(600)
      await page.keyboard.press('Escape')
      return judge('widened', await closing, false)
    },
  },
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
      // Once the toast is gone, the line under the buttons undoes it.
      await page.getByRole('button', { name: 'Not this person' }).click()
      await linkedin.waitFor({ state: 'detached' })
      await page.locator('.set-aside').getByRole('button', { name: 'Undo' }).click()
      await linkedin.waitFor()
      if ((await review(page, 1001)) === 'rejected') return 'the line\'s Undo left the profile set aside'
      return ''
    },
  },
  {
    // Without a choice of where LinkedIn opens, the app says so with a
    // field, and the line leads to Settings. The button is itself again
    // as soon as the answer is in.
    name: 'LinkedIn with nothing chosen leads to Settings',
    async run(page) {
      await page.goto(`${url}/app/people/1002`)
      const linkedin = page.locator('.actions a.btn')
      await linkedin.click()
      const line = page.locator('.open-problem')
      await line.waitFor()
      if ((await line.getByRole('link', { name: 'Settings' }).count()) !== 1) return `the line says ${await line.innerText()}`
      const text = (await linkedin.innerText()).trim()
      if (text !== 'LinkedIn') return `the button says ${text} once the answer is in`
      return ''
    },
  },
  {
    // Someone connected is only under Connected, so a search for their
    // name under All finds nobody there. The empty list says where they
    // are and leads there with the search kept.
    name: 'Searching All for someone connected leads to them under Connected',
    async run(page) {
      await page.goto(`${url}/app/people`)
      await page.locator('main .filters button', { hasText: 'Connected' }).click()
      const name = await page.locator('.prow b').first().innerText()
      await page.locator('main .filters button', { hasText: 'All' }).click()
      await page.getByRole('searchbox', { name: 'Search people' }).or(page.getByLabel('Search people')).first().fill(name)
      const empty = page.locator('.rows-wrap', { hasText: 'Only under Connected' })
      await empty.waitFor()
      await empty.getByRole('button', { name: 'Show under Connected' }).click()
      await page.locator('.prow', { hasText: name }).waitFor()
      const pressed = await page.locator('main .filters button[aria-pressed="true"]').innerText()
      if (!pressed.startsWith('Connected')) return `the filter shown is ${pressed}`
      return ''
    },
  },
  {
    // Tim, 21 November 2026: people he connected with stayed in All and
    // Under 30, so he scrolled past them to find someone open. Connected,
    // a person leaves both for Connected, the counts follow, the person
    // stays open, and Down goes to whom took their place.
    name: 'Connecting a person moves them from All to Connected, and taking it back brings them back',
    async run(page) {
      const count = (label) => page.locator('main .filters button', { hasText: label }).locator('.n').innerText().then(Number)
      const rows = () => page.locator('.prow').evaluateAll((rs) => rs.map((r) => r.id))
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      await page.waitForLoadState('networkidle')
      const before = await rows()
      if (await page.locator('.prow', { hasText: 'Connected' }).count()) return 'someone connected is listed under All'
      const all = await count('All')
      const connected = await count('Connected')
      const row = page.locator('.prow').nth(1)
      const id = await row.getAttribute('id')
      await row.click()
      await page.locator('.detail-pane button.connect[aria-pressed="false"]').click()
      await page.locator('.detail-pane button.connect[aria-pressed="true"]').waitFor()
      await page.locator(`#${id}`).waitFor({ state: 'detached' })
      const wrong = []
      if ((await count('All')) !== all - 1) wrong.push(`All counts ${await count('All')}, want ${all - 1}`)
      if ((await count('Connected')) !== connected + 1) wrong.push(`Connected counts ${await count('Connected')}, want ${connected + 1}`)
      if (!(await page.locator('.detail-pane').isVisible())) wrong.push('the person closed')
      await page.keyboard.press('ArrowDown')
      await page.waitForURL(`**/app/people/${before[2].replace('person-row-', '')}`)
      await page.goBack()
      await page.locator('main .filters button', { hasText: 'Connected' }).click()
      await page.locator(`#${id}`).waitFor()
      await page.locator(`#${id}`).click()
      await page.locator('.detail-pane button.connect[aria-pressed="true"]').click()
      await page.locator(`#${id}`).waitFor({ state: 'detached' })
      await page.locator('main .filters button', { hasText: 'All' }).click()
      await page.locator(`#${id}`).waitFor()
      if (JSON.stringify(await rows()) !== JSON.stringify(before)) wrong.push(`taken back, the list is ${await rows()}, was ${before}`)
      if ((await count('All')) !== all) wrong.push(`taken back, All counts ${await count('All')}, want ${all}`)
      return wrong.join('\n\t')
    },
  },
  {
    // Tim, 21 November 2026, issue #66: people whose profile he rejected
    // stayed among those he still has to go through. Rejected, a person
    // leaves All for Rejected, the counts follow and the person stays
    // open. A search by name under All leads there, and Undo brings them
    // back where they stood.
    name: 'Not this person moves a person from All to Rejected, and Undo brings them back',
    async run(page) {
      const count = (label) => page.locator('main .filters button', { hasText: label }).locator('.n').innerText().then(Number)
      const rows = () => page.locator('.prow').evaluateAll((rs) => rs.map((r) => r.id))
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
      await page.waitForLoadState('networkidle')
      const before = await rows()
      const all = await count('All')
      const rejected = await count('Rejected')
      const row = page.locator('.prow').nth(1)
      const id = await row.getAttribute('id')
      const name = await row.locator('b').innerText()
      await row.click()
      await page.locator('.detail-pane').getByRole('button', { name: 'Not this person' }).click()
      await page.locator(`#${id}`).waitFor({ state: 'detached' })
      const wrong = []
      if ((await count('All')) !== all - 1) wrong.push(`All counts ${await count('All')}, want ${all - 1}`)
      if ((await count('Rejected')) !== rejected + 1) wrong.push(`Rejected counts ${await count('Rejected')}, want ${rejected + 1}`)
      if (!(await page.locator('.detail-pane').isVisible())) wrong.push('the person closed')
      await page.keyboard.press('Escape')
      await page.getByLabel('Search people').first().fill(name)
      const empty = page.locator('.rows-wrap', { hasText: 'Only under Rejected' })
      await empty.waitFor()
      await empty.getByRole('button', { name: 'Show under Rejected' }).click()
      await page.locator(`#${id}`).click()
      await page.locator('.set-aside').getByRole('button', { name: 'Undo' }).click()
      await page.locator(`#${id}`).waitFor({ state: 'detached' })
      await page.keyboard.press('Escape')
      await page.getByLabel('Search people').first().fill('')
      await page.locator('main .filters button', { hasText: 'All' }).click()
      await page.locator(`#${id}`).waitFor()
      await page.waitForLoadState('networkidle')
      if (JSON.stringify(await rows()) !== JSON.stringify(before)) wrong.push(`undone, the list is ${await rows()}, was ${before}`)
      if ((await count('Rejected')) !== rejected) wrong.push(`undone, Rejected counts ${await count('Rejected')}, want ${rejected}`)
      return wrong.join('\n\t')
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
