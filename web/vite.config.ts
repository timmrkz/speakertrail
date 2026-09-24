import { defineConfig, loadEnv, type Plugin, type PluginOption } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Go embeds dist/ and needs at least one file there, even before a build.
// The build empties dist/, so it writes the empty .keep file again.
function keepFile(): Plugin {
  return {
    name: 'speakertrail-keep-file',
    apply: 'build',
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: '.keep', source: '' })
    },
  }
}

// `MOCK=1 npm run dev` serves the API from fixture data in mock/.
// Without it, /api goes to the Go server on :8080.
// The mock is imported only for the dev server, so it never reaches dist/.
export default defineConfig(async ({ command, mode }) => {
  const env = loadEnv(mode, '.', '')
  const mock = command === 'serve' && env.MOCK === '1'
  const plugins: PluginOption[] = [svelte(), keepFile()]
  if (mock) {
    const { mockApi } = await import('./mock/plugin.ts')
    plugins.push(mockApi())
  }
  const proxy = mock ? undefined : { '/api': 'http://localhost:8080' }
  return {
    plugins,
    server: { proxy },
    preview: { proxy },
    build: { outDir: 'dist', emptyOutDir: true, target: 'es2022' },
  }
})
