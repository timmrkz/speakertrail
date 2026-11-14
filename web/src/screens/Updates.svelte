<script lang="ts">
  // Which build of the app this is, and what is happening about the next
  // one: the build and the channel it follows on top, and under it one
  // line that says where things stand, with the one thing to do about it
  // at its end. A newer build downloads by itself, so that thing is
  // Relaunch once it is here, and Check the rest of the time a channel is
  // followed. A build made on the Mac follows nothing until a channel is
  // chosen. Ported from FrameFairy's Updates page. See docs/UPDATES.md.
  import { api } from '../lib/api'
  import { runWatch } from '../lib/run.svelte'
  import { errorText } from '../lib/toast.svelte'
  import Busy from '../lib/components/Busy.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Skeleton from '../lib/components/Skeleton.svelte'
  import { channelName, conflicted as conflictedIn, following as followingIn, standing as standingOf, updates } from '../lib/updates.svelte'

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
    updates.heard({ ...update, picked: channel, gone: '', building: '', next: '', phase: 'checking' })
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

  // What the system said copies with one click, for a report.
  let copied = $state(false)
  async function copyDetail(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      copied = true
      setTimeout(() => (copied = false), 1500)
    } catch {
      problem = 'Copying was refused. Select the text and copy it instead.'
    }
  }

  const following = $derived(followingIn(update))
  const conflicted = $derived(conflictedIn(update))
  const options = $derived.by(() => {
    const listed = (update?.channels ?? []).map((c) => ({
      value: c.id,
      label: c.conflict ? `${channelName(c.id)}, conflicts · ${c.name}` : c.id === 'main' ? 'main' : `${channelName(c.id)} · ${c.name.replace(/^#\d+\s*/, '')}`,
    }))
    // A pull request followed that has gone stays in the list while it is
    // followed, so the list never names nothing and says what became of it.
    if (following && !listed.some((o) => o.value === following)) {
      listed.push({ value: following, label: update?.gone === following ? `${channelName(following)}, closed` : channelName(following) })
    }
    return listed
  })

  // A run going, which Relaunch waits for: quitting into the new build
  // would stop it half way. The app refuses a Relaunch pressed all the
  // same.
  const working = $derived(runWatch.run ? 'the run' : '')
  const standing = $derived(standingOf(update, { working, now }))
  const detail = $derived(update?.phase === 'failed' ? update.detail : '')
  const looking = $derived(update?.phase === 'checking')
  const downloading = $derived(update?.phase === 'downloading')
  const short = (commit: string) => commit.slice(0, 7)
</script>

<div class="page">
  <header class="bar">
    <h1 title="This build, and where the next one comes from.">Updates</h1>
  </header>
  <div class="page-body">

  {#if problem}<p class="field-error" role="alert">{problem}</p>{/if}

  {#if !update}
    <Skeleton count={2} />
  {:else}
    <div class="list">
      <div class="item build">
        <div class="words">
          <span class="version">{update.version}</span>
          <span class="faint small">
            {#if update.commit}Commit <button class="link" type="button" title="Opens this commit on GitHub"
                onclick={() => api.openCommit().catch((e) => (problem = errorText(e)))}>{short(update.commit)}</button>
            {:else}Built on this Mac{/if}
          </span>
        </div>
        {#if !update.off}
          <select class="select channel" class:warn={update.phase === 'gone' || conflicted} aria-label="Follow"
            title="Where this app updates from: main, or one pull request. Every push to it makes a new build, which downloads by itself"
            value={following} disabled={options.length === 0} onchange={(e) => follow(e.currentTarget.value)}>
            {#if !following}<option value="" disabled>{options.length ? 'Choose what to follow' : 'No builds to follow yet'}</option>{/if}
            {#each options as o (o.value)}
              <option value={o.value}>{o.label}</option>
            {/each}
          </select>
        {/if}
      </div>

      <div class="item status" aria-live="polite">
        <span class="mark {standing.mark}" aria-hidden="true">
          {#if standing.mark === 'ok'}
            <Icon name="check" />
          {:else if standing.mark === 'warn' || standing.mark === 'err'}
            <Icon name="alert" />
          {:else if standing.mark !== 'off' && standing.mark !== 'idle'}
            <span class="dot" class:busy={standing.mark === 'look'} class:ready={standing.mark === 'new'}
              class:warn={conflicted && (standing.mark === 'look' || standing.mark === 'new')}></span>
          {/if}
        </span>
        <div class="words">
          <b>{standing.head}</b>
          <span class="faint small">{standing.more}</span>
        </div>
        {#if update.phase === 'ready'}
          <button class="btn primary act" type="button" onclick={relaunch} disabled={restarting || !!working}
            title={working ? `Waits until ${working} is done` : 'Quits and comes back as the new build'}>
            {restarting ? 'Relaunching' : 'Relaunch'}
          </button>
        {:else if !update.off && following}
          <button class="btn act" type="button" onclick={check} disabled={looking || downloading}
            title="Look for a newer build now">
            {#if looking || downloading}<Busy fraction={downloading && update.total ? update.written / update.total : -1} />{/if}
            {downloading ? 'Downloading' : looking ? 'Checking' : update.phase === 'failed' ? 'Try again' : 'Check'}
          </button>
        {/if}
      </div>

      {#if detail}
        <div class="item said">
          <code title="What the system said">{detail}</code>
          <button class="btn quiet icon" type="button" title={copied ? 'Copied' : 'Copy what the system said, for a report'}
            aria-label={copied ? 'Copied' : 'Copy what the system said'} onclick={() => copyDetail(detail)}>
            <Icon name={copied ? 'check' : 'doc'} size={16} />
          </button>
        </div>
      {/if}
    </div>
  {/if}
</div>
</div>

<style>
  .item { display: flex; align-items: center; gap: 12px; padding: 14px 16px; min-width: 0; }
  .words { display: grid; gap: 3px; min-width: 0; flex: 1; }
  .build { flex-wrap: wrap; }
  .build .words { flex: 1 1 12em; }
  .version { font-size: 19px; font-weight: 600; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
  .link { border: 0; padding: 0; background: none; color: var(--accent); cursor: pointer; font: inherit; font-variant-numeric: tabular-nums; }
  .link:hover { text-decoration: underline; }
  /* As wide as the longest name it needs, never wider than the row. */
  .channel { width: auto; max-width: 100%; flex: 0 1 auto; min-width: 12em; }
  .channel.warn { border-color: var(--warn); color: var(--warn); }
  .mark { display: inline-grid; place-items: center; width: 20px; height: 20px; flex: none; align-self: start; margin-top: 1px; }
  .mark.ok { color: var(--good); }
  .mark.warn { color: var(--warn); }
  .mark.err { color: var(--bad); }
  /* Room for the longest word, so the button keeps its width. */
  .act { flex: none; min-width: 7.6em; justify-content: center; }
  .progress { margin-top: 6px; }
  .said { padding-top: 10px; padding-bottom: 10px; }
  .said code {
    flex: 1; min-width: 0; overflow-x: auto; white-space: pre; font-size: 12px; color: var(--ink-2);
    font-family: ui-monospace, "SF Mono", Menlo, monospace; user-select: text; scrollbar-width: none;
  }
  @media (max-width: 760px) {
    .channel { flex: 1 1 100%; }
  }
</style>
