// Walks the first message template in Settings against the mock, in
// Chromium, the way walks/draft.mjs does: the example, a variation
// changed, refused and undone, a part added and removed, the first
// template put back, and 390 px. The mock checks changes the way
// internal/store/template.go does. `make walks` runs them, and so does CI.
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

const tpl = (page) => page.locator('section.template')
const part = (page, name) => tpl(page).locator('details.part').filter({ has: page.locator(`summary .name:text-is("${name}")`) })
const toastUndo = (page) => page.locator('.toast button').filter({ hasText: 'Undo' })
const example = (page) => tpl(page).locator('.example-text')

async function settings(page) {
  // Every walk starts from Tim's first template.
  await page.goto(`${url}/app/settings`)
  await page.evaluate(() => fetch('/api/template', { method: 'DELETE', headers: { 'Content-Type': 'application/json' }, body: '{}' }))
  await page.reload()
  await tpl(page).locator('details.part').first().waitFor()
  await page.waitForFunction(() => document.querySelector('section.template .example-text')?.textContent?.includes('[reason]'))
}

const walks0 = [
  {
    name: 'The template shows every part, origin open, and an example of how they fit',
    async run(page) {
      await settings(page)
      const names = await tpl(page).locator('summary .name').allInnerTexts()
      if (names.slice(0, 4).join(' ') !== 'origin greeting opener hook_frame' || names.length !== 13) return `the parts are ${names.join(' ')}`
      if (!(await part(page, 'origin').evaluate((d) => d.open))) return 'origin is closed'
      if (await part(page, 'opener').evaluate((d) => d.open)) return 'opener is open from the start'
      const text = await example(page).innerText()
      if (!/^Hallo \[first_name\],\n\n/.test(text) || !text.includes('als [portrait] zu erzählen') || !text.includes('Liebe Grüße, Tim')) return `the example reads ${JSON.stringify(text)}`
      const count = await part(page, 'opener').locator('.count').innerText()
      if (count !== '7 variations') return `opener says ${count}`
      return ''
    },
  },
  {
    name: 'A variation saves when it is left, shows in the example, and Undo puts it back',
    async run(page) {
      await settings(page)
      await part(page, 'greeting').locator('summary').click()
      const box = part(page, 'greeting').locator('textarea').first()
      await box.fill('Servus')
      await box.blur()
      await page.locator('.toast').filter({ hasText: 'Saved greeting' }).waitFor()
      await page.waitForFunction(() => document.querySelector('section.template .example-text')?.textContent?.startsWith('Servus [first_name]'))
      const saved = await page.evaluate(() => fetch('/api/template').then((r) => r.json()).then((t) => t.parts.find((p) => p.name === 'greeting').variations))
      if (JSON.stringify(saved) !== '["Servus"]') return `the app kept ${JSON.stringify(saved)}`
      if (!(await tpl(page).locator('button.reset').count())) return 'nothing offers the first template back'
      await toastUndo(page).click()
      await page.waitForFunction(() => document.querySelector('section.template .example-text')?.textContent?.startsWith('Hallo [first_name]'))
      if ((await box.inputValue()) !== 'Hallo') return `after Undo the box reads ${await box.inputValue()}`
      return ''
    },
  },
  {
    name: 'A change shows in the same example, so the example keeps its shape',
    async run(page) {
      await settings(page)
      const before = await example(page).innerText()
      await part(page, 'greeting').locator('summary').click()
      const box = part(page, 'greeting').locator('textarea').first()
      await box.fill('Servus')
      await box.blur()
      await page.locator('.toast').filter({ hasText: 'Saved greeting' }).waitFor()
      await page.waitForFunction(() => document.querySelector('section.template .example-text')?.textContent?.startsWith('Servus'))
      const after = await example(page).innerText()
      if (after !== before.replace(/^Hallo/, 'Servus')) return `the example became\n${after}\nnot\n${before.replace(/^Hallo/, 'Servus')}`
      return ''
    },
  },
  {
    name: 'One part is open at a time, and every × and + sits in one column of one size',
    async run(page) {
      await settings(page)
      await part(page, 'greeting').locator('summary').click()
      await page.waitForFunction(() => document.querySelectorAll('section.template details.part[open]').length === 1)
      const open = await tpl(page).locator('details.part[open] summary .name').innerText()
      if (open !== 'greeting') return `${open} is open, not greeting`
      const icons = await tpl(page).locator('summary button.remove-part, .variation .btn, .add-part .btn').evaluateAll((bs) =>
        bs.filter((b) => b.offsetParent).map((b) => { const r = b.getBoundingClientRect(); return [Math.round(r.right), Math.round(r.width), Math.round(r.height)].join(' ') }))
      const kinds = [...new Set(icons)]
      if (icons.length < 15 || kinds.length !== 1) return `the icons end at, are wide and high: ${kinds.join(', ')}`
      if (await tpl(page).locator('button', { hasText: 'Remove greeting' }).count()) return 'a second button still removes greeting'
      return ''
    },
  },
  {
    name: 'A save leaves New example as it is, only its own press makes it wait',
    async run(page) {
      await settings(page)
      await page.evaluate(() => {
        window.flips = 0
        const b = [...document.querySelectorAll('section.template .example button')][0]
        new MutationObserver(() => window.flips++).observe(b, { attributes: true, attributeFilter: ['disabled'] })
      })
      await part(page, 'greeting').locator('summary').click()
      const box = part(page, 'greeting').locator('textarea').first()
      await box.fill('Servus')
      await box.blur()
      await page.waitForFunction(() => document.querySelector('section.template .example-text')?.textContent?.startsWith('Servus'))
      const flips = await page.evaluate(() => window.flips)
      if (flips) return `New example went grey and back ${flips / 2} times on a save`
      return ''
    },
  },
  {
    name: 'The part clicked stays under the pointer when the one above it closes, in WebKit too',
    viewport: { width: 1100, height: 700 },
    async run(page) {
      for (const anchoring of ['auto', 'none']) {
        await settings(page)
        // WebKit keeps nothing in place when content above it shrinks.
        await page.addStyleTag({ content: `* { overflow-anchor: ${anchoring} !important }` })
        const row = part(page, 'ending').locator('summary')
        await row.scrollIntoViewIfNeeded()
        const was = (await row.boundingBox()).y
        await row.click()
        await page.waitForFunction(() => document.querySelectorAll('section.template details.part[open]').length === 1)
        await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => r())))
        const now = (await row.boundingBox()).y
        if (Math.abs(now - was) > 1) return `with overflow-anchor ${anchoring} the row moved from ${was} to ${now}`
      }
      return ''
    },
  },
  {
    name: 'Tab from one variation to the next keeps what is typed in both',
    async run(page) {
      await settings(page)
      await part(page, 'opener').locator('summary').click()
      const boxes = part(page, 'opener').locator('textarea')
      await boxes.nth(0).fill('erstens')
      // Tab goes to its ×, then to the next variation, while the first
      // saves.
      await boxes.nth(0).press('Tab')
      await page.keyboard.press('Tab')
      await page.keyboard.press('ControlOrMeta+a')
      await page.keyboard.type('zweitens')
      const focused = await page.evaluate(() => document.activeElement?.getAttribute('aria-label'))
      if (focused !== 'Variation 2 of opener') return `after Tab the focus is on ${focused}`
      await boxes.nth(1).blur()
      await page.waitForFunction(() => fetch('/api/template').then((r) => r.json()).then((t) => {
        const v = t.parts.find((p) => p.name === 'opener').variations
        return v[0] === 'erstens' && v[1] === 'zweitens'
      }))
      const shown = await boxes.evaluateAll((ts) => ts.slice(0, 2).map((t) => t.value))
      if (shown.join(',') !== 'erstens,zweitens') return `the boxes read ${shown}`
      return ''
    },
  },
  {
    name: 'A variation that uses a part there is not is refused, says why, and Escape puts it back',
    async run(page) {
      await settings(page)
      await part(page, 'ending').locator('summary').click()
      const box = part(page, 'ending').locator('textarea').first()
      const was = await box.inputValue()
      await box.fill('#nope# kennen zu lernen.')
      await box.blur()
      await part(page, 'ending').locator('.field-error').filter({ hasText: 'uses #nope#, and there is no part of that name' }).waitFor()
      await box.focus()
      await box.press('Escape')
      if ((await box.inputValue()) !== was) return `Escape left ${JSON.stringify(await box.inputValue())}`
      if (await part(page, 'ending').locator('.field-error').count()) return 'the error stayed after Escape'
      return ''
    },
  },
  {
    name: 'A variation is added and removed, and a part needs one',
    async run(page) {
      await settings(page)
      await part(page, 'intensity').locator('summary').click()
      const p = part(page, 'intensity')
      await p.locator('form.add input').fill('richtig gern')
      await p.locator('button[aria-label="Add a variation to intensity"]').click()
      await page.locator('.toast').filter({ hasText: 'Added a variation to intensity' }).waitFor()
      await p.locator('textarea').nth(2).waitFor()
      if ((await p.locator('.count').innerText()) !== '3 variations') return `it says ${await p.locator('.count').innerText()}`
      await p.locator('button[aria-label="Remove variation 1 of intensity"]').click()
      await page.locator('.toast').filter({ hasText: 'Removed a variation of intensity' }).waitFor()
      const left = await p.locator('textarea').evaluateAll((ts) => ts.map((t) => t.value))
      if (JSON.stringify(left) !== '["super gern","richtig gern"]') return `left ${JSON.stringify(left)}`
      // The last one cannot go: the app says the part needs one.
      await part(page, 'greeting').locator('summary').click()
      await part(page, 'greeting').locator('button[aria-label="Remove variation 1 of greeting"]').click()
      await part(page, 'greeting').locator('.field-error').filter({ hasText: 'Greeting needs at least one variation' }).waitFor()
      if ((await part(page, 'greeting').locator('textarea').count()) !== 1) return 'the last variation of greeting went'
      return ''
    },
  },
  {
    name: 'A part is added, used and removed, and one in use stays',
    async run(page) {
      await settings(page)
      await tpl(page).locator('#new-part').fill('wave')
      await tpl(page).locator('#new-part').press('Enter')
      await part(page, 'wave').waitFor()
      if (!(await part(page, 'wave').evaluate((d) => d.open))) return 'the new part is not open'
      // Origin stays, and its × says why the app keeps it when pressed,
      // closed as it is now that wave is open, without opening it.
      const keepOrigin = part(page, 'origin').locator('button.remove-part')
      if ((await keepOrigin.getAttribute('aria-disabled')) !== 'true' || (await keepOrigin.getAttribute('title')) !== 'Origin is where the message starts, so it stays.') return 'origin can be removed'
      // Playwright holds aria-disabled for not enabled, a person does not.
      await keepOrigin.click({ force: true })
      await page.locator('.toast').filter({ hasText: 'Origin is where the message starts, so it stays.' }).waitFor()
      if (await part(page, 'origin').evaluate((d) => d.open)) return 'the × of origin opened it'
      if (!(await part(page, 'origin').count())) return 'origin went'
      await part(page, 'origin').locator('summary').click()
      // In use by origin, wave cannot go either, and says so.
      const origin = part(page, 'origin').locator('textarea').first()
      await origin.fill((await origin.inputValue()) + ' #wave#')
      await origin.blur()
      await page.locator('.toast').filter({ hasText: 'Saved origin' }).waitFor()
      const removeWave = part(page, 'wave').locator('button.remove-part')
      await page.waitForFunction(() => [...document.querySelectorAll('section.template button.remove-part')].some((b) => b.title === 'Origin still uses #wave#. Take it out there first.' && b.getAttribute('aria-disabled') === 'true'))
      // Unused, it goes, and Undo brings it back where it was.
      await origin.fill((await origin.inputValue()).replace(' #wave#', ''))
      await origin.blur()
      await page.waitForFunction(() => !document.querySelector('section.template details.part textarea')?.value.includes('#wave#'))
      await page.waitForFunction(() => [...document.querySelectorAll('section.template button.remove-part')].some((b) => b.getAttribute('aria-label') === 'Remove wave' && !b.hasAttribute('aria-disabled')))
      await removeWave.click()
      await page.locator('.toast').filter({ hasText: 'Removed wave' }).waitFor()
      if (await part(page, 'wave').count()) return 'wave is still there'
      await toastUndo(page).click()
      await part(page, 'wave').waitFor()
      const names = await tpl(page).locator('summary .name').allInnerTexts()
      if (names[names.length - 1] !== 'wave') return `wave came back as part ${names.indexOf('wave') + 1} of ${names.length}`
      // A name that cannot be is refused.
      await tpl(page).locator('#new-part').fill('Big Part')
      await tpl(page).locator('#new-part').press('Enter')
      await tpl(page).locator('.field-error').filter({ hasText: 'cannot name a part' }).waitFor()
      return ''
    },
  },
  {
    name: 'The first template comes back, and Undo brings Tim’s own back',
    async run(page) {
      await settings(page)
      if (await tpl(page).locator('button.reset').count()) return 'the first template is offered while it is in place'
      await part(page, 'signoff').locator('summary').click()
      const box = part(page, 'signoff').locator('textarea').first()
      await box.fill('Bis bald, Tim')
      await box.blur()
      await tpl(page).locator('button.reset').click()
      await page.locator('.toast').filter({ hasText: 'Put back the first template' }).waitFor()
      if ((await box.inputValue()) !== 'Liebe Grüße, Tim') return `after the reset it reads ${await box.inputValue()}`
      await toastUndo(page).click()
      await page.waitForFunction(() => [...document.querySelectorAll('section.template textarea')].some((t) => t.value === 'Bis bald, Tim'))
      return ''
    },
  },
  {
    name: 'At 390 px the template fits with nothing to scroll sideways',
    viewport: { width: 390, height: 844 },
    async run(page) {
      await settings(page)
      await part(page, 'invitation').locator('summary').click()
      const wide = await page.evaluate(() => [document.scrollingElement.scrollWidth, innerWidth])
      if (wide[0] > wide[1]) return `the page is ${wide[0]} px wide in a window of ${wide[1]}`
      const boxes = await tpl(page).locator('textarea, input, button').evaluateAll((es) =>
        es.filter((e) => e.offsetParent).map((e) => e.getBoundingClientRect()).filter((r) => r.x < 0 || r.right > 390).length)
      if (boxes) return `${boxes} controls run past the window`
      // Every variation shows whole, with no box to scroll inside.
      const hidden = await tpl(page).locator('details[open] textarea').evaluateAll((ts) => ts.filter((t) => t.scrollHeight > t.clientHeight + 1).length)
      if (hidden) return `${hidden} variations hide some of their text`
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
