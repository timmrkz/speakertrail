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
  await act(page).waitFor()
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
const act = (page) => page.locator('.person button.act').first()

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
      // The box is there before anything was drafted, empty, so Tim can
      // write one himself (Tim, 22 November 2026).
      if ((await box(page).count()) !== 1 || (await box(page).inputValue()) !== '') return 'no empty box stands ready before anything was drafted'
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
    name: 'The message is Tim’s to change, a double click selects all of it, and the copy mark copies it',
    async run(page) {
      // The app's web view refuses the window's own clipboard, as Tim saw
      // testing #60 on 22 November 2026, so the walk refuses it too.
      await page.addInitScript(() => {
        navigator.clipboard.writeText = () => Promise.reject(new DOMException('The request is not allowed by the user agent or the platform in the current context, possibly because the user denied permission.', 'NotAllowedError'))
      })
      await open(page, 1004)
      await drafted(page, 1004, 1, first)
      await box(page).dblclick()
      const sel = await box(page).evaluate((t) => [t.selectionStart, t.selectionEnd, t.value.length])
      if (sel[0] !== 0 || sel[1] !== sel[2]) return `a double click selects ${sel[0]} to ${sel[1]} of ${sel[2]}`
      await box(page).fill(`${first} PS`)
      await box(page).blur()
      await page.locator('.first-message .state').filter({ hasText: 'Saved' }).waitFor()
      if ((await kept(page, 1004)) !== `${first} PS`) return 'the change was not saved'
      // The copy mark shows only while the pointer is on the box.
      const mark = page.locator('.first-message button.copy')
      await page.mouse.move(0, 0)
      if ((await mark.evaluate((b) => getComputedStyle(b).opacity)) !== '0') return 'the copy mark shows with the pointer away from the box'
      await box(page).hover()
      if ((await mark.evaluate((b) => getComputedStyle(b).opacity)) !== '1') return 'the copy mark does not show with the pointer on the box'
      await mark.click()
      await page.waitForFunction(() => document.querySelector('.first-message button.copy')?.getAttribute('aria-label') === 'Copied')
      const copied = await page.evaluate(() => fetch('/api/clipboard').then((r) => r.json()).then((c) => c.text))
      if (copied !== `${first} PS`) return `Copy copied ${JSON.stringify(copied)}`
      if (await page.locator('.first-message .refused, .toast').count()) return 'a copy that worked says something went wrong'
      // The tick stays while it shows, with the pointer gone.
      await page.mouse.move(0, 0)
      if ((await mark.evaluate((b) => getComputedStyle(b).opacity)) !== '1') return 'the tick hides with the pointer away'
      return ''
    },
  },
  {
    // Tim saw the box keep its first size while the model wrote and jump
    // once it was done, 22 November 2026. It grows with every piece.
    name: 'The box grows with the draft as the model writes it',
    async run(page) {
      await open(page, 1000)
      await act(page).click()
      await page.locator('.first-message .step').waitFor()
      const words = 'Hallo Mara, schön mit dir vernetzt zu sein. Du bist mir aufgefallen, weil du ein Keramikstudio neben dem Studium aufgebaut hast.'.repeat(5).split(' ')
      let text = ''
      let grew = 0
      let before = 0
      for (let i = 0; i < words.length; i += 5) {
        text += ` ${words.slice(i, i + 5).join(' ')}`
        await say(page, job(1000, 1, { state: 'writing', text, fraction: 0.5, seconds_left: 4 }))
        const m = await page.waitForFunction((want) => {
          const t = document.querySelector('.first-message textarea')
          return t && t.value === want && { over: t.scrollHeight - t.clientHeight, h: t.clientHeight }
        }, text.trimStart()).then((h) => h.jsonValue())
        if (m.over > 0) return `with ${text.length} letters written the text is ${m.over} px taller than its box`
        if (m.h > before) grew++
        before = m.h
      }
      if (grew < 4) return `the box grew only ${grew} times while the draft was written`
      return ''
    },
  },
  {
    // An error shows where it happened, over the box, never as a toast
    // over the window, and in words Tim can act on (22 November 2026).
    name: 'A copy the Mac refuses says what to do, over the message, with no toast',
    async run(page) {
      await page.route('**/api/clipboard', (route) => route.fulfill({ status: 500, json: { error: 'The Mac did not take it on the clipboard. Select the text and press Cmd+C.' } }))
      await open(page, 1001)
      await drafted(page, 1001, 1, first)
      await box(page).hover()
      await page.locator('.first-message button.copy').click()
      const line = page.locator('.first-message .box .refused')
      await line.waitFor()
      const said = (await line.innerText()).trim()
      if (said !== 'Could not copy the message. Select it and press Cmd+C.') return `the line says ${JSON.stringify(said)}`
      if (await page.locator('.toast').count()) return 'a toast shows too'
      const [b, l] = await Promise.all([box(page).boundingBox(), line.boundingBox()])
      if (l.y < b.y || l.y + l.height > b.y + b.height || l.x < b.x || l.x + l.width > b.x + b.width) return 'the line is not over the box'
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
      await act(page).waitFor()
      await act(page).click()
      await page.locator('.first-message .failed').waitFor()
      await page.locator('.person button.act', { hasText: /^Draft$/ }).waitFor()
      if (await page.locator('.first-message .step').count()) return 'the draft still waits after it failed'
      return ''
    },
  },
  {
    name: 'The drafts read as a person opens leave a newer end alone',
    async run(page) {
      // The window asks for the drafts as a person opens, and the app
      // tells that the draft is done before that answer arrives, an
      // answer that still says it writes. The newer end stays.
      await page.route(/\/api\/people\/\d+\/draft\/cancel$/, (route) => route.fulfill({ status: 409, json: { error: 'No draft is under way for them' } }))
      let release
      const held = new Promise((r) => (release = r))
      const writing = job(1007, 1, { state: 'writing', text: first.slice(0, 20) })
      await page.route('**/api/drafts', async (route) => {
        await held
        await route.fulfill({ json: [writing] })
      })
      await page.goto(`${url}/app/people/1007`)
      await act(page).waitFor()
      await say(page, job(1007, 1, { state: 'done', text: first }))
      release()
      await page.waitForResponse('**/api/drafts')
      await page.waitForTimeout(100)
      if (await page.locator('.first-message .step').count()) return 'the draft went back to writing after it was done'
      return ''
    },
  },
  {
    name: 'Settings in the sidebar wears no dot while nothing there stands in the way',
    async run(page) {
      // Both answers in, then the rail is read, rather than a wait on
      // the clock.
      const answered = Promise.all([page.waitForResponse('**/api/keys'), page.waitForResponse('**/api/language')])
      await page.goto(`${url}/app/people/1000`)
      await answered
      await act(page).waitFor()
      if (await page.locator('a.nav[href="/app/settings"] .dot').count()) return 'Settings wears a dot with the key saved and the model here'
      return ''
    },
  },
  {
    name: 'Settings in the sidebar wears a red dot without the Exa key',
    async run(page) {
      await page.route('**/api/keys', (route) => route.fulfill({ json: { keys: [{ provider: 'exa', used: 0, budget: 1000, set: false }] } }))
      await page.goto(`${url}/app/people/1000`)
      const settings = page.locator('a.nav[href="/app/settings"]')
      await settings.locator('.dot.err').waitFor()
      const title = await settings.getAttribute('title')
      if (title !== 'Settings. Runs need the Exa key') return `Settings in the sidebar says ${JSON.stringify(title)}`
      return ''
    },
  },
  {
    name: 'Draft without a language model goes to the model in Settings',
    async run(page) {
      await page.route('**/api/language', (route) => route.fulfill({ json: {
        models: [{ name: 'gemma-4-26b-a4b', title: 'Gemma 4 26B A4B', maker: 'Google', about: '', download: 15_500_000_000,
          recommended: true, installed: false, inUse: true, needs: 16_624_595_968, fit: 'fits' }],
        folder: '~/Library/Application Support/Speaker Trail/models', server: true,
        standing: { mark: 'warn', why: 'Drafts need Gemma 4 26B A4B. Download it in Settings' } } }))
      let asked = 0
      await page.route(/\/api\/people\/\d+\/draft$/, (route) => { asked++; return route.fulfill({ status: 409, json: { error: 'no' } }) })
      await page.goto(`${url}/app/people/1000`)
      await act(page).waitFor()
      await page.waitForFunction(() => /Download it in Settings/.test(document.querySelector('.person button.act')?.title ?? ''))
      await act(page).click()
      await page.locator('#language').waitFor()
      await page.waitForFunction(() => {
        const r = document.querySelector('#language').getBoundingClientRect()
        return r.top >= 0 && r.bottom <= innerHeight
      })
      if (asked) return 'Draft was asked for with no model'
      // And Settings in the sidebar says why it is the way, the way
      // Updates wears its own dot.
      const settings = page.locator('a.nav[href="/app/settings"]')
      if (!(await settings.locator('.dot.warn').count())) return 'Settings in the sidebar wears no dot without a model'
      const title = await settings.getAttribute('title')
      if (title !== 'Settings. Drafts need Gemma 4 26B A4B. Download it in Settings') return `Settings in the sidebar says ${JSON.stringify(title)}`
      return ''
    },
  },
  {
    // The key is at the top of Settings, not at the language model.
    name: 'A draft that failed for want of the Exa key leads to the key in Settings',
    async run(page) {
      await open(page, 1002)
      await act(page).click()
      await page.locator('.first-message .step').waitFor()
      await say(page, job(1002, 1, { state: 'failed', error: 'The app has no profile text for them yet, and reading it needs the Exa key. Add it in Settings.' }))
      const link = page.locator('.first-message .failed').getByRole('link', { name: 'Open Settings' })
      await link.click()
      await page.locator('#exa-key').waitFor()
      if (new URL(page.url()).hash) return `the link went to ${new URL(page.url()).hash}, not the top of Settings`
      const top = await page.locator('#exa-key').evaluate((e) => { const r = e.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight })
      if (!top) return 'the key is not in view'
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
      // Tim saw "Settings.Open Settings", the link stuck to the full stop.
      const said = (await alert.innerText()).trim()
      if (!said.endsWith('Mac. Open Settings')) return `the line reads ${JSON.stringify(said)}`
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
      // Loaded as the app starts and kept, Tim's choice of 21 November 2026.
      const said = await line.first().textContent()
      if (!/Loaded, about 16\.6 GB of memory while the app is open\./.test(said)) return `the line says ${JSON.stringify(said)}`
      // The model is named in the list to pick it from, see walks/pick.mjs.
      const title = (await page.locator('.language button.pick .said > span:not(.room)').innerText()).trim()
      if (title !== 'Gemma 4 26B A4B') return `the model reads ${JSON.stringify(title)}`
      // The language model comes before the speech model.
      const order = await page.evaluate(() => {
        const l = document.querySelector('#language'), s = document.querySelector('#speech')
        return l && s ? l.compareDocumentPosition(s) & Node.DOCUMENT_POSITION_FOLLOWING : 0
      })
      if (!order) return 'the speech model comes before the language model'
      const prompt = page.locator('#set-hook_prompt')
      if ((await prompt.evaluate((t) => t.tagName)) !== 'TEXTAREA') return 'the prompt is a field of one line'
      // The box stands on its own: nothing round it draws a line of its own.
      const framed = await prompt.evaluate((t) => {
        for (let e = t.parentElement; e && !e.classList.contains('group'); e = e.parentElement) {
          if (parseFloat(getComputedStyle(e).borderTopWidth) > 0) return e.className
        }
        return ''
      })
      if (framed) return `the prompt sits in a frame, ${framed}`
      if ((await prompt.getAttribute('aria-labelledby')) !== 'head-hook_prompt') return 'the prompt has no name'
      const text = await prompt.inputValue()
      if (!text.includes('{{profile}}')) return 'the prompt has no {{profile}}'
      await prompt.fill(text.replace('{{profile}}', ''))
      await prompt.blur()
      await page.locator('.field-error').filter({ hasText: '{{profile}}' }).waitFor()
      return ''
    },
  },
  {
    name: 'A draft whose hook did not fit has … in its place, and says why until the next',
    async run(page) {
      await open(page, 1004)
      const text = 'Hallo Mara, du bist mir aufgefallen, weil …'
      await act(page).click()
      await page.locator('.first-message .step').waitFor()
      // While the model writes, the message is there with … for its part.
      await say(page, job(1004, 1, { state: 'writing', text, fraction: 0.5, seconds_left: 2 }))
      await page.waitForFunction((text) => document.querySelector('.first-message textarea')?.value === text, text)
      await keep(page, 1004, text)
      const why = 'The language model\'s answer did not fit, so … marks what it should have written: it left out portrait.'
      await say(page, job(1004, 1, { state: 'done', text, fraction: 1, seconds_left: 0, problem: why }))
      const line = page.locator('.first-message .failed')
      await line.filter({ hasText: 'it left out portrait' }).waitFor()
      if ((await line.getAttribute('role')) !== 'alert') return 'the line is not read out'
      if (await box(page).evaluate((t) => t.readOnly)) return 'the message cannot be changed'
      // The next draft that fits says nothing of it.
      await drafted(page, 1004, 2, first)
      if (await line.count()) return `after the next draft it still says ${await line.innerText()}`
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
