<script lang="ts">
  // The first message to a person: Draft has the language model write it
  // on this Mac, in the background, and it grows here as it is written.
  // Then it is Tim's to change, saved the moment he leaves it. A double
  // click selects all of it, ready for Cmd+C, and Copy does the same in
  // one press. LinkedIn above opens the profile to send it from. See
  // internal/draft and docs/api.md, Drafts.
  import { api, type DraftJob, type PersonDetail } from '../api'
  import { draftWatch, underWay } from '../drafts.svelte'
  import { fmtAgo, fmtElapsed, fmtLeft } from '../format'
  import { errorText, fault, toast } from '../toast.svelte'
  import Busy from './Busy.svelte'
  import Icon from './Icon.svelte'

  let { person, onsaved }: { person: PersonDetail; onsaved?: (p: PersonDetail) => void } = $props()

  $effect(() => draftWatch.watch())
  const job = $derived(draftWatch.of(person.id))
  const running = $derived(underWay(job) ? job : undefined)
  const busy = $derived(!!running)

  // The parent keys this by the person, so the first values hold.
  // svelte-ignore state_referenced_locally
  let text = $state(person.draft)
  // svelte-ignore state_referenced_locally
  let saved = $state(person.draft)
  // svelte-ignore state_referenced_locally
  let draftedAt = $state(person.drafted_at)
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle')

  // Draft pressed and not answered yet, so the click shows at once, and
  // Cancel pressed, which takes a moment to reach the model.
  let starting = $state(false)
  let cancelling = $state(false)
  // The message before Draft again, so the new one can be undone.
  let before: string | undefined

  // A draft that ends done is kept by the app: take it from there, the
  // way it was saved.
  let seen = ''
  $effect(() => {
    const now = job ? `${job.id}:${job.state}` : ''
    if (now === seen) return
    seen = now
    if (!busy) cancelling = false
    if (job?.state === 'done') take()
    else if (job && !busy) before = undefined
  })

  async function take() {
    try {
      const p = await api.person(person.id)
      const was = before
      before = undefined
      text = saved = p.draft
      draftedAt = p.drafted_at
      saveState = 'idle'
      onsaved?.(p)
      if (was && was !== p.draft) {
        toast.show('Drafted again', 'plain', { label: 'Undo', run: () => keep(was).catch((e) => fault(e)) })
      }
    } catch (e) {
      fault(e)
    }
  }

  async function draft() {
    if (busy || starting) return
    // What is typed and not yet saved is saved first, so Undo can bring
    // it back.
    await save()
    before = saved
    starting = true
    try {
      draftWatch.put(await api.startDraft(person.id))
    } catch (e) {
      before = undefined
      fault(e)
    } finally {
      starting = false
    }
  }

  async function cancel() {
    if (!busy) return
    cancelling = true
    try {
      await api.cancelDraft(person.id)
    } catch (e) {
      cancelling = false
      toast.show(errorText(e), 'bad')
    }
  }

  async function keep(value: string) {
    const updated = await api.patchPerson(person.id, { draft: value })
    text = saved = updated.draft
    onsaved?.(updated)
  }

  async function save() {
    if (text === saved) return
    saveState = 'saving'
    try {
      await keep(text)
      saveState = 'saved'
    } catch (e) {
      saveState = 'error'
      fault(e)
    }
  }

  // A double click takes the whole message, not a word of it, because
  // what follows is Cmd+C.
  function selectAll(e: MouseEvent) {
    ;(e.currentTarget as HTMLTextAreaElement).select()
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      toast.show('Copied the message')
    } catch (e) {
      toast.show(errorText(e), 'bad')
    }
  }

  // The clock ticks here, between the app's tells, and the time left
  // counts down from the last one, as RunProgress does.
  let now = $state(Date.now())
  let toldAt = $state(Date.now())
  $effect(() => {
    void job?.seconds_left
    toldAt = Date.now()
  })
  $effect(() => {
    if (!busy) return
    const t = setInterval(() => (now = Date.now()), 1000)
    return () => clearInterval(t)
  })

  function stepOf(j: DraftJob): string {
    switch (j.state) {
      case 'queued':
        return 'Waiting for the draft before it'
      case 'reading':
        return 'Reading their profile from Exa'
      case 'loading':
        return `Loading ${j.model ?? 'the language model'}`
      default:
        return `Writing with ${j.model ?? 'the language model'}`
    }
  }
  const fraction = $derived(running && running.fraction >= 0 ? running.fraction : -1)
  const left = $derived(
    running?.seconds_left == null ? 'measuring the time left' : fmtLeft(running.seconds_left - (now - toldAt) / 1000),
  )
  // While the model writes, the box shows what it wrote so far.
  const shown = $derived(running ? (running.text ?? '').trimStart() : text)
  const failed = $derived(job?.state === 'failed' ? (job.error ?? 'The draft stopped.') : '')
  const stateText = $derived(
    saveState === 'saving' ? 'Saving' : saveState === 'saved' ? 'Saved' : saveState === 'error' ? 'Not saved'
      : draftedAt && !busy ? `Drafted ${fmtAgo(draftedAt)}` : '',
  )
