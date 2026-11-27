<script lang="ts">
  // The first message to a person: Draft has the language model write it
  // on this Mac, in the background, and it grows here as it is written.
  // Then it is Tim's to change, saved the moment he leaves it. The box is
  // always there, empty too, so he can write a message himself, and as
  // tall as the message, so it never scrolls. The copy mark in its
  // corner, there while the pointer is on the box, copies it in one
  // press. Draft stands in the person's row of buttons, between Connect
  // and LinkedIn, which row draws. See internal/draft and docs/api.md,
  // Drafts.
  import type { Snippet } from 'svelte'
  import { fit, fitNow } from '../fit'
  import { api, type DraftJob, type PersonDetail } from '../api'
  import { draftWatch, underWay } from '../drafts.svelte'
  import { fmtAgo, fmtElapsed, fmtLeft } from '../format'
  import { languageWatch } from '../language.svelte'
  import { pasteAsShown } from '../pastedlines'
  import { navigate } from '../router.svelte'
  import { said } from '../said'
  import { stepOut } from '../keys'
  import { errorText, Problem } from '../problem.svelte'
  import Busy from './Busy.svelte'
  import Icon from './Icon.svelte'
  import ProblemNote from './ProblemNote.svelte'

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

  // What went wrong with the message, a draft that could not start or
  // stop, a save or a copy, says so over the top of the box, where the
  // click was (Tim, testing #60 on 22 November 2026, #72).
  const boxProblem = new Problem()

  // A draft that ends done is kept by the app: take it from there, the
  // way it was saved.
  let seen = ''
  $effect(() => {
    const now = job ? `${job.id}:${job.state}` : ''
    if (now === seen) return
    seen = now
    if (!busy) cancelling = false
    if (job?.state === 'done') take()
  })

  async function take() {
    try {
      const p = await api.person(person.id)
      text = saved = p.draft
      draftedAt = p.drafted_at
      saveState = 'idle'
      onsaved?.(p)
    } catch (e) {
      boxProblem.failed(e)
    }
  }

  async function draft() {
    if (busy || starting) return
    if (noModel) {
      navigate('/app/settings#language')
      return
    }
    // What is typed and not yet saved is saved first, so Undo in the Edit
    // menu can bring it back. A save that failed says so over the box and
    // keeps the draft from writing over what it could not save.
    boxProblem.clear()
    await save()
    if (saveState === 'error') return
    starting = true
    try {
      draftWatch.put(await api.startDraft(person.id))
    } catch (e) {
      boxProblem.failed(e)
    } finally {
      starting = false
    }
  }

  async function cancel() {
    if (!busy) return
    cancelling = true
    boxProblem.clear()
    try {
      await api.cancelDraft(person.id)
    } catch (e) {
      cancelling = false
      boxProblem.say(e)
      // The app knows better how the draft stands.
      draftWatch.read()
    }
  }

  async function keep(value: string) {
    const updated = await api.patchPerson(person.id, { draft: value })
    text = saved = updated.draft
    // An emptied message was drafted at no time, as the app says.
    draftedAt = updated.drafted_at
    onsaved?.(updated)
  }

  async function save() {
    if (text === saved) return
    saveState = 'saving'
    boxProblem.clear()
    try {
      await keep(text)
      saveState = 'saved'
    } catch (e) {
      saveState = 'error'
      boxProblem.failed(e)
    }
  }

  // The copy mark copies through the app, since the web view refuses the
  // window's own clipboard, see docs/api.md, Copy. It turns into a tick
  // in the same frame as the click and back a moment later, as Copy on
  // the Updates page does. A copy that fails says so over the box, where
  // the click was, in words that say what to do, and the web view's own
  // words go to the app's log.
  let copied = $state(false)
  let copiedTimer: ReturnType<typeof setTimeout> | undefined
  let refusedTimer: ReturnType<typeof setTimeout> | undefined
  $effect(() => () => {
    clearTimeout(copiedTimer)
    clearTimeout(refusedTimer)
  })
  async function copy() {
    copied = true
    boxProblem.clear()
    clearTimeout(copiedTimer)
    clearTimeout(refusedTimer)
    copiedTimer = setTimeout(() => (copied = false), 1500)
    try {
      await api.copy(text)
    } catch (e) {
      clearTimeout(copiedTimer)
      copied = false
      said(`copy: ${errorText(e)}`, { level: 'warn' })
      const refused = 'Could not copy the message. Click in it, press Cmd+A, then Cmd+C.'
      boxProblem.say(refused)
      refusedTimer = setTimeout(() => {
        if (boxProblem.text === refused) boxProblem.clear()
      }, 8000)
    }
  }

  // The clock ticks here, between the app's tells, and the time left
  // counts down from the last one, as a run does on its Stop button.
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
  // The box grows with every piece the model writes and every key Tim
  // types, measured once the text is in it, see lib/fit.ts.
  let field = $state<HTMLTextAreaElement>()
  $effect(() => {
    void shown
    if (field) fitNow(field)
  })
  // A draft that failed before their profile came, like one Exa had no
  // copy for, says nothing once Tim pasted the page: it is no longer how
  // they stand (Tim's report of 24 November 2026, #88).
  const stale = $derived(!!job && !!person.profile_read_at && new Date(person.profile_read_at) > new Date(job.started_at))
  // Nor does one that says what the person says above it already, that
  // Exa has no copy of their profile, which a draft started by adding
  // their link often ends with (#115).
  const told = $derived(person.read === 'not_indexed' && !!job?.no_copy)
  const failed = $derived(job?.state === 'failed' && !stale && !told ? (job.error ?? 'The draft stopped.') : '')
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

<section class="section first-message" aria-labelledby="message-{person.id}">
  <div class="row between">
    <label class="label" id="message-{person.id}" for="message-text-{person.id}">First message</label>
    <span class="small state" aria-live="polite">{stateText}</span>
  </div>
  <div class="box">
    <textarea id="message-text-{person.id}" class="textarea" rows="3" value={shown} readonly={busy}
      bind:this={field} use:fit use:pasteAsShown aria-busy={busy} oninput={(e) => { text = e.currentTarget.value; saveState = 'idle'; boxProblem.clear() }} onblur={save}
      onkeydown={(e) => stepOut(e, `person-row-${person.id}`)}></textarea>
    {#if text && !busy}
      <button class="btn icon copy" class:copied type="button" onclick={copy} aria-label={copied ? 'Copied' : 'Copy'}
        title={copied ? 'Copied' : 'Copy the whole message'}>
        <Icon name={copied ? 'check' : 'copy'} />
      </button>
    {/if}
    <ProblemNote of={boxProblem} over />
  </div>
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
  .box:hover .copy, .copy:focus-visible, .box .copy.copied { opacity: 1; pointer-events: auto; }
  .box .copy.copied { color: var(--good); }
  /* What went wrong lies over the top of the box, beside the mark. */
  .box { --problem-end: 38px; }
  .draft-progress { display: grid; gap: 6px; min-width: 0; }
  .step { font-size: 14px; font-weight: 600; color: var(--ink); min-height: 1.4em; }
  .meta { font-size: 12px; color: var(--ink-2); }
  .failed { color: var(--bad); }
</style>
