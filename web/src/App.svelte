<script lang="ts">
  import { navigate, router } from './lib/router.svelte'
  import { api, onQuit, QUIT_AGAIN_MS } from './lib/api'
  import Toast from './lib/components/Toast.svelte'
  import PrivateApp from './screens/PrivateApp.svelte'

  // The app is the workspace and nothing else. It opens on People.
  $effect(() => {
    if (!router.under('/app')) navigate('/app/people', { replace: true })
  })

  // Cmd+Q heard: the app dims at once and says what happens next. The
  // first press always asks, see cmd/speakertrail-app/quit.go. Ported from
  // FrameFairy's App.svelte, issue #17.
  let leaving = $state<'ask' | 'going' | null>(null)
  const quitKey = /Mac/.test(navigator.userAgent) ? '⌘Q' : 'Ctrl+Q'
  // The question taken away, with Escape or a click. The Go side hears it
  // too, so the next Cmd+Q asks again: the question went from the screen
  // and the press still counted, so the next one quit on what looked like
  // the first press.
  function stayOpen() {
    if (leaving !== 'ask') return
    leaving = null
    api.stayOpen().catch(() => {})
  }

  $effect(() => {
    // The question lasts as long as the Go side waits for the second
    // press, quitAgain in quit.go.
    let asked: ReturnType<typeof setTimeout> | undefined
    const dismiss = (e: KeyboardEvent) => {
      if (e.key === 'Escape') stayOpen()
    }
    window.addEventListener('keydown', dismiss)
    const noQuit = onQuit((what) => {
      clearTimeout(asked)
      leaving = what
      if (what === 'ask') asked = setTimeout(() => (leaving = null), QUIT_AGAIN_MS)
    })
    return () => {
      clearTimeout(asked)
      window.removeEventListener('keydown', dismiss)
      noQuit()
    }
  })
</script>

<a class="skip-link" href="#main">Skip to content</a>

{#if router.under('/app')}
  <PrivateApp />
{/if}

{#if leaving}
  <!-- Over everything, the moment the key is heard. A click or Escape
       takes the question away, the same as waiting does. -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="leaving"
    role="status"
    onpointerdown={stayOpen}
  >
    <p>{leaving === 'ask' ? `Press ${quitKey} again to quit` : 'Quitting'}</p>
  </div>
{/if}

<Toast />

<style>
  /* Cmd+Q heard. The dim is the one a sheet over the app brings,
     Sheet.svelte, so the app looks the same whenever it is waiting on an
     answer. FrameFairy's is the dim of its box, and the same as --scrim
     in Dusk Fog. The words are the size of a box's title. */
  .leaving {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: grid;
    place-items: center;
    background: var(--scrim);
  }

  /* The words on a dark ground of their own, the way macOS shows the
     volume, in both themes, because the dim alone leaves them over
     whatever is under them. No line round it and nothing to press. */
  .leaving p {
    margin: 0;
    padding: 12px 20px;
    border-radius: var(--radius-m);
    background: var(--ink-0);
    color: var(--on-ink-0);
    font-size: var(--size-l);
    font-weight: 600;
  }
</style>
