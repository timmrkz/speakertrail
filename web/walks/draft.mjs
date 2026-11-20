// Walks a first message against the mock, in Chromium, the way
// walks/quit.mjs does. The app tells the window how a draft stands through
// the Wails event called draft, and the walk sends it the way the app does,
// through window._wails.dispatchWailsEvent, so nothing waits on the
// clock. What the app keeps of a draft the walk saves itself, as the app
// would have. `make walks` runs them, and so does CI.
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

const first = 'Hallo Mara, danke fürs Vernetzen! Dein Studio in Köln hat mich neugierig gemacht. Hättest du Lust auf ein Gespräch? Tim'
const second = 'Hallo Mara, schön, dass wir jetzt vernetzt sind. Magst du mein Gast sein? Tim'

// A person open, with the drafts in the walk's hands: the app has none
// under way, and Draft gives a queued draft at once, as the app does.
async function open(page, id) {
  let n = 0
  await page.route('**/api/drafts', (route) => route.fulfill({ json: [] }))
  await page.route(/\/api\/people\/\d+\/draft$/, (route) =>
    route.fulfill({ json: job(id, ++n, { state: 'queued' }) }))
  await page.route(/\/api\/people\/\d+\/draft\/cancel$/, (route) => route.fulfill({ status: 204 }))
  await page.goto(`${url}/app/people/${id}`)
  await page.locator('.first-message').waitFor()
}

// Each change numbered higher than the one before, as Seq in
// internal/draft is.
let seq = 0
function job(id, n, more) {
  return { id: `draft-${n}`, person_id: id, started_at: new Date(Date.now() - 4000).toISOString(), model: 'Gemma 4 26B A4B', fraction: -1, seconds_left: null, seq: ++seq, ...more }
}

// What the Go side says, the way it says it, see Job in internal/draft.
const say = (page, data) => page.evaluate((data) => window._wails.dispatchWailsEvent({ name: 'draft', data }), data)