</script>

<section class="section first-message" aria-labelledby="message-{person.id}">
  <div class="row between">
    <label class="label" id="message-{person.id}" for="message-text-{person.id}">First message</label>
    <span class="small state" aria-live="polite">{stateText}</span>
  </div>
  {#if busy || shown}
    <textarea id="message-text-{person.id}" class="textarea" rows="8" value={shown} readonly={busy}
      aria-busy={busy} oninput={(e) => { text = e.currentTarget.value; saveState = 'idle' }} onblur={save}
      ondblclick={selectAll} title="Double click selects all of it, ready for Cmd+C"></textarea>
  {/if}
  {#if running}
    <div class="draft-progress" aria-live="polite">
      <p class="step ellipsis">{stepOf(running)}</p>
      <div class="progress" class:unknown={fraction < 0} role="progressbar" aria-label="How far the draft is"
        aria-valuemin={0} aria-valuemax={100} aria-valuenow={fraction >= 0 ? Math.round(fraction * 100) : undefined}>
        <i style:width={`${Math.max(0, fraction) * 100}%`}></i>
      </div>
      <p class="meta num">{fmtElapsed(now - new Date(running.started_at).getTime())} so far, {left}</p>
    </div>
  {/if}
  {#if failed}
    <p class="failed small" role="alert">
      {failed}{#if failed.includes('Settings')} <a href="/app/settings#language">Open Settings</a>{/if}
    </p>
  {/if}
  <div class="actions">
    {#if busy}
      <button class="btn act" type="button" disabled={cancelling} onclick={cancel}
        title="Stop writing. The message before it stays as it was">
        <Busy {fraction} />{cancelling ? 'Cancelling' : 'Cancel'}
      </button>
    {:else}
      <button class="btn act" class:primary={!text} type="button" disabled={starting} onclick={draft}
        title={text ? 'Write a new message from their profile. Undo brings this one back' : 'Write a first message from their profile, on this Mac'}>
        {#if starting}<Busy />{/if}<Icon name={text ? 'refresh' : 'draft'} />{starting ? 'Starting' : text ? 'Draft again' : 'Draft'}
      </button>
    {/if}
    {#if text && !busy}
      <button class="btn act" type="button" onclick={copy} title="Copy the whole message">
        <Icon name="copy" />Copy
      </button>
    {/if}
  </div>
</section>

<style>
  .first-message { display: grid; gap: 10px; }
  .state { color: var(--ink-3); font-variant-numeric: tabular-nums; }
  .actions { display: flex; flex-wrap: wrap; gap: 10px; }
  /* Draft, Draft again and Cancel take the same place, as wide as the
     longest of them, so nothing moves when one turns into another. */
  .act { position: relative; min-width: 9.5em; }
  .draft-progress { display: grid; gap: 6px; min-width: 0; }
  .step { font-size: 14px; font-weight: 600; color: var(--ink); min-height: 1.4em; }
  .meta { font-size: 12px; color: var(--ink-2); }
  .failed { color: var(--bad); }
</style>
