<script lang="ts">
  // The report window, which Bugs in the sidebar opens, one at a time:
  // Report a Problem alone, under the same title bar as the app's window.
  // A picture dropped anywhere on it is attached, see bugs.go. Ported from
  // FrameFairy's screens/ReportWindow.svelte.
  import { onMount } from 'svelte'
  import Report from './Report.svelte'
  import Landing from '../lib/components/Landing.svelte'
  import TitleBar from '../lib/components/TitleBar.svelte'
  import Toast from '../lib/components/Toast.svelte'
  import { titleBar } from '../lib/chrome.svelte'
  import { theme, type ThemeChoice } from '../lib/theme.svelte'

  onMount(() => {
    // Wails lets files be dropped only where the page says so.
    document.documentElement.toggleAttribute('data-file-drop-target', true)
    const noChrome = titleBar.follow('report')
    // Light or dark, changed in Settings in the app's window, reaches this
    // one through the storage both share.
    const themed = (e: StorageEvent) => {
      if (e.key === 'st-theme' || e.key === null) theme.set((e.newValue as ThemeChoice | null) ?? 'system')
    }
    window.addEventListener('storage', themed)
    return () => {
      noChrome()
      window.removeEventListener('storage', themed)
    }
  })
</script>

<div class="shell">
  <TitleBar title="Report a Problem" />
  <main id="main" class="main"><Report /></main>
  <!-- What went wrong, over the window. A toast here is about the report
       itself, so it offers no Report of its own. -->
  <Toast report={false} />
  <!-- Where a picture dragged over the window would land: all of it. -->
  <Landing />
</div>

<style>
  .shell {
    display: flex; flex-direction: column; height: 100dvh;
    background: var(--mist), var(--bg); background-size: 100% 100%;
  }
  .main { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; display: flex; }
</style>
