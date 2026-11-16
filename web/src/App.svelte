<script lang="ts">
  import { navigate, router } from './lib/router.svelte'
  import Toast from './lib/components/Toast.svelte'
  import PrivateApp from './screens/PrivateApp.svelte'
  import ReportWindow from './screens/ReportWindow.svelte'

  // The app is the workspace and nothing else. It opens on People. The
  // report window shows the same page at /report, see bugs.go.
  const reportWindow = router.path === '/report'
  $effect(() => {
    if (!reportWindow && !router.under('/app')) navigate('/app/people', { replace: true })
  })
</script>

<a class="skip-link" href="#main">Skip to content</a>

{#if reportWindow}
  <ReportWindow />
{:else}
  {#if router.under('/app')}
    <PrivateApp />
  {/if}
  <Toast />
{/if}
