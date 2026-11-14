<script lang="ts">
  // The app's frame, laid out the way FrameFairy lays out its own: the
  // title bar across the whole top with the close, minimise and zoom
  // buttons and the name of what is on screen, and under it the sidebar
  // and the page. The sidebar is a rail of icons until the pointer
  // reaches it, see lib/sidebar.svelte.ts. What you go to at the top,
  // what looks after the app at the foot: Settings, then Updates.
  import { navigate, router } from '../lib/router.svelte'
  import { runWatch } from '../lib/run.svelte'
  import { sidebar } from '../lib/sidebar.svelte'
  import { titleBar } from '../lib/chrome.svelte'
  import { railDot, standing, updates } from '../lib/updates.svelte'
  import Icon, { type IconName } from '../lib/components/Icon.svelte'
  import People from './People.svelte'
  import Settings from './Settings.svelte'
  import Searches from './Searches.svelte'
  import Updates from './Updates.svelte'
  import NotFound from './NotFound.svelte'

  type Item = { href: string; label: string; icon: IconName }
  const TOP: Item[] = [
    { href: '/app/people', label: 'People', icon: 'people' },
    { href: '/app/searches', label: 'Searches', icon: 'search' },
  ]

  // A going run shows as a pulsing dot on People, from the same watch
  // every screen uses.
  $effect(() => runWatch.watch())
  let going = $derived(runWatch.run !== null)

  // Where the app's updates stand, from the start, so Updates wears a dot
  // when a newer build is coming or ready, or something needs a look.
  $effect(() => updates.listen())

  // How tall the title bar macOS laid out is, and where its buttons are,
  // from the start, see lib/chrome.svelte.ts.
  $effect(() => titleBar.follow())
  let update = $derived(updates.shown)
  let updateDot = $derived(railDot(update))
  let updateTitle = $derived.by(() => {
    if (!update) return 'Updates'
    if (update.phase === 'ready') return `Updates. ${update.next || 'A newer build'} is ready, relaunch to finish updating`
    if (updateDot) {
      const s = standing(update, { working: '', now: Date.now() })
      return `Updates. This is ${update.version}. ${s.head}. ${s.more}`.trim()
    }
    return `Updates. This is ${update.version}`
  })

  // The workspace opens on People.
  $effect(() => {
    if (router.path === '/app' || router.path === '/app/') navigate('/app/people', { replace: true })
  })

  // The name of what is on screen, in the title bar.
  let title = $derived(
    router.under('/app/people') ? 'People' : router.under('/app/searches') ? 'Searches'
      : router.path === '/app/settings' ? 'Settings' : router.path === '/app/updates' ? 'Updates' : 'Speaker Trail',
  )

  const current = (href: string) => router.under(href)
</script>

<svelte:window
  onpointermove={(e) => sidebar.pointerMoved(e)}
  onkeydowncapture={(e) => sidebar.escape(e)}
  onblur={() => sidebar.away()} />

