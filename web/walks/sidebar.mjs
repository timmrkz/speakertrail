// Walks through the sidebar the way FrameFairy's walks do, ported from
// its frontend/preview/walks: what macOS and WebKit hand the app is
// played to it, and what the sidebar does about it is checked. They run
// against the interface with invented data, the mock, in Chromium. `make
// walks` runs them, and so does CI.
//
// No walk waits on the clock: each takes a step and waits for the
// sidebar to be still, its transitions finished, before it looks.
import { chromium } from 'playwright'
import { createServer } from 'vite'
import { fileURLToPath } from 'node:url'
import { existsSync } from 'node:fs'

// Without Playwright's Chromium there is nothing to walk in. Say so in one
// line rather than in a stack trace. CI installs it, a cloud session gets
// it from scripts/cloud-setup.sh.
if (!existsSync(chromium.executablePath())) {
  console.error(`The walks need Playwright's Chromium, and it is not at ${chromium.executablePath()}. Install it with: cd web && npx playwright install chromium`)
  process.exit(1)
}

const web = fileURLToPath(new URL('..', import.meta.url))
process.env.MOCK = '1'
const server = await createServer({ root: web, configFile: `${web}/vite.config.ts`, logLevel: 'error', server: { port: 0, strictPort: false } })
await server.listen()
const url = server.resolvedUrls.local[0].replace(/\/$/, '')

// On the rail, under the title bar, and well off it over the page.
const RAIL = [20, 500]
const PAGE = [750, 400]

// Once the sidebar has done whatever it does: two frames, and every
// transition on it finished.
async function still(page) {
  await page.evaluate(async () => {
    await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)))
    const aside = document.querySelector('aside')
    while (aside?.getAnimations().length) await Promise.all(aside.getAnimations().map((a) => a.finished.catch(() => {})))
  })
}

const isOpen = (page) => page.evaluate(() => document.querySelector('aside').classList.contains('open'))

