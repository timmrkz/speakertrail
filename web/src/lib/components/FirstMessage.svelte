<script lang="ts">
  // The first message to a person: Draft has the language model write it
  // on this Mac, in the background, and it grows here as it is written.
  // Then it is Tim's to change, saved the moment he leaves it. The box is
  // as tall as the message, so it never scrolls. A double click selects
  // all of it, ready for Cmd+C, and the copy mark in its corner, there
  // while the pointer is on the box, copies it in one press. Draft stands
  // in the person's row of buttons, between Connect and LinkedIn, which
  // row draws. See internal/draft and docs/api.md, Drafts.
  import type { Snippet } from 'svelte'
  import { fit } from '../fit'
  import { api, type DraftJob, type PersonDetail } from '../api'
  import { draftWatch, underWay } from '../drafts.svelte'
  import { fmtAgo, fmtElapsed, fmtLeft } from '../format'
  import { languageWatch } from '../language.svelte'
  import { navigate } from '../router.svelte'
  import { errorText, fault, toast } from '../toast.svelte'
  import Busy from './Busy.svelte'
  import Icon from './Icon.svelte'

  let { person, row, onsaved }: {
    person: PersonDetail
    // The person's row of buttons, handed the Draft button to place.
    row: Snippet<[Snippet]>
    onsaved?: (p: PersonDetail) => void
  } = $props()

  $effect(() => draftWatch.watch())
  $effect(() => languageWatch.watch())
  // Without a model here, or llama-server to run it, Draft leads to the
  // model in Settings rather than starting work that cannot end, the way
  // a control in FrameFairy that needs its model sends there.
  const noModel = $derived(languageWatch.loaded && languageWatch.state.standing.mark !== 'ok')
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
    if (noModel) {
      navigate('/app/settings#language')
      return
    }
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
      // The app knows better how the draft stands.
      draftWatch.read()
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
  // A draft the model's answer did not fit has … where its part goes, and
  // says why until the next draft.
  const problem = $derived(job?.state === 'done' ? (job.problem ?? '') : '')
  const stateText = $derived(
    saveState === 'saving' ? 'Saving' : saveState === 'saved' ? 'Saved' : saveState === 'error' ? 'Not saved'
      : draftedAt && !busy ? `Drafted ${fmtAgo(draftedAt)}` : '',
  )
</script>

<!-- Draft and Cancel take one place in the row, each as wide as its words,
     so LinkedIn after it moves a little as one turns into the other. While
     it writes, the draft carries the beam
     and the fill in the one control that can do something about it, as a
     download does in Settings. Only one button in the row is filled:
     Connect, until Tim is connected, and then Draft while there is no
     message. -->
{#snippet draftButton()}
  {#if busy}
    <button class="btn act" type="button" disabled={cancelling} onclick={cancel}
      title="Stop writing. The message before it stays as it was">
      <Busy {fraction} />{cancelling ? 'Cancelling' : 'Cancel'}
    </button>
  {:else}
    <button class="btn act" class:primary={!text && person.connected} type="button" disabled={starting} onclick={draft}
      title={noModel ? languageWatch.state.standing.why
        : text ? 'Write a new message from their profile. Undo brings this one back' : 'Write a first message from their profile, on this Mac'}>
      {#if starting}<Busy />{/if}<Icon name={text ? 'refresh' : 'draft'} />{starting ? 'Starting' : text ? 'Draft again' : 'Draft'}
    </button>
  {/if}
{/snippet}

{@render row(draftButton)}

{#if busy || shown || failed || problem || stateText}
  <section class="section first-message" aria-labelledby="message-{person.id}">
    <div class="row between">
      <label class="label" id="message-{person.id}" for="message-text-{person.id}">First message</label>
      <span class="small state" aria-live="polite">{stateText}</span>
    </div>
    {#if busy || shown}
      <div class="box">
        <textarea id="message-text-{person.id}" class="textarea" rows="3" value={shown} readonly={busy}
          use:fit={shown} aria-busy={busy} oninput={(e) => { text = e.currentTarget.value; saveState = 'idle' }} onblur={save}
          ondblclick={selectAll} title="Double click selects all of it, ready for Cmd+C"></textarea>
        {#if text && !busy}
          <button class="btn icon copy" type="button" onclick={copy} aria-label="Copy" title="Copy the whole message">
            <Icon name="copy" />
          </button>
        {/if}
      </div>
    {/if}
    {#if running}
      <div class="draft-progress" aria-live="polite">
        <p class="step ellipsis">{stepOf(running)}</p>
        <p class="meta num">{fmtElapsed(now - new Date(running.started_at).getTime())} so far, {left}</p>
      </div>
    {/if}
    {#if failed}
      <p class="failed small" role="alert">
        <!-- A failure that needs the Exa key leads to the top of Settings,
             where the key is, and any other to the language model. -->
        {failed}{#if failed.includes('Settings')}{' '}<a href={failed.includes('Exa') ? '/app/settings' : '/app/settings#language'}>Open Settings</a>{/if}
      </p>
    {/if}
    {#if problem}
      <p class="failed small" role="alert">{problem}</p>
    {/if}
  </section>
{/if}

<style>
  .first-message { display: grid; gap: var(--gap-group); }
  .state { color: var(--ink-3); font-variant-numeric: tabular-nums; }
  .act { position: relative; }
  /* The box is as tall as the message, see lib/fit.ts, so it never
     scrolls and needs no handle to pull it longer. */
  .box { position: relative; min-width: 0; }
  .box .textarea { display: block; width: 100%; resize: none; overflow-y: hidden; }
  /* The copy mark sits in the box's top right corner, there while the
     pointer is on the box or the keyboard is on the mark, the way a code
     block on GitHub shows its copy button. Its ground is the box's, so
     it never sits on the words under it unseen, and its states are every
     plain button's. */
  .box .copy {
    position: absolute; top: 4px; right: 4px; width: 30px; height: 30px; border-color: transparent; color: var(--ink-2);
    opacity: 0; pointer-events: none;
  }
  .box:hover .copy, .copy:focus-visible { opacity: 1; pointer-events: auto; }
  .draft-progress { display: grid; gap: 6px; min-width: 0; }
  .step { font-size: 14px; font-weight: 600; color: var(--ink); min-height: 1.4em; }
  .meta { font-size: 12px; color: var(--ink-2); }
  .failed { color: var(--bad); }
</style>