<div class="shell">
  <!-- The title bar: the close, minimise and zoom buttons, and the name of
       what is on screen beside them. It is the whole width of the app, so
       the sidebar opens under it and never covers the name. -->
  <!-- The bar is the title bar, the way FrameFairy's is. macOS lays it out
       and puts its three buttons on its middle, and the bar takes the
       height macOS gave it rather than asking for one. Then macOS centres
       the buttons in the bar, and the name is centred between the edge of
       the window macOS draws over its top and the line at its foot, see
       .name below. Where the system draws its own title bar there is
       nothing to agree with and the stylesheet keeps its own numbers. -->
  <header class="titlebar" style={titleBar.style}>
    <h1 class="name"><span class="what">{title}</span></h1>
  </header>

  <div class="body" class:kept={sidebar.pinned}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <aside
      bind:this={sidebar.aside}
      class:open={sidebar.open}
      aria-label="Sidebar"
      onpointerleave={(e) => sidebar.pointerLeft(e)}
      onfocusin={(e) => sidebar.focusIn(e)}
      onfocusout={(e) => sidebar.focusOut(e)}>
      <div class="head">
        <button class="glyph" type="button" onclick={() => sidebar.toggle()} aria-pressed={sidebar.pinned}
          title={sidebar.pinned ? 'Close the sidebar' : 'Keep the sidebar open'}>
          <Icon name={sidebar.pinned ? 'sidebar-close' : 'sidebar-keep'} />
        </button>
      </div>
      <nav class="rows" aria-label="Main">
        {#each TOP as item (item.href)}
          <a class="nav" href={item.href} aria-current={current(item.href) ? 'page' : undefined} title={item.label}>
            <span class="mark"><Icon name={item.icon} />{#if item.href === '/app/people' && going}<span class="dot busy"></span>{/if}</span>
            <span class="word">{item.label}</span>
          </a>
        {/each}
      </nav>
      <div class="foot">
        <a class="nav" href="/app/settings" aria-current={current('/app/settings') ? 'page' : undefined} title="Settings">
          <span class="mark"><Icon name="settings" /></span>
          <span class="word">Settings</span>
        </a>
        <!-- The build that is running, which is also the way to its
             updates, so a report from testing always names what was tested. -->
        <a class="nav" href="/app/updates" aria-current={current('/app/updates') ? 'page' : undefined} title={updateTitle}>
          <span class="mark"><Icon name="update" />{#if updateDot}<span class="dot {updateDot}"></span>{/if}</span>
          <span class="word">Updates</span>
          <span class="word build">{update?.version ?? ''}</span>
        </a>
      </div>
    </aside>

    <main id="main" class="main">
      {#if router.under('/app/people')}
        <People />
      {:else if router.under('/app/searches')}
        <Searches />
      {:else if router.path === '/app/settings'}
        <Settings />
      {:else if router.path === '/app/updates'}
        <Updates />
      {:else}
        <NotFound embedded />
      {/if}
    </main>
  </div>
</div>

<style>
  /* The mist lies behind the whole window, and every pane over it is
     frosted, see --mist and --frost in app.css. */
  .shell {
    display: flex; flex-direction: column; height: 100dvh;
    background: var(--mist), var(--bg); background-size: 100% 100%;
  }

  /* The title bar: what the app is dragged by, and where macOS puts its
     three buttons. Exactly as tall as the title bar macOS laid out, which
     the bar's style sets from what macOS answers, or the token where the
     system draws its own bar. The line at the foot is drawn inside that
     height rather than under it, so the bar is the title bar and nothing
     else. Its own style carries the measurement, not the shell's as in
     FrameFairy, because here a sheet's bar is --bar-h tall too, and it
     keeps the token. */
  .titlebar {
    flex: none; position: relative; height: var(--bar-h); background: var(--side);
    backdrop-filter: var(--frost); -webkit-backdrop-filter: var(--frost);
    box-shadow: inset 0 -1px var(--line); --wails-draggable: drag;
  }

  /* The name stands in a box as tall as the bar, so it is centred on the
     bar's middle, and starts one space after the buttons. Nothing is
     moved by a transform, so nothing is put on a compositing surface of
     its own and the whole pixel rule holds. */
  .titlebar .name {
    position: absolute; top: 0; height: var(--bar-h);
    left: calc(var(--lights-r, 0px) + var(--gap)); right: var(--gap);
    display: flex; align-items: center; overflow: hidden;
  }

  /* The name itself, as FrameFairy has it. A line box keeps more room
     below the baseline than above the capitals, so a name centred by its
     line box reads high. Trimming the box to the capitals and the
     baseline puts the letters themselves on the middle. The box is also
     what is clipped, for the ellipsis, so it is padded back out evenly,
     deeper than any letter goes, and nothing is cut. */
  .titlebar .what {
    min-width: 0; font-size: 13px; font-weight: 600; line-height: 1;
    text-box: trim-both cap alphabetic; padding: 6px 0;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }

  /* Where the browser cannot trim a text box there is nothing to pad back
     out, and a plain line box centred on the buttons is a small fraction
     high rather than wrong. */
  @supports not (text-box: trim-both cap alphabetic) {
    .titlebar .what { line-height: normal; padding: 0; }
  }

  /* Where macOS puts its buttons in the bar, it draws the edge of the
     window over the top 2 points of the page, and the line at the foot
     takes the last point. What a person sees as the bar is what lies
     between the two, and the name is centred in that. FrameFairy read it
     off Tim's pictures, its #399. */
  .titlebar[style*="--lights-r"] .name { top: 2px; height: calc(var(--bar-h) - 3px); }

  /* Where the system draws its own title bar, and in fullscreen, the name
     keeps the left of the bar, because there is nothing at the left to
     make room for. */
  .titlebar:not([style*="--lights-r"]) .name { left: var(--gap); }

  /* The rail keeps its own column, so nothing of the page ever hides under
     it. The sidebar grows over the page from there. Kept open, it takes
     its whole width as a column instead, so the list beside it is never
     covered for good. */
  .body { position: relative; display: grid; grid-template-columns: var(--rail) minmax(0, 1fr); flex: 1; min-height: 0; }
  @media (min-width: 761px) {
    .body.kept { grid-template-columns: var(--side-w) minmax(0, 1fr); }
  }

  aside {
    position: absolute; top: 0; bottom: 0; left: 0; z-index: 20; width: var(--rail);
    display: flex; flex-direction: column; background: var(--side);
    backdrop-filter: var(--frost); -webkit-backdrop-filter: var(--frost);
    /* The line at the edge is drawn inside rather than as a border, so the
       rail is 44 px of room for the icons and not 43, and an icon sits on
       whole pixels in the middle of it. */
    box-shadow: inset -1px 0 var(--line); overflow: hidden; transition: width .14s ease;
  }
  aside.open {
    width: var(--side-w);
    box-shadow: inset -1px 0 var(--line), 8px 0 24px rgba(20, 27, 38, .16);
    /* The shadow falls on the page to the right and nowhere else, not up
       over the title bar. */
    clip-path: inset(0 -40px 0 0);
  }
  .kept aside.open { box-shadow: inset -1px 0 var(--line); }

  /* What there is no room for on the rail waits until the sidebar opens.
     An icon on the rail is exactly where its row is when the sidebar
     opens, so a row keeps its height and only its words go. */
  aside:not(.open) .word { display: none; }
  /* The sidebar slides open, so while it moves nothing in it may reflow:
     every line stays one line, and what does not fit yet is clipped. */
  .word { white-space: nowrap; min-width: 0; overflow: hidden; }

  /* 4 px each side of a 36 px box leaves the icon in the middle of the
     rail, and the same box holds every row. */
  .head { flex: none; padding: 8px 4px; }
  .glyph {
    display: flex; align-items: center; justify-content: center; width: 36px; height: 36px; padding: 0;
    border: 0; border-radius: 8px; background: none; color: var(--ink-3); cursor: pointer;
  }
  .glyph:hover { color: var(--ink); background: color-mix(in srgb, var(--ink) 6%, transparent); }
  .rows { display: grid; gap: 2px; padding: 0 4px; flex: 1; min-height: 0; align-content: start; }
  .foot { border-top: 1px solid var(--line); padding: 8px 4px; display: grid; gap: 2px; }

  /* 10 px in puts a 16 px icon in the middle of the 44 px rail, the same
     column as the button at the head. */
  .nav {
    display: flex; align-items: center; gap: 10px; height: 36px; padding: 0 10px; border-radius: 8px;
    color: var(--ink-2); font-size: 14px; font-weight: 500; text-decoration: none;
  }
  .nav:hover { background: color-mix(in srgb, var(--ink) 6%, transparent); color: var(--ink); text-decoration: none; }
  .nav[aria-current="page"] { background: var(--accent-soft); color: var(--accent); font-weight: 600; backdrop-filter: var(--frost); -webkit-backdrop-filter: var(--frost); box-shadow: inset 0 1px var(--glass-hi), 0 1px 3px rgba(20, 27, 38, .12); }
  .nav .word { flex: 1; }
  /* The build stands at the far end of its row, whole, since it names
     what is being tested. */
  .nav .build { flex: none; margin-left: auto; font-size: 12px; color: var(--ink-3); font-weight: 500; font-variant-numeric: tabular-nums; }
  .mark { position: relative; display: flex; flex: none; }
  /* A dot in the corner of an icon, whatever its colour. */
  .mark .dot { position: absolute; top: -2px; right: -3px; width: 7px; height: 7px; }

  /* The page is out of the sidebar's way, in the second column, and
     scrolls by itself. */
  .main { grid-column: 2; position: relative; min-width: 0; min-height: 0; overflow-y: auto; overscroll-behavior: contain; }

  @media (prefers-reduced-motion: reduce) { aside { transition: none; } }
</style>
