// Walks through the speech model in Settings against the mock, in
// Chromium, the way walks/logs.mjs does. The download and its unpacking
// are answered by the walk itself, so nothing waits on the clock.
// `make walks` runs them, and so does CI.
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

// Settings with the speech model's download where the walk puts it.
async function installing(page, progress) {
  await page.route('**/api/speech', async (route) => {
    const res = await route.fetch()
    const body = await res.json()
    body.models = body.models.map((m) => ({ ...m, installed: false, inUse: false }))
    body.job = { id: 'speech-1', kind: 'model', title: body.models[0].title, state: 'running', progress }
    await route.fulfill({ response: res, json: body })
  })
  await page.goto(`${url}/app/settings`)
  await page.locator('.speech .line').filter({ hasText: /^(Unpacking|Downloading)/ }).waitFor()
}

// How far the fill in a control has come, from 0 to 1, as it is drawn.
const filled = (button) => button.evaluate((b) => {
  const i = b.querySelector('.fill i')
  if (!i) return -1
  return 1 + new DOMMatrix(getComputedStyle(i).transform).m41 / i.getBoundingClientRect().width
})

const walks = [
  {
    // Unpacking takes over a minute on Tim's Mac, so it is told like the
    // download: how far, against what the model takes once unpacked, and
    // the time left, with the fill in Cancel. Tim saw only "Unpacking"
    // testing #24 on 17 November 2026.
    name: 'Unpacking the speech model says how far it has come and how long is left',
    async run(page) {
      await installing(page, { text: 'unpacking', fraction: 0.4, remaining: 45 })
      const line = (await page.locator('.speech .line').innerText()).trim()
      if (!/^Unpacking, 269 MB of 671 MB, about 45 s left$/.test(line)) return `the line reads ${JSON.stringify(line)}`
      const cancel = page.locator('.speech button.act')
      if ((await cancel.innerText()).trim() !== 'Cancel') return `the control reads ${await cancel.innerText()}`
      await page.waitForFunction(() => {
        const i = document.querySelector('.speech button.act .fill i')
        return i && getComputedStyle(i).transform !== 'none' && !i.getAnimations().some((a) => a.playState === 'running' && a.transitionProperty === 'transform')
      })
      const f = await filled(cancel)
      if (Math.abs(f - 0.4) > 0.01) return `the fill is at ${f.toFixed(3)}, want 0.4`
      const title = await cancel.getAttribute('title')
      if (!title.startsWith('Stop unpacking')) return `Cancel says ${JSON.stringify(title)}`
      return ''
    },
  },
  {
    // The download reads as it did, against the archive's size.
    name: 'Downloading the speech model still says how far it has come',
    async run(page) {
      await installing(page, { text: 'fetching 120 MB of 487 MB', fraction: 0.5, remaining: 30 })
      const line = (await page.locator('.speech .line').innerText()).trim()
      if (!/^Downloading, 244 MB of 487 MB, about 30 s left$/.test(line)) return `the line reads ${JSON.stringify(line)}`
      const title = await page.locator('.speech button.act').getAttribute('title')
      if (!title.startsWith('Stop the download')) return `Cancel says ${JSON.stringify(title)}`
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
