<script lang="ts">
  // Which build of the app this is, and what is happening about the next
  // one, in one list: the build and the channel it follows on top, and
  // under it one line that says where things stand, with the one thing to
  // do about it at its end. A newer build downloads by itself, so that
  // thing is Relaunch once it is here, and Check the rest of the time a
  // channel is followed. A build made on the Mac follows nothing until a
  // channel is chosen, and with nothing to look for there is no Check.
  // FrameFairy's Updates page, ported as it is. See docs/UPDATES.md.
  import { api } from '../lib/api'
  import { runWatch } from '../lib/run.svelte'
  import { errorText } from '../lib/toast.svelte'
  import Busy from '../lib/components/Busy.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Info from '../lib/components/Info.svelte'
  import Pick from '../lib/components/Pick.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import { channelName, conflicted as conflictedIn, ended as endedIn, following as followingIn, standing as standingOf, updates } from '../lib/updates.svelte'

  const update = $derived(updates.shown)
  let restarting = $state(false)
  let problem = $state('')
  // Now, a few times a minute, so "just now" becomes a time by itself.
  let now = $state(Date.now())

  $effect(() => {
    document.title = 'Updates · Speaker Trail'
  })

  // What the app says comes as it changes, see updates.svelte.ts. The
  // run is watched so Relaunch knows to wait for it.
  $effect(() => runWatch.watch())
  // The workspace listens from its start, see PrivateApp.svelte. Opening
  // the page asks again, so what it shows is never older than the opening.
  $effect(() => {
    updates.ask()
    const tick = setInterval(() => (now = Date.now()), 15_000)
    return () => clearInterval(tick)
  })

  // A click shows at once. The app's own word that it is looking follows
  // a moment later and changes nothing.
  async function check() {
    if (!update) return
    problem = ''
    updates.heard({ ...update, phase: 'checking' })
    try {
      await api.checkForUpdates()
    } catch (e) {
      problem = errorText(e)
    }
  }

  // A pick shows at once too: the channel in the list and looking under
  // it, before the app has said a word. Should it say no, what it says
  // is true is asked for again.
  async function follow(channel: string) {
    if (!update || !channel) return
    problem = ''
    updates.heard({ ...update, picked: channel, gone: '', ended: '', building: '', next: '', phase: 'checking' })
    try {
      await api.followChannel(channel)
    } catch (e) {
      problem = errorText(e)
      updates.ask()
    }
  }

  async function relaunch() {
    problem = ''
    restarting = true
    try {
      await api.relaunch()
    } catch (e) {
      restarting = false
      problem = errorText(e)
    }
  }

  // What the system said copies with one click, for a report. The button
  // says it copied in the same frame as the click, and goes back a moment
  // later.
  let copied = $state(false)
  let copyRefused = $state('')
  let copiedTimer: ReturnType<typeof setTimeout> | undefined
  $effect(() => () => clearTimeout(copiedTimer))
  async function copyDetail(text: string) {
    copied = true
    copyRefused = ''
    clearTimeout(copiedTimer)
    copiedTimer = setTimeout(() => (copied = false), 1500)
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      copied = false
      copyRefused = 'Copying was refused. Select the text and copy it instead.'
    }
  }

  const following = $derived(followingIn(update))
  const conflicted = $derived(conflictedIn(update))
  const ended = $derived(endedIn(update))
  const options = $derived.by(() => {
    // A channel is named by what it is, a branch or a pull request by its
    // number, never by the pull request's title, which is not what is
    // picked here and made the list as wide as the window. A pull request
    // that no longer merges into main says so in its name, the way a
    // merged or closed one does.
    const listed = (update?.channels ?? []).map((c) => ({
      value: c.id,
      label: c.conflict ? `${channelName(c.id)}, conflicts` : channelName(c.id),
    }))
    // A pull request followed that has gone stays in the list while it is
    // followed, so the list never names nothing and says what became of
    // it: merged, closed or not open, only when a list that was read says it
    // has gone.
    if (following && !listed.some((o) => o.value === following)) {
      listed.push({ value: following, label: update?.gone === following ? `${channelName(following)}, ${ended.word}` : channelName(following) })
    }
    return listed
  })

  // A run going, which Relaunch waits for: quitting into the new build
  // would stop it half way. The app refuses a Relaunch pressed all the
  // same.
  const working = $derived(runWatch.run ? 'the run' : '')
  const standing = $derived(standingOf(update, { working, now }))
  // What the system said when a check did not get through.
  const detail = $derived(update?.phase === 'failed' ? update.detail : '')
  const looking = $derived(update?.phase === 'checking')
  const downloading = $derived(update?.phase === 'downloading')
  const short = (commit: string) => commit.slice(0, 7)
