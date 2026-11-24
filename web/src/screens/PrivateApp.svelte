<script lang="ts">
  // The app's frame, laid out the way FrameFairy lays out its own: the
  // title bar across the whole top with the close, minimise and zoom
  // buttons and the name of what is on screen, and under it the sidebar
  // and the page. The sidebar is a rail of icons until the pointer
  // reaches it, see lib/sidebar.svelte.ts. What you go to at the top,
  // what looks after the app at the foot: Settings, Bugs, then Updates.
  import { navigate, router } from '../lib/router.svelte'
  import { runWatch } from '../lib/run.svelte'
  import { sidebar } from '../lib/sidebar.svelte'
  import { titleBar } from '../lib/chrome.svelte'
  import { exaWatch } from '../lib/exa.svelte'
  import { languageWatch } from '../lib/language.svelte'
  import { railDot, standing, updates } from '../lib/updates.svelte'
  import { reporting } from '../lib/reporting.svelte'
  import { api, onUndo } from '../lib/api'
  import { undoHistory } from '../lib/history.svelte'
  import { typing } from '../lib/keys'
  import Icon, { type IconName } from '../lib/components/Icon.svelte'
  import Landing from '../lib/components/Landing.svelte'
  import TitleBar from '../lib/components/TitleBar.svelte'
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

  // Settings wears a dot when something there stands in the way, the way
  // Updates does: the Exa key every run needs, and the language model
  // every draft needs, Tim asked on 21 November 2026. A model coming
  // pulses, as work in hand does everywhere. Draft without a model sends
  // here, so the way there is marked too.
  $effect(() => languageWatch.watch())
  $effect(() => {
    exaWatch.read().catch(() => {})
  })
  // What stands in the way of a draft is the app's to judge, see
  // LanguageStanding in languagemodels.go. Here it is only drawn.
  let settingsNeed = $derived.by((): { dot: string; why: string } => {
    const job = languageWatch.state.job
    if (job && (job.state === 'running' || job.state === 'queued')) return { dot: 'busy', why: 'The language model is downloading' }
    if (exaWatch.loaded && !exaWatch.exa?.set) return { dot: 'err', why: 'Runs need the Exa key' }
    const st = languageWatch.loaded ? languageWatch.state.standing : undefined
    if (st && st.mark !== 'ok') return { dot: st.mark, why: st.why }
    return { dot: '', why: '' }
  })

  // Where the app's updates stand, from the start, so Updates wears a dot
  // when a newer build is coming or ready, or something needs a look.
  $effect(() => updates.listen())

  // How tall the title bar macOS laid out is, and where its buttons are,
  // from the start, see lib/chrome.svelte.ts.
  $effect(() => titleBar.follow())

  // Bugs is lit while the report window is open, see bugs.go.
  $effect(() => reporting.listen())

  // Undo and Redo in the Edit menu come to the page, see menu.go. A
  // field being typed in undoes its own text, as the stock ones did.
  // Anywhere else they take back the last change Tim made, kept by the
  // app, see lib/history.svelte.ts.
  $effect(() => onUndo((how) => (typing() ? document.execCommand(how) : void undoHistory.step(how))))
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

  // Links dragged from Chrome land anywhere on the window, lit the way
  // Report a Problem lights for a picture, and add the people behind
  // them, see runWatch.add. A drag that starts on the page, like words
  // picked out of a profile, is the page's own and lands nowhere. The
  // window has no file drops of its own, so the page gets the link.
  let ours = false
  let held = $state(0)
  const carriesLink = (e: DragEvent) => !ours && !!e.dataTransfer && [...e.dataTransfer.types].some((t) => t === 'text/uri-list' || t === 'text/plain')
  function dragEnter(e: DragEvent) {
    if (!carriesLink(e)) return
    held++
  }
  function dragOver(e: DragEvent) {
    if (!carriesLink(e)) return
    e.preventDefault()
    e.dataTransfer!.dropEffect = 'copy'
  }
  function drop(e: DragEvent) {
    if (!carriesLink(e)) return
    e.preventDefault()
    held = 0
    const text = e.dataTransfer!.getData('text/uri-list') || e.dataTransfer!.getData('text/plain')
    if (text.trim()) void runWatch.add(text)
  }
</script>

