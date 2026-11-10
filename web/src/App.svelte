<script lang="ts">
  import { navigate, router } from './lib/router.svelte'
  import Toast from './lib/components/Toast.svelte'
  import PrivateApp from './screens/PrivateApp.svelte'

  // The app is the workspace and nothing else. It opens on People.
  $effect(() => {
    if (!router.under('/app')) navigate('/app/people', { replace: true })
  })
</script>

<a class="skip-link" href="#main">Skip to content</a>

{#if router.under('/app')}
  <PrivateApp />
{/if}

<Toast />