</script>

<div class="page">
  <div class="page-body">

  {#if problem}<p class="field-error" role="alert">{problem}</p>{/if}

  {#if !update}
    <Skeleton count={2} />
  {:else}
    <div class="list">
      <!-- The build, and where the next one comes from. -->
      <div class="item build">
        <span class="mark" aria-hidden="true"><Icon name="update" size={24} /></span>
        <div class="words">
          <span class="version">{update.version}</span>
          <span class="faint small">
            {#if update.commit}Commit <button class="link" type="button" title="Opens this commit on GitHub"
                onclick={() => api.openCommit().catch((e) => (problem = errorText(e)))}>{short(update.commit)}</button>
            {:else}Built on this Mac{/if}
          </span>
        </div>
        <!-- How updates work is behind the mark beside the list, a word in
             front of it said nothing the list does not say. -->
        {#if !update.off}
          <span class="channel">
            <span class="ask">
              <Info label="How updates work" side="right">
                The list says where this app updates from: main, or one pull request. Every push to it makes a new build. The app looks
                when it starts, when you choose a channel and every ten minutes, every twenty seconds while a newer commit is being built,
                and downloads the newest build by itself. <b>Relaunch</b> restarts the app into it, and if you quit instead, it goes in on
                the way out. A build made on this Mac follows nothing until you choose a channel. The commit under the version opens on
                GitHub.
              </Info>
            </span>
            <Pick label="Channel" id="channel" value={following} {options} align="right" placeholder="Choose a channel"
              tone={update.phase === 'gone' ? ended.tone : conflicted ? 'warn' : undefined} disabled={options.length === 0}
              title="Where this app updates from: main, or one pull request. It downloads the newest build by itself"
              onpick={(v) => follow(v)} />
          </span>
        {/if}
      </div>

      <!-- Where things stand, and the one thing to do about it. -->
      {#snippet act()}
        {#if update.phase === 'ready'}
          <button class="btn primary act" type="button" onclick={relaunch} disabled={restarting || !!working}
            title={working ? `Waits until ${working} is done` : 'Quits and comes back as the new build'}>
            {#if restarting}<Busy />{/if}{restarting ? 'Relaunching' : 'Relaunch'}
          </button>
        {:else if !update.off && following}
          <button class="btn act" type="button" onclick={check} disabled={looking || downloading}
            title="Look for a newer build of the channel now">
            {#if downloading}<Busy fraction={update.total > 0 ? update.written / update.total : -1} />{:else if looking}<Busy />{/if}
            {downloading ? 'Downloading' : looking ? 'Checking' : update.phase === 'failed' ? 'Try again' : 'Check'}
          </button>
        {/if}
      {/snippet}
      <div class="item status" class:told={!!detail} aria-live="polite">
        <!-- A dot is work, running or waiting to be picked up, and grey
             while nothing happens. Something that needs a look is the
             triangle. Work on a pull request in conflict keeps its dot, in
             the colour of a warning. -->
        <span class="mark {standing.mark}" aria-hidden="true">
          {#if standing.mark === 'ok'}
            <Icon name="check" />
          {:else if standing.mark === 'warn' || standing.mark === 'err'}
            <Icon name="alert" />
          {:else}
            <span class="dot" class:busy={standing.mark === 'look'} class:ready={standing.mark === 'new'}
              class:warn={conflicted && (standing.mark === 'look' || standing.mark === 'new')}></span>
          {/if}
        </span>
        <div class="words">
          <b>{standing.head}</b>
          <span class="faint small">{standing.more}</span>
        </div>
        {#if detail}
          <!-- What the system said, apart from the words above it, on one
               line that scrolls sideways, so a long address never wraps
               the row into a wall of text, and Try again on the same row
               as it, the way a field and its button sit in the settings. -->
          <div class="said">
            <div class="detail">
              <code title="What the system said">{detail}</code>
              <button class="btn quiet copy" class:copied type="button" title={copied ? 'Copied' : 'Copy what the system said, for a report'}
                aria-label={copied ? 'Copied' : 'Copy what the system said'} onclick={() => copyDetail(detail)}>
                <Icon name={copied ? 'check' : 'copy'} size={14} />
              </button>
            </div>
            {@render act()}
          </div>
          {#if copyRefused}<span class="small field-error refused">{copyRefused}</span>{/if}
        {:else}
          {@render act()}
        {/if}
      </div>
    </div>
  {/if}
</div>
</div>

<style>
  .item { display: flex; align-items: center; gap: 12px; padding: 14px 16px; min-height: 64px; box-sizing: border-box; min-width: 0; }
  .words { display: grid; gap: 3px; min-width: 0; flex: 1; }
  /* The build is the head of the page, so its version is set larger. */
  .version { font-size: 19px; font-weight: 600; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
  /* The words take the room they need. In a window too narrow for the
     version and the list on one line, the mark and the list go under it,
     still at the right. */
  .build { flex-wrap: wrap; row-gap: 8px; }
  .build .words { flex: 0 1 auto; }
  /* The list is as wide as the channel it names and no wider, up to
     --pick-max, see Pick. It hangs from its right edge. */
  .channel { display: flex; align-items: center; gap: 12px; flex: none; margin-left: auto; }
  .link { border: 0; padding: 0; background: none; color: var(--accent); cursor: pointer; font: inherit; font-variant-numeric: tabular-nums; }
  .link:hover { text-decoration: underline; }
  .ask { display: flex; align-items: center; height: 20px; flex: none; }
  /* The mark before the words, as wide as the biggest icon a row carries,
     so the words of both rows start in one column. */
  .mark { display: flex; align-items: center; justify-content: center; width: 24px; flex: none; }
  .mark.ok { color: var(--good); }
  .mark.warn { color: var(--warn); }
  .mark.err { color: var(--bad); }
  /* As wide as what it says now, as in FrameFairy. */
  .act { flex: none; }

  /* A row that says what the system said wraps: the mark and the words on
     its first line, as in every other row, and under them the field and
     Try again on a line of their own, starting where the words start,
     past the mark's 24 px and the row's 12 px gap. */
  .item.told { flex-wrap: wrap; row-gap: 12px; }
  .told > .said,
  .told > .refused { flex-basis: calc(100% - 24px - 12px); margin-left: calc(24px + 12px); }
  .said { display: flex; align-items: center; gap: var(--gap-control); min-width: 0; }

  /* What the system said: a field at the height of every control, its
     words at one width the way code is, on one line that scrolls, and the
     button that copies them at its end. */
  .detail {
    display: flex; align-items: center; flex: 1; height: var(--control-h); min-width: 0; box-sizing: border-box;
    border: var(--line-w) solid var(--line-strong); border-radius: var(--radius-m); background: var(--surface);
  }
  .detail code {
    flex: 1; min-width: 0; overflow-x: auto; white-space: pre; padding: 0 12px; user-select: text;
    font-family: var(--mono); font-size: 12px; line-height: calc(var(--control-h) - 2px); color: var(--ink-2);
    /* No scroll bar: WebKit draws one inside the line and pushes the words
       up. It scrolls with the trackpad, and Copy takes the whole of it. */
    scrollbar-width: none;
  }
  .detail code::-webkit-scrollbar { display: none; }
  /* Square, inside the field's own line. */
  .detail .copy { flex: none; width: calc(var(--control-h) - 2px); height: calc(var(--control-h) - 2px); padding: 0; }
  .detail .copy.copied { color: var(--good); }
</style>