<svelte:window
  ondragstart={() => (ours = true)}
  ondragend={() => ((ours = false), (held = 0))}
  ondragenter={dragEnter}
  ondragleave={(e) => { if (carriesLink(e)) held = Math.max(0, held - 1) }}
  ondragover={dragOver}
  ondrop={drop}
  onpointermove={(e) => sidebar.pointerMoved(e)}
  onkeydowncapture={(e) => sidebar.escape(e)}
  onblur={() => sidebar.away()} />

<div class="shell">
  <TitleBar {title} />
  <Landing lit={held > 0} />

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
        <a class="nav" href="/app/settings" aria-current={current('/app/settings') ? 'page' : undefined}
          title={settingsNeed.why ? `Settings. ${settingsNeed.why}` : 'Settings'}>
          <span class="mark"><Icon name="settings" />{#if settingsNeed.dot}<span class="dot {settingsNeed.dot}"></span>{/if}</span>
          <span class="word">Settings</span>
        </a>
        <!-- Report a Problem, in a window of its own, one at a time, so a
             person can talk about a problem while going through it here.
             Lit while that window is open, the way FrameFairy has it. -->
        <button class="nav" type="button" aria-pressed={reporting.open} onclick={() => api.openReport('bugs').catch(() => {})} title="Report a problem">
          <span class="mark"><Icon name="report" /></span>
          <span class="word">Bugs</span>
        </button>
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
  /* The page's solid ground behind the whole window, see app.css. */
  .shell { display: flex; flex-direction: column; height: 100dvh; background: var(--bg); }

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
    /* The line at the edge is drawn inside rather than as a border, so the
       rail is 44 px of room for the icons and not 43, and an icon sits on
       whole pixels in the middle of it. */
    box-shadow: inset -1px 0 var(--line); overflow: hidden; transition: width .14s ease;
  }
  /* Open over the page it casts no shadow: its solid ground and the line
     at its edge set it apart, the look is flat, issue #45. */
  aside.open { width: var(--side-w); }

  /* What there is no room for on the rail waits until the sidebar opens.
     An icon on the rail is exactly where its row is when the sidebar
     opens, so a row keeps its height and only its words go. */
  aside:not(.open) .word { display: none; }
  /* The sidebar slides open, so while it moves nothing in it may reflow:
     every line stays one line, and what does not fit yet is clipped. */
  .word { white-space: nowrap; min-width: 0; overflow: hidden; }

  /* 4 px each side of a 36 px box leaves the icon in the middle of the
     rail, and the same box holds every row. The same 4 px, --gap-inset,
     stands between two rows, so the page under the pointer never touches
     the page chosen (Tim, 21 November 2026). Only the foot keeps 8 px
     below its last row, clear of the window's round corner. */
  .head { flex: none; padding: var(--gap-inset); }
  .glyph {
    display: flex; align-items: center; justify-content: center; width: 36px; height: 36px; padding: 0;
    border: 0; border-radius: 8px; background: none; color: var(--ink-3); cursor: pointer;
  }
  .glyph:hover { color: var(--ink); background: var(--hover); }
  .glyph:active { background: var(--press); }
  .rows { display: grid; gap: var(--gap-inset); padding: 0 var(--gap-inset); flex: 1; min-height: 0; align-content: start; }
  .foot { border-top: 1px solid var(--line); padding: var(--gap-inset) var(--gap-inset) 8px; display: grid; gap: var(--gap-inset); }

  /* 10 px in puts a 16 px icon in the middle of the 44 px rail, the same
     column as the button at the head. */
  .nav {
    display: flex; align-items: center; gap: 10px; height: 36px; padding: 0 10px; border-radius: 8px;
    color: var(--ink-2); font-size: 14px; font-weight: 500; text-decoration: none;
  }
  /* Bugs is a button, since it opens a window rather than a page, and
     looks like the links beside it. */
  button.nav { width: 100%; border: 0; background: none; font-family: inherit; text-align: left; cursor: pointer; }
  .nav:hover { background: var(--hover); color: var(--ink); text-decoration: none; }
  .nav:active { background: var(--press); }
  /* The page open, and Bugs while its window is, lie flat in the tint
     every chosen thing wears. Its weight stays, so the word never moves. */
  .nav[aria-current="page"], .nav[aria-pressed="true"] { background: var(--chosen); color: var(--chosen-ink); }
  .nav[aria-current="page"]:hover, .nav[aria-pressed="true"]:hover { background: var(--chosen-hover); color: var(--chosen-ink); }
  .nav[aria-current="page"]:active, .nav[aria-pressed="true"]:active { background: var(--chosen-press); }
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