const walks = [
  {
    // Tim had the pointer on FrameFairy's sidebar and swiped up for
    // Mission Control, and saw it open and close over and over until he
    // clicked back into the app.
    name: 'The sidebar stays shut under a pointer that does not move, and while the app is not in front',
    async run(page) {
      await page.evaluate(() => {
        window.__opened = 0
        const aside = document.querySelector('aside')
        let was = aside.classList.contains('open')
        new MutationObserver(() => {
          const now = aside.classList.contains('open')
          if (now && !was) window.__opened++
          was = now
        }).observe(aside, { attributes: true, attributeFilter: ['class'] })
      })
      await page.mouse.move(...PAGE)
      await still(page)
      await page.mouse.move(...RAIL)
      await still(page)
      if (!(await isOpen(page))) return 'the sidebar did not open under the pointer'
      // What WebKit makes of the mouse exited and entered macOS sends while
      // the pointer stands still: the pointer leaving, and moves to where
      // it is with no movement.
      const stand = (leave) =>
        page.evaluate(([leave, x, y]) => {
          const aside = document.querySelector('aside')
          const at = { clientX: x, clientY: y, pointerType: 'mouse', isPrimary: true }
          if (leave) {
            aside.dispatchEvent(new PointerEvent('pointerout', { ...at, bubbles: true }))
            aside.dispatchEvent(new PointerEvent('pointerleave', at))
          } else {
            aside.dispatchEvent(new PointerEvent('pointerover', { ...at, bubbles: true }))
            aside.dispatchEvent(new PointerEvent('pointerenter', at))
            aside.dispatchEvent(new PointerEvent('pointermove', { ...at, bubbles: true }))
          }
        }, [leave, ...RAIL])
      const before = await page.evaluate(() => window.__opened)
      for (let i = 0; i < 4; i++) {
        await stand(true)
        await still(page)
        await stand(false)
        await still(page)
      }
      const again = (await page.evaluate(() => window.__opened)) - before
      if (again) return `the sidebar opened ${again} times more under a pointer that did not move`
      if (await isOpen(page)) return 'the sidebar is open after the pointer left it and came back without moving'
      // The app goes behind another: the sidebar answers no pointer.
      await page.mouse.move(...PAGE)
      await still(page)
      await page.evaluate(() => {
        document.hasFocus = () => false
        window.dispatchEvent(new FocusEvent('blur'))
      })
      await page.mouse.move(...RAIL)
      await still(page)
      if (await isOpen(page)) return 'the sidebar opened under the pointer while the app was not in front'
      // In front again with the pointer on it, the next move opens it.
      await page.evaluate(() => {
        delete document.hasFocus
        window.dispatchEvent(new FocusEvent('focus'))
      })
      await page.mouse.move(RAIL[0] + 4, RAIL[1])
      await still(page)
      if (!(await isOpen(page))) return 'the sidebar stayed shut under a pointer that moved on it once the app was in front again'
      await page.mouse.move(...PAGE)
      await still(page)
      return ''
    },
  },
  {
    // Tim hovered FrameFairy's sidebar open and clicked its button to keep
    // it, and it closed.
    name: "One click on the sidebar's button keeps a sidebar hovering opened, and the next closes it",
    async run(page) {
      const button = page.locator('aside .head .glyph')
      const seen = async () => ({
        open: await isOpen(page),
        title: await button.getAttribute('title'),
        pressed: await button.getAttribute('aria-pressed'),
        icon: await button.innerHTML(),
      })
      await page.mouse.move(...PAGE)
      await still(page)
      await page.mouse.move(...RAIL)
      await still(page)
      const hovered = await seen()
      if (!hovered.open || hovered.pressed !== 'false' || hovered.title !== 'Keep the sidebar open')
        return `the sidebar hovering opened shows ${JSON.stringify({ ...hovered, icon: undefined })}, open with a button that keeps it`
      await button.click()
      await page.mouse.move(...PAGE)
      await still(page)
      const kept = await seen()
      if (!kept.open) return 'one click on the button of a sidebar hovering opened did not keep it open once the pointer left'
      if (kept.pressed !== 'true' || kept.title !== 'Close the sidebar') return `the kept sidebar's button says ${JSON.stringify({ pressed: kept.pressed, title: kept.title })}, not that it closes it`
      if (kept.icon === hovered.icon) return 'the button wears the same icon whether the sidebar is kept or not'
      // Kept, it is still kept when the app opens again.
      await page.reload()
      await page.locator('aside').waitFor()
      await still(page)
      if (!(await isOpen(page))) return 'the kept sidebar is shut once the app opens again'
      await page.mouse.move(...RAIL)
      await still(page)
      await button.click()
      await still(page)
      const closed = await seen()
      if (closed.open) return 'a click on the button of a kept sidebar did not close it under the pointer'
      if (closed.pressed !== 'false' || closed.title !== 'Keep the sidebar open' || closed.icon !== hovered.icon)
        return `once closed, the button says ${JSON.stringify({ pressed: closed.pressed, title: closed.title })}, not that it keeps it`
      await page.mouse.move(...PAGE)
      await still(page)
      return ''
    },
  },
  {
    // FrameFairy's logs showed its sidebar, opened by the pointer, closing
    // again within 100 ms one time in three, on a leave no hand made.
    name: 'The sidebar opened by the pointer stays open on a leave with no move, and closes on a real one',
    async run(page) {
      await page.mouse.move(...PAGE)
      await still(page)
      await page.evaluate(() => {
        window.__closed = 0
        const aside = document.querySelector('aside')
        let was = aside.classList.contains('open')
        new MutationObserver(() => {
          const now = aside.classList.contains('open')
          if (was && !now) window.__closed++
          was = now
        }).observe(aside, { attributes: true, attributeFilter: ['class'] })
      })
      // The leave and what follows it, as WebKit sends them: out of the
      // sidebar into the page beside the rail, and a move with no
      // movement there.
      const leave = () =>
        page.evaluate(([x, y]) => {
          const aside = document.querySelector('aside')
          const at = { clientX: x, clientY: y, pointerType: 'mouse', isPrimary: true }
          const beside = document.elementsFromPoint(400, y).find((el) => !aside.contains(el)) ?? document.body
          aside.dispatchEvent(new PointerEvent('pointerout', { ...at, bubbles: true, relatedTarget: beside }))
          aside.dispatchEvent(new PointerEvent('pointerleave', { ...at, relatedTarget: beside }))
          beside.dispatchEvent(new PointerEvent('pointerover', { ...at, bubbles: true, relatedTarget: aside }))
          beside.dispatchEvent(new PointerEvent('pointermove', { ...at, bubbles: true }))
        }, RAIL)
      await page.mouse.move(...RAIL)
      await leave()
      await still(page)
      await leave()
      await still(page)
      const closed = await page.evaluate(() => window.__closed)
      if (closed || !(await isOpen(page))) return `the sidebar opened by the pointer closed ${closed} times on a leave with no move of the pointer`
      await page.mouse.move(...PAGE)
      if (await isOpen(page)) return 'the sidebar stayed open after the pointer moved off it'
      await still(page)
      // The pointer leaving the app altogether, to nothing.
      await page.mouse.move(...RAIL)
      await still(page)
      await page.evaluate(([x, y]) => {
        document.querySelector('aside').dispatchEvent(new PointerEvent('pointerleave', { clientX: x, clientY: y, pointerType: 'mouse', isPrimary: true }))
      }, RAIL)
      if (await isOpen(page)) return 'the sidebar stayed open after the pointer left the app'
      // The same where WebKit names the page itself as where the pointer
      // went, at a point left of the app.
      await still(page)
      await page.mouse.move(...PAGE)
      await still(page)
      await page.mouse.move(...RAIL)
      await still(page)
      await page.evaluate(([, y]) => {
        const at = { clientX: -1, clientY: y, pointerType: 'mouse', isPrimary: true, relatedTarget: document.documentElement }
        document.querySelector('aside').dispatchEvent(new PointerEvent('pointerleave', at))
      }, RAIL)
      if (await isOpen(page)) return 'the sidebar stayed open after the pointer left the app to its left'
      return ''
    },
  },
  {
    // Escape closes one thing at a time, the way a menu on the Mac does:
    // the sidebar the hover opened first, the person behind it next.
    name: 'Escape closes a sidebar the hover opened first, and the person on the next press',
    async run(page) {
      await page.goto(`${url}/app/people/1001`)
      await page.locator('.detail-pane').waitFor()
      await page.mouse.move(...RAIL)
      await still(page)
      if (!(await isOpen(page))) return 'the sidebar did not open under the pointer'
      await page.keyboard.press('Escape')
      await still(page)
      if (await isOpen(page)) return 'Escape left the sidebar the hover opened open'
      if (!(await page.locator('.detail-pane').count())) return 'the first Escape closed the person as well as the sidebar'
      await page.keyboard.press('Escape')
      await page.locator('.detail-pane').waitFor({ state: 'detached' })
      return ''
    },
  },
]

const browser = await chromium.launch()
let failed = 0
for (const w of walks) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 860 } })
  const errors = []
  page.on('pageerror', (e) => errors.push(e.message))
  await page.goto(`${url}/app/people`)
  await page.locator('aside').waitFor()
  await page.locator('.prow').first().waitFor()
  let wrong
  try {
    wrong = await w.run(page)
  } catch (e) {
    wrong = String(e)
  }
  if (!wrong && errors.length) wrong = `the page threw: ${errors.join(', ')}`
  console.log(`${wrong ? 'FAIL' : 'ok  '}\t${w.name}${wrong ? `\n\t${wrong}` : ''}`)
  if (wrong) failed++
  await page.close()
}
await browser.close()
await server.close()
process.exit(failed ? 1 : 0)
