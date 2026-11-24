// What every walk starts from: Playwright's Chromium, and the interface
// served by Vite against the mock in web/mock/, on a port of its own, so
// several walks can run side by side. `import { url, server } from
// './lib/mock.mjs'` starts it, and the walk closes the server when done.
//
// The mock answers at once here. For a person trying the interface in
// `make mock` it waits a little before each answer, the way the app over
// HTTP does, so they see what shows while an answer is on its way. A walk
// never waits on the clock: one that has to see a request on its way
// holds that request itself, with page.route, and lets it go.
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

const web = fileURLToPath(new URL('../..', import.meta.url))
process.env.MOCK = '1'
process.env.MOCK_DELAY_MS = '0'
export const server = await createServer({ root: web, configFile: `${web}/vite.config.ts`, logLevel: 'error', server: { port: 0, strictPort: false } })
await server.listen()
export const url = server.resolvedUrls.local[0].replace(/\/$/, '')