// The message the app kept, saved the way the app saves it.
const keep = (page, id, draft) =>
  page.evaluate(([id, draft]) => fetch(`/api/people/${id}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ draft }) }), [id, draft])
const kept = (page, id) => page.evaluate((id) => fetch(`/api/people/${id}`).then((r) => r.json()).then((p) => p.draft), id)

const box = (page) => page.locator('.first-message textarea')
const act = (page) => page.locator('.first-message button.act').first()

// A draft written from start to end: Draft, the text as it grows, then
// the whole message, Tim's to change.
async function drafted(page, id, n, text) {
  await act(page).click()
  await page.locator('.first-message .step').waitFor()
  await say(page, job(id, n, { state: 'writing', text: ' Hallo', fraction: 0.6, seconds_left: 4 }))
  await keep(page, id, text)
  await say(page, job(id, n, { state: 'done', text, fraction: 1, seconds_left: 0 }))
  await page.waitForFunction((text) => {
    const t = document.querySelector('.first-message textarea')
    return t && !t.readOnly && t.value === text
  }, text)
}

const walks = [
  {
    name: 'Draft writes the message where it can be read as it grows',
    async run(page) {
      await open(page, 1003)
      const button = act(page)
      if ((await button.innerText()).trim() !== 'Draft') return `the button reads ${JSON.stringify(await button.innerText())}`
      if (await box(page).count()) return 'an empty box shows before anything was drafted'
      await button.click()
      await page.locator('.first-message .step').filter({ hasText: 'Waiting for the draft before it' }).waitFor()
      if ((await act(page).innerText()).trim() !== 'Cancel') return `while it runs the button reads ${await act(page).innerText()}`
      await say(page, job(1003, 1, { state: 'loading', fraction: 0.2, seconds_left: 12 }))
      await page.locator('.first-message .step').filter({ hasText: 'Loading Gemma 4 26B A4B' }).waitFor()
      await say(page, job(1003, 1, { state: 'writing', text: ' Hallo Mara,', fraction: 0.6, seconds_left: 4 }))
      await page.locator('.first-message .step').filter({ hasText: 'Writing with Gemma 4 26B A4B' }).waitFor()
      const value = await box(page).inputValue()
      if (value !== 'Hallo Mara,') return `the box shows ${JSON.stringify(value)} while it is written`
      if (!(await box(page).evaluate((t) => t.readOnly))) return 'the box can be typed in while the model writes'
      const meta = (await page.locator('.first-message .meta').innerText()).trim()
      if (!/^\d+ s so far, about 5 s left$/.test(meta)) return `the time reads ${JSON.stringify(meta)}`
      // One progress, the fill in Cancel, and no second bar beside it.
      if (await page.locator('.first-message .progress').count()) return 'a bar of its own shows beside Cancel'
      const fill = await act(page).locator('.fill i').evaluate((i) => i.style.transform)
      if (fill !== 'translateX(-40%)') return `Cancel is filled ${JSON.stringify(fill)}, want 60 %`
      return ''
    },
  },
  {
    name: 'The message is Tim’s to change, a double click selects all of it, and Copy copies it',
    async run(page) {
      await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
      await open(page, 1004)
      await drafted(page, 1004, 1, first)
      await box(page).dblclick()
      const sel = await box(page).evaluate((t) => [t.selectionStart, t.selectionEnd, t.value.length])
      if (sel[0] !== 0 || sel[1] !== sel[2]) return `a double click selects ${sel[0]} to ${sel[1]} of ${sel[2]}`
      await box(page).fill(`${first} PS`)
      await box(page).blur()
      await page.locator('.first-message .state').filter({ hasText: 'Saved' }).waitFor()
      if ((await kept(page, 1004)) !== `${first} PS`) return 'the change was not saved'
      await page.locator('.first-message button', { hasText: 'Copy' }).click()
      const copied = await page.evaluate(() => navigator.clipboard.readText())
      if (copied !== `${first} PS`) return `Copy copied ${JSON.stringify(copied)}`
      return ''
    },
  },
  {
    name: 'Draft again writes a new message, and Undo brings the one before back',
    async run(page) {
      await open(page, 1005)
      await drafted(page, 1005, 1, first)
      if ((await act(page).innerText()).trim() !== 'Draft again') return `after a draft the button reads ${await act(page).innerText()}`
      await drafted(page, 1005, 2, second)
      await page.getByRole('button', { name: 'Undo' }).click()
      await page.waitForFunction((text) => document.querySelector('.first-message textarea')?.value === text, first)
      if ((await kept(page, 1005)) !== first) return 'Undo did not save the message before'
      return ''
    },
  },
  {
    name: 'Draft’s answer arriving after the event that the draft failed leaves it failed',
    async run(page) {
      // The app tells that it failed before its answer to Draft reaches
      // the window, as it did on Tim’s Mac on 20 November 2026.
      await page.route('**/api/drafts', (route) => route.fulfill({ json: [] }))
      await page.route(/\/api\/people\/\d+\/draft$/, async (route) => {
        const queued = job(1008, 1, { state: 'queued' })
        await say(page, job(1008, 1, { state: 'failed', error: 'The language model stopped answering.' }))
        await route.fulfill({ json: queued })
      })
      await page.goto(`${url}/app/people/1008`)
      await page.locator('.first-message').waitFor()
      await act(page).click()
      await page.locator('.first-message .failed').waitFor()
      await page.locator('.first-message button.act', { hasText: /^Draft$/ }).waitFor()
      if (await page.locator('.first-message .step').count()) return 'the draft still waits after it failed'
      return ''
    },
  },
  {
    name: 'Draft without a language model goes to the model in Settings',
    async run(page) {
      await page.route('**/api/language', (route) => route.fulfill({ json: {
        models: [{ name: 'gemma-4-26b-a4b', title: 'Gemma 4 26B A4B', maker: 'Google', about: '', download: 15_500_000_000,
          recommended: true, installed: false, inUse: true, needs: 16_624_595_968, fit: 'comfortable' }],
        folder: '~/Library/Application Support/Speaker Trail/models', server: true } }))
      let asked = 0
      await page.route(/\/api\/people\/\d+\/draft$/, (route) => { asked++; return route.fulfill({ status: 409, json: { error: 'no' } }) })
      await page.goto(`${url}/app/people/1000`)
      await page.locator('.first-message').waitFor()
      await page.waitForFunction(() => /Download a language model/.test(document.querySelector('.first-message button.act')?.title ?? ''))
      await act(page).click()
      await page.locator('#language').waitFor()
      await page.waitForFunction(() => {
        const r = document.querySelector('#language').getBoundingClientRect()
        return r.top >= 0 && r.bottom <= innerHeight
      })
      if (asked) return 'Draft was asked for with no model'
      return ''
    },
  },
  {
    name: 'A draft that failed for want of something in Settings leads there',
    async run(page) {
      await open(page, 1006)
      await act(page).click()
      await page.locator('.first-message .step').waitFor()
      await say(page, job(1006, 1, { state: 'failed', error: 'Download a language model in Settings first. It writes the message on this Mac.' }))
      const alert = page.locator('.first-message .failed')
      await alert.waitFor()
      if ((await act(page).innerText()).trim() !== 'Draft') return `after it failed the button reads ${await act(page).innerText()}`
      await alert.getByRole('link', { name: 'Open Settings' }).click()
      await page.locator('#language').waitFor()
      await page.waitForFunction(() => {
        const r = document.querySelector('#language').getBoundingClientRect()
        return r.top >= 0 && r.bottom <= innerHeight
      })
      return ''
    },
  },
  {
    name: 'Settings has the language model and the prompt, and keeps {{profile}} in it',
    async run(page) {
      await page.goto(`${url}/app/settings`)
      const line = page.locator('.language .line')
      await line.filter({ hasText: /^By Google\./ }).waitFor()
      const title = (await page.locator('.language .desc').first().innerText()).trim()
      if (title !== 'Gemma 4 26B A4B') return `the model reads ${JSON.stringify(title)}`
      const prompt = page.locator('#set-first_message_prompt')
      if ((await prompt.evaluate((t) => t.tagName)) !== 'TEXTAREA') return 'the prompt is a field of one line'
      const text = await prompt.inputValue()
      if (!text.includes('{{profile}}')) return 'the prompt has no {{profile}}'
      await prompt.fill(text.replace('{{profile}}', ''))
      await prompt.blur()
      await page.locator('.field-error').filter({ hasText: '{{profile}}' }).waitFor()
      return ''
    },
  },
  {
    name: 'At 390 px the message fits with nothing to scroll sideways',
    viewport: { width: 390, height: 844 },
    async run(page) {
      await open(page, 1007)
      await drafted(page, 1007, 1, first)
      const wide = await page.evaluate(() => [document.scrollingElement.scrollWidth, innerWidth])
      if (wide[0] > wide[1]) return `the page is ${wide[0]} px wide in a window of ${wide[1]}`
      const r = await box(page).boundingBox()
      if (r.x < 0 || r.x + r.width > 390) return `the box runs from ${r.x} to ${r.x + r.width}`
      return ''
    },
  },
]

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
