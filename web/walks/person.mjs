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

const person = (page, id) => page.evaluate((id) => fetch(`/api/people/${id}`).then((r) => r.json()), id)
const keepDraft = (page, id, draft) =>
  page.evaluate(([id, draft]) => fetch(`/api/people/${id}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ draft }) }), [id, draft])
// A first message as long as the ones the app drafts, invented.
const message = 'Hallo Mara,\n\nschön mit dir vernetzt zu sein. Du bist mir aufgefallen, weil du ein Keramikstudio neben dem Studium aufgebaut hast.\n\nDaher würde ich dich gern in meinen Podcast einladen, um deine Geschichte als Gründerin zu erzählen. Die erste Frage ist immer: was ist deine erste Erinnerung?\n\nWenn du Lust hast, lass uns die Tage einen kurzen Video Call machen.\n\nLiebe Grüße, Tim'


// Whether a scroller scrolls only when what is in it and one inset under
// it do not fit, issue #77. The window is made just 10 px taller than
// that, where it must not scroll, then 60 px shorter, where it must, and
// scrolled to the end the last content stands one inset above the bottom.
// Only the height changes, so nothing wraps anew.
async function scrollsOnlyWhenNeeded(page, where, scroller, content) {
  const measure = () => page.evaluate(([scroller, content]) => {
    const s = document.querySelector(scroller)
    const c = [...document.querySelectorAll(content)].at(-1)
    const inset = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--pad'))
    const end = c.getBoundingClientRect().bottom - s.getBoundingClientRect().top + s.scrollTop
    return { need: end + inset, room: s.clientHeight, over: s.scrollHeight - s.clientHeight, inset, height: innerHeight, width: innerWidth }
  }, [scroller, content])
  const m = await measure()
  await page.setViewportSize({ width: m.width, height: Math.round(m.height + m.need + 10 - m.room) })
  const fits = await measure()
  if (fits.over > 0) return `${where} fits with ${Math.round(fits.room - fits.need)} px to spare and still scrolls by ${fits.over} px`
  await page.setViewportSize({ width: m.width, height: Math.round(m.height + m.need - 60 - m.room) })
  const short = await measure()
  if (short.over <= 0) return `${where} does not fit and cannot be scrolled`
  await page.evaluate((scroller) => { const s = document.querySelector(scroller); s.scrollTop = s.scrollHeight }, scroller)
  const end = await page.evaluate(([scroller, content]) => {
    const s = document.querySelector(scroller)
    return s.getBoundingClientRect().top + s.clientHeight - [...document.querySelectorAll(content)].at(-1).getBoundingClientRect().bottom
  }, [scroller, content])
  if (Math.abs(end - short.inset) > 1) return `${where} scrolled to the end leaves ${Math.round(end)} px under the last section, not ${short.inset}`
  return ''
}

const walks = [
  {
    // Tim, 21 November 2026: no empty avatar, the name on the row of the
    // cross, and then one row of what he does, Draft between Connect and
    // LinkedIn.
    name: 'A person starts with the name beside the cross, then Connect, Draft and LinkedIn in one row',
    async run(page) {
      await page.goto(`${url}/app/people/1003`)
      const name = page.locator('.detail-top .person-name')
      await page.locator('.person .actions a.btn').waitFor()
      const want = (await person(page, 1003)).name
      if (!(await name.count())) return 'no name stands beside the cross'
      if ((await name.innerText()).trim() !== want) return `the name beside the cross reads ${await name.innerText()}`
      const [n, x] = await Promise.all([name.boundingBox(), page.locator('.detail-top button[aria-label="Close"]').boundingBox()])
      if (Math.abs(n.y + n.height / 2 - (x.y + x.height / 2)) > 3) return 'the name and the cross are not on one row'
      if (await page.locator('.person .avatar').count()) return 'the avatar is still there'
      const row = await page.locator('.person .actions > *').evaluateAll((all) => all.map((e) => [e.getAttribute('aria-label') ?? e.textContent.trim(), Math.round(e.getBoundingClientRect().top)]))
      const order = row.map(([what]) => what).join(', ')
      if (order !== 'Connect, Draft, LinkedIn, Not this person') return `the row reads ${order}`
      if (new Set(row.map(([, top]) => top)).size !== 1) return 'the buttons are not in one row'
      return ''
    },
  },
  {
    // Tim never wants to scroll inside the first message, 21 November
    // 2026. The box grows with what is typed and fits again when the
    // window narrows and the lines wrap anew.
    name: 'The first message is as tall as its text, when typed in and at any width',
    async run(page) {
      await page.goto(`${url}/app/people/1003`)
      await page.locator('.person .actions a.btn').waitFor()
      await keepDraft(page, 1003, message)
      await page.reload()
      const box = page.locator('.first-message textarea')
      await box.waitFor()
      const fits = () => box.evaluate((t) => ({ over: t.scrollHeight - t.clientHeight, resize: getComputedStyle(t).resize, h: t.clientHeight }))
      let m = await fits()
      if (m.over > 0) return `the message is ${m.over} px taller than its box`
      if (m.resize !== 'none') return `the box can still be pulled longer, resize is ${m.resize}`
      const before = m.h
      await box.click()
      await page.keyboard.press('Control+End')
      await page.keyboard.type('\n\nPS: Eine Zeile mehr.\nUnd noch eine.')
      m = await fits()
      if (m.over > 0) return `after typing, the message is ${m.over} px taller than its box`
      if (m.h <= before) return 'the box did not grow with what was typed'
      await page.setViewportSize({ width: 1100, height: 860 })
      await page.waitForFunction(() => { const t = document.querySelector('.first-message textarea'); return t && t.scrollHeight <= t.clientHeight })
      return ''
    },
  },
  {
    // Tim, 22 November 2026: an emptied message took its box with it, and
    // only Draft brought it back, with a new message he did not want. The
    // box stays, for a person with no message too, so he can write one
    // himself.
    name: 'The first message box stays when it is emptied, and what is written in it is kept',
    async run(page) {
      await page.goto(`${url}/app/people/1003`)
      await page.locator('.person .actions a.btn').waitFor()
      await keepDraft(page, 1003, message)
      // The message was drafted by the model an hour ago.
      await page.route((u) => u.pathname === '/api/people/1003', async (route) => {
        if (route.request().method() !== 'GET') return route.continue()
        const r = await route.fetch()
        const p = await r.json()
        return route.fulfill({ response: r, json: { ...p, drafted_at: p.draft ? new Date(Date.now() - 3600e3).toISOString() : p.drafted_at } })
      })
      await page.reload()
      const box = page.locator('.first-message textarea')
      await box.waitFor()
      await page.locator('.first-message .state').filter({ hasText: 'Drafted' }).waitFor()
      await box.fill('')
      if (!(await box.count())) return 'the box went away once it was emptied'
      await box.blur()
      await page.waitForFunction(() => document.querySelector('.first-message .state')?.textContent === 'Saved')
      if ((await person(page, 1003)).draft !== '') return 'the emptied message was not kept'
      await box.click()
      await page.keyboard.type('H')
      const said = await page.locator('.first-message .state').innerText()
      if (said.startsWith('Drafted')) return `over a message Tim writes himself it says ${said}`
      await page.keyboard.type('allo Mara, ich schreibe selbst.')
      await box.blur()
      await page.waitForFunction(() => document.querySelector('.first-message .state')?.textContent === 'Saved')
      if ((await person(page, 1003)).draft !== 'Hallo Mara, ich schreibe selbst.') return 'what was written in the empty box was not kept'
      await keepDraft(page, 1003, '')
      await page.goto(`${url}/app/people/1002`)
      await page.locator('.person .actions a.btn').waitFor()
      await keepDraft(page, 1002, '')
      await page.reload()
      await page.locator('.person .actions a.btn').waitFor()
      if (!(await page.locator('.first-message textarea').count())) return 'a person with no message has no box to write one in'
      return ''
    },
  },
  {
    // The review of 22 November 2026: a person opened before answered
    // late and put their name over the one opened after. Holding an arrow
    // key through the list does that.
    name: 'A person answering late never puts their name over the one open now',
    async run(page) {
      let release
      const held = new Promise((r) => (release = r))
      await page.route((u) => u.pathname === '/api/people/1004', async (route) => {
        if (route.request().method() === 'GET') await held
        return route.continue()
      })
      await page.goto(`${url}/app/people`)
      await page.locator('#person-row-1004').waitFor()
      const want = (await person(page, 1000)).name
      await page.locator('#person-row-1004').click()
      await page.locator('#person-row-1000').click()
      const name = page.locator('.detail-top .person-name')
      await page.waitForFunction((n) => document.querySelector('.detail-top .person-name')?.textContent === n, want)
      const late = page.waitForResponse((r) => new URL(r.url()).pathname === '/api/people/1004')
      release()
      await late
      await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))))
      const shown = await name.innerText()
      if (shown !== want) return `person 1000 is open under the name ${shown}`
      return ''
    },
  },
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
    // rejects it, and the toast's Undo brings it back as it was. Like
    // Connect, the button stays, pressed, and the same button again takes
    // it back. Tim found it gone once pressed, testing #67 on 22 November
    // 2026, and could not get anyone out of Rejected.
    name: 'Not this person rejects the LinkedIn profile, stays, and takes it back',
    async run(page) {
      await page.goto(`${url}/app/people/1001`)
      const linkedin = page.locator('.actions a.btn')
      const notThem = page.getByRole('button', { name: 'Not this person' })
      await linkedin.waitFor()
      const before = await review(page, 1001)
      if ((await notThem.getAttribute('aria-pressed')) !== 'false') return 'Not this person is pressed before it was pressed'
      await notThem.click()
      await linkedin.waitFor({ state: 'detached' })
      if ((await review(page, 1001)) !== 'rejected') return 'the profile was not rejected'
      if (!(await notThem.isVisible())) return 'Not this person is gone once the profile is rejected'
      if ((await notThem.getAttribute('aria-pressed')) !== 'true') return 'Not this person is not pressed once the profile is rejected'
      await page.locator('.toast').getByRole('button', { name: 'Undo' }).click()
      await linkedin.waitFor()
      const after = await review(page, 1001)
      if (after !== before) return `Undo left the profile ${after}, it was ${before}`
      // Once the toast is gone, the same button takes it back.
      await notThem.click()
      await linkedin.waitFor({ state: 'detached' })
      await page.locator('button[aria-label="Not this person"][aria-pressed="true"]').waitFor()
      await notThem.click()
      await linkedin.waitFor()
      if ((await review(page, 1001)) === 'rejected') return 'pressed again, the profile is still rejected'
      if ((await notThem.getAttribute('aria-pressed')) !== 'false') return 'pressed again, Not this person is still pressed'
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
    // Tim, 21 November 2026: he picks the best fit from the top, and every
    // point of the fit has a reason he can read, docs/fit-score.md.
    name: 'People puts the best fit first, and a person says what their fit adds up to',
    async run(page) {
      await page.goto(`${url}/app/people`)
      const set = (v) => page.evaluate((v) => fetch('/api/settings/home_city', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ value: v }) }).then((r) => r.status), v)
      if ((await set('Cologne')) !== 200) return 'Where you are did not take Cologne'
      try {
        await page.reload()
        await page.locator('.prow').first().waitFor()
        const order = (await page.locator('.list-pane').innerText()).includes('Best fit')
        if (!order) return 'the list is not in the order Best fit'
        const fits = await page.$$eval('.prow .fit', (els) => els.map((e) => Number(e.textContent.replace(/\D/g, ''))))
        if (fits.length < 3) return `only ${fits.length} rows show a fit`
        if (fits.some((f, i) => i && f > fits[i - 1])) return `the fits are not best first: ${fits.join(' ')}`
        await page.locator('.prow').first().click()
        await page.locator('.fit .part').first().waitFor()
        const parts = await page.$$eval('.fit li .pts', (els) => els.map((e) => Number(e.textContent.replace(/\D/g, ''))))
        const head = Number((await page.locator('h3:has-text("Fit") .num').innerText()).trim())
        if (parts.length !== 4) return `${parts.length} parts, want 4`
        if (parts.reduce((a, b) => a + b, 0) !== head || head !== fits[0]) return `the parts ${parts.join(' + ')} are not the fit ${head}, the row says ${fits[0]}`
        const near = await page.locator('.fit .part').nth(2).innerText()
        if (!/where you are|km/.test(near)) return `Near you says ${near}`
        return ''
      } finally {
        await set('')
      }
    },
  },
  {
    // Tim, 22 November 2026: people who founded something but work as an
    // employee elsewhere stood high. Who they are takes points away for
    // that, shown with a minus, and the parts still add up to the fit.
    name: 'Fit is one list: someone employed elsewhere loses points under Who they are, with a minus and a cross under it',
    async run(page) {
      await page.goto(`${url}/app/people/1009`)
      await page.locator('.fit .part').first().waitFor()
      const who = await page.locator('.fit .part').first().innerText()
      if (!/^−\d+/.test(who.trim()) || !who.includes('Employed elsewhere')) return `Who they are says ${who}`
      // Tim, 22 November 2026: one list, not Fit and Why saying the same
      // twice. The cross and the tick sit under the part they count in.
      const against = await page.locator('.fit .part').first().locator('li.against').allInnerTexts()
      if (!against.some((t) => t.includes('Employed elsewhere'))) return `the crosses under Who they are: ${against.join(' | ')}`
      if (await page.locator('.fit .part').first().locator('li:not(.against):not(.neither)', { hasText: 'Runs their own business' }).count() !== 1) return 'Runs their own business has no tick under Who they are'
      if (await page.locator('h3', { hasText: /^Why$/ }).count()) return 'Why is still a list of its own'
      if (await page.locator('.fit .part').nth(1).locator('li').count() !== 1) return 'Active lately shows more than its why'
      const parts = await page.$$eval('.fit li .pts', (els) => els.map((e) => Number(e.textContent.replace('−', '-').replace('+', ''))))
      const head = Number((await page.locator('h3:has-text("Fit") .num').innerText()).trim())
      if (parts.reduce((a, b) => a + b, 0) !== head) return `the parts ${parts.join(' ')} are not the fit ${head}`
      return ''
    },
  },
  {
    // Tim, 21 November 2026, issue #66: people whose profile he rejected
    // stayed among those he still has to go through. Rejected, a person
    // leaves All for Rejected, the counts follow and the person stays
    // open. A search by name under All leads there, and the same button
    // brings them back where they stood.
    name: 'Not this person moves a person from All to Rejected, and pressed again brings them back',
    async run(page) {
      const count = (label) => page.locator('main .filters button', { hasText: label }).locator('.n').innerText().then(Number)
      const rows = () => page.locator('.prow').evaluateAll((rs) => rs.map((r) => r.id))
      await page.goto(`${url}/app/people`)
      await page.locator('.prow').first().waitFor()
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
      await page.locator('.detail-pane').getByRole('button', { name: 'Not this person' }).click()
      await page.locator(`#${id}`).waitFor({ state: 'detached' })
      await page.keyboard.press('Escape')
      await page.getByLabel('Search people').first().fill('')
      await page.locator('main .filters button', { hasText: 'All' }).click()
      await page.locator(`#${id}`).waitFor()
      if (JSON.stringify(await rows()) !== JSON.stringify(before)) wrong.push(`undone, the list is ${await rows()}, was ${before}`)
      if ((await count('Rejected')) !== rejected) wrong.push(`undone, Rejected counts ${await count('Rejected')}, want ${rejected}`)
      return wrong.join('\n\t')
    },
  },
  {
    // Tim, 22 November 2026, issue #77: everything on the person fitted,
    // and the page still scrolled. Under the last section stood more room
    // than the window had left.
    name: 'A person who fits does not scroll, beside the list and in the narrow sheet, nor does Searches',
    async run(page) {
      const wrong = []
      await page.goto(`${url}/app/people/1003`)
      await page.locator('.detail-body .person .actions a.btn').waitFor()
      wrong.push(await scrollsOnlyWhenNeeded(page, 'the person beside the list', '.detail-body', '.detail-body .person'))
      await page.setViewportSize({ width: 390, height: 844 })
      await page.goto(`${url}/app/people/1003`)
      await page.locator('.sheet .person .actions a.btn').waitFor()
      wrong.push(await scrollsOnlyWhenNeeded(page, 'the person in the sheet', '.sheet', '.sheet-body .person'))
      await page.setViewportSize({ width: 1280, height: 900 })
      await page.goto(`${url}/app/searches`)
      await page.locator('main button.toggle').first().waitFor()
      wrong.push(await scrollsOnlyWhenNeeded(page, 'Searches', 'main', 'main .page-body > *'))
      return wrong.filter(Boolean).join('\n\t')
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
