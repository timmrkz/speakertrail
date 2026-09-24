<script lang="ts">
  import { setUnauthorizedHandler } from './lib/api'
  import { navigate, router } from './lib/router.svelte'
  import Toast from './lib/components/Toast.svelte'
  import Home from './screens/Home.svelte'
  import Login from './screens/Login.svelte'
  import PrivateApp from './screens/PrivateApp.svelte'
  import NotFound from './screens/NotFound.svelte'

  // Any 401 from a private endpoint sends Tim to the login, then back.
  setUnauthorizedHandler(() => {
    if (router.under('/app')) navigate(`/login?next=${encodeURIComponent(router.path)}`, { replace: true })
  })
</script>

<a class="skip-link" href="#main">Skip to content</a>

{#if router.path === '/'}
  <Home />
{:else if router.path === '/login'}
  <Login />
{:else if router.under('/app')}
  <PrivateApp />
{:else}
  <NotFound />
{/if}

<Toast />
