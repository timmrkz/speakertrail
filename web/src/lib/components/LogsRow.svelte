<script lang="ts">
  import { Events } from '@wailsio/runtime'
  import { api, type LogStatus } from '../api'
  import { plural, size } from '../format'
  import { errorText, toast } from '../toast.svelte'
  import Confirm from './Confirm.svelte'
  import Icon from './Icon.svelte'

  // The app's own log, the file a report takes along, in an area of its
  // own, kept up to date as lines are written: while Settings is open the
  // app sends it as the event applog, about once a second at most. It is
  // the app's and nobody else's, so it can go whole, after asking. It
  // lists the last toasts about something that went wrong, so one that
  // ran out unread can still be read. Ported from FrameFairy's Settings,
  // Logs. See docs/LOGGING.md.
  let log = $state<LogStatus>({ path: '', files: 0, bytes: 0, lines: 0, toasts: [] })
  let removing = $state(false)
  const folder = $derived(log.path.replace(/[\\/][^\\/]*$/, ''))

  async function read() {
    try {
      log = await api.appLog()
    } catch (err) {
      toast.show(errorText(err), 'bad')
    }
  }

  async function remove() {
    removing = false
    try {
      log = await api.removeAppLog()
    } catch (err) {
      toast.show(errorText(err), 'bad')
      await read()
    }
  }

  async function show() {
    try {
      await api.showAppLog()
    } catch (err) {
      toast.show(errorText(err), 'bad')
    }
  }

  // When a toast came, the way Finder writes a date in its lists: the day
  // and the time.
  const when = (at: number) =>
    new Date(at).toLocaleString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false })

  $effect(() => {
    const off = Events.On('applog', (ev) => (log = ev.data as LogStatus))
    read()
    api.watchAppLog(true).catch(() => {})
    return () => {
      off()
      api.watchAppLog(false).catch(() => {})
    }
  })
</script>

<section class="group">
  <h2>Logs</h2>
  <div class="list">
    <div class="setting">
      <div class="info">
        {#if log.path}
          <span class="desc count">{plural(log.files, 'file')}, {size(log.bytes) || 'empty'}, {plural(log.lines, 'line')}</span>
          <span class="path small">{log.path}</span>
        {:else}
          <span class="desc">Nothing written yet</span>
        {/if}
      </div>
      <div class="actions">
        <button class="btn icon" type="button" disabled={!log.path} title="Show in Finder" aria-label="Show the app's log in Finder" onclick={show}>
          <Icon name="folder" />
        </button>
        <button class="btn icon danger" type="button" disabled={!log.path} title="Remove the log, the older files too"
          aria-label="Remove the app's log" aria-haspopup="dialog" onclick={() => (removing = true)}>
          <Icon name="trash" />
        </button>
      </div>
    </div>
    {#each log.toasts as t, i (i)}
      <!-- A toast the app showed about something that went wrong, in its
           words, for one that ran out unread. -->
      <div class="setting logged">
        <span class="mark" aria-hidden="true"><Icon name="alert" size={14} /></span>
        <div class="info">
          <span class="text">{t.text}</span>
          <span class="when small count">{when(t.at)}</span>
        </div>
      </div>
    {/each}
  </div>
</section>

{#if removing}
  <Confirm title="Remove the app's log?" oncancel={() => (removing = false)}>
    <p>
      Every file of the log in <b>{folder}</b> goes, the older ones too, and nothing else in that
      folder. The app starts a new log with its next line. This cannot be taken back.
    </p>
    {#snippet actions()}
      <button class="btn" type="button" onclick={() => (removing = false)}>Cancel</button>
      <button class="btn danger" type="button" onclick={remove}>Remove them</button>
    {/snippet}
  </Confirm>
{/if}

<style>
  .group { display: grid; gap: 10px; }
  .group h2 { font-size: 15px; margin: 0; }
  /* The same row as every other setting, see SettingRow. */
  .setting {
    display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 4px 24px; padding: 14px 16px; align-items: center;
  }
  .info { display: grid; gap: 2px; min-width: 0; }
  .desc { font-weight: 500; font-size: 14px; }
  .path, .when { color: var(--ink-3); overflow-wrap: anywhere; user-select: text; }
  .actions { display: flex; gap: 8px; }
  .count { font-variant-numeric: tabular-nums; }
  .logged { grid-template-columns: auto minmax(0, 1fr); gap: 4px 10px; align-items: start; }
  .mark { color: var(--bad); padding-top: 2px; }
  .text { font-size: 14px; overflow-wrap: anywhere; user-select: text; }
</style>
