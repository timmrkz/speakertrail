// What every walk starts from: the interface served by Vite against the
// mock in web/mock/, on a port of its own, so several walks can run side
// by side. `import { url, server } from './lib/mock.mjs'` starts it, and
// the walk closes the server when done.
//
// The mock answers at once here. For a person trying the interface in
// `make mock` it waits a little before each answer, the way the app over
// HTTP does, so they see what shows while an answer is on its way. A walk
// never waits on the clock: one that has to see a request on its way
// holds that request itself, with page.route, and lets it go.
//
// The walks launch Chromium headless, which is Playwright's headless
// shell, all CI installs. Without it Playwright says what is missing and
// how to install it. A cloud session has it from scripts/cloud-setup.sh.
import { createServer } from 'vite'
import { fileURLToPath } from 'node:url'

const web = fileURLToPath(new URL('../..', import.meta.url))
process.env.MOCK = '1'
process.env.MOCK_DELAY_MS = '0'
export const server = await createServer({ root: web, configFile: `${web}/vite.config.ts`, logLevel: 'error', server: { port: 0, strictPort: false } })
await server.listen()
export const url = server.resolvedUrls.local[0].replace(/\/$/, '')
