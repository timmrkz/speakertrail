<script lang="ts">
  import { api, ApiError } from '../lib/api'
  import { navigate, router } from '../lib/router.svelte'
  import Brand from '../lib/components/Brand.svelte'
  import ThemeButton from '../lib/components/ThemeButton.svelte'

  // `calendarOff` is set when the front page shows the login because the
  // public calendar is switched off.
  let { calendarOff = false }: { calendarOff?: boolean } = $props()

  let password = $state('')
  let error = $state('')
  let busy = $state(false)
  let publicOn = $state(false)
  let field: HTMLInputElement | undefined = $state()

  let next = $derived.by(() => {
    const n = new URLSearchParams(router.search).get('next') ?? ''
    return n.startsWith('/app') ? n : '/app'
  })

  $effect(() => {
    document.title = 'Log in · Speaker Trail'
  })

  // Already logged in? Go straight to the workspace.
  api.me().then((m) => m.logged_in && navigate(next, { replace: true })).catch(() => {})
  $effect(() => {
    if (!calendarOff) api.publicConfig().then((c) => (publicOn = c.public_calendar)).catch(() => {})
  })

  $effect(() => {
    field?.focus()
  })

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    if (!password) {
      error = 'Enter the password.'
      field?.focus()
      return
    }
    busy = true
    error = ''
    try {
      await api.login(password)
      navigate(next, { replace: true })
    } catch (err) {
      error = err instanceof ApiError && err.status === 401 ? 'That password is not right. Try again.' : err instanceof Error ? err.message : 'Could not log in.'
      password = ''
      field?.focus()
    } finally {
      busy = false
    }
  }
</script>

<div class="page">
  <header class="top">
    <Brand href={calendarOff ? '/' : publicOn ? '/' : '/login'} />
    <ThemeButton />
  </header>
  <main id="main" class="center">
    <form class="card" onsubmit={submit} novalidate>
      <div class="intro">
        <h1>Log in</h1>
        <p class="muted">
          {calendarOff ? 'The public calendar is not open yet. This is the private workspace.' : 'The private workspace for people, sources and crawl runs.'}
        </p>
      </div>
      <div class="field">
        <label for="password">Password</label>
        <input
          bind:this={field}
          bind:value={password}
          id="password"
          class="input"
          type="password"
          name="password"
          autocomplete="current-password"
          aria-invalid={error ? 'true' : undefined}
          aria-describedby={error ? 'login-error' : undefined} />
        {#if error}<p id="login-error" class="field-error" role="alert">{error}</p>{/if}
      </div>
      <button class="btn primary wide" type="submit" disabled={busy}>{busy ? 'Logging in' : 'Log in'}</button>
    </form>
    {#if publicOn}<a class="back" href="/">Back to the public calendar</a>{/if}
  </main>
</div>

<style>
  .page { min-height: 100dvh; display: grid; grid-template-rows: auto 1fr; }
  .top { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px var(--gutter); }
  .center { display: grid; place-content: start center; justify-items: center; gap: 16px; padding: 8vh 16px 48px; }
  .card {
    width: min(380px, calc(100vw - 32px)); display: grid; gap: 18px; padding: 24px;
    background: var(--surface); border: 1px solid var(--line); border-radius: 14px; box-shadow: var(--shadow);
  }
  .intro { display: grid; gap: 6px; }
  .wide { width: 100%; height: 40px; }
  .card .input { height: 40px; }
  .back { font-size: 14px; font-weight: 600; }
</style>
