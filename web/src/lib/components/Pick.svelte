<script lang="ts">
  // A list to pick from, wearing the app's own clothes. Ported from
  // FrameFairy's Pick.svelte, its plain list without the search, in Speaker
  // Trail's tokens.
  //
  // A plain <select> is drawn by the system: on macOS the webview hands the
  // whole list to AppKit, which paints it in its own box, and a row there
  // can hold a name and nothing else. A model's row says whether it is on
  // this Mac and how big it is, so the list is the app's own, built on
  // bits-ui, the headless half of shadcn-svelte: it brings the keyboard,
  // the roles, the focus, the typeahead and the placement, and no look at
  // all. Everything seen here is this file and the tokens in app.css.
  //
  // This is the one list to pick from in the app. A second one would be a
  // second way of doing the same thing.
  //
  // Every element is drawn here rather than left to bits-ui, through the
  // child snippets it offers for exactly this, so no class has to be
  // reached with :global.
  import { Select } from 'bits-ui'
  import { tick } from 'svelte'
  import { said } from '../said'
  import Icon, { type IconName } from './Icon.svelte'

  type Option = {
    value: string
    label: string
    // A word beside the name, what it costs or that it will not run here.
    detail?: string
    warn?: boolean
    // Rows stand in groups under a head, the way a pop-up menu on the Mac
    // parts kinds of thing. A group is named on its first row and runs
    // until the next.
    group?: string
    // A mark of the row's own where the tick goes, which says how the
    // thing stands rather than that it is chosen: a model on this Mac has
    // the tick, one still to fetch a download arrow in grey. A list with a
    // mark on any row says what is chosen by the accent alone.
    mark?: { icon: IconName; title: string; faint?: boolean }
  }

  let {
    value = '',
    options,
    onpick,
    label,
    title = '',
    id = undefined,
    // Which way the trigger reads. A settings row ends at the right.
    align = 'left',
    disabled = false,
    // A choice that cannot work yet wears a frame in the colour of a
    // warning, so it is seen on the control itself.
    tone = undefined,
    // Asked as the list is about to open. A choice that may not be changed
    // now says yes, and the list stays shut.
    refuse = undefined,
    // What the trigger says while nothing is picked. It is a state, not a
    // row: a list never offers nothing as a choice.
    placeholder = '',
    // A trigger as wide as what it says now, rather than as wide as the
    // longest thing it could say, for a list alone at the end of a row.
    fit = false,
    // Lets the caller reach the trigger, to bring it into view and focus it.
    trigger: button = $bindable(),
  }: {
    // What is picked. It goes one way only: what the trigger says is what
    // the caller says is true, never what was clicked before the answer.
    value?: string
    options: Option[]
    onpick?: (value: string) => void
    // What the list is for, said out loud for anything reading the screen.
    label: string
    title?: string
    id?: string
    align?: 'left' | 'right'
    disabled?: boolean
    tone?: 'warn' | 'err'
    refuse?: () => boolean
    placeholder?: string
    fit?: boolean
    trigger?: HTMLButtonElement
  } = $props()

  const edge = $derived(align === 'right' ? 'end' : 'start')
  const marked = $derived(options.some((o) => o.mark))

  // How far under its trigger the list hangs.
  const drop = 6

  const picked = $derived(options.find((o) => o.value === value))
  // A value not in the list yet still reads as itself rather than nothing.
  const shown = $derived(picked?.label ?? (value || placeholder))
  const unpicked = $derived(!value && !!placeholder)

  // The longest thing the list can say. The trigger keeps room for it, so
  // the row does not change width as it is used. The stylesheet measures
  // it: the longest label is drawn and hidden.
  const longest = $derived(options.reduce((most, o) => (o.label.length > most.length ? o.label : most), placeholder))

  // A list whose rows say something beside their names reads from the
  // left, the way a pop-up menu on the Mac puts names down the left.
  const columns = $derived(options.some((o) => o.detail || o.group))
  const fromRight = $derived(align === 'right' && !columns)

  // Which row the list itself thinks is ticked. Between a click and the
  // answer the list has moved on and the answer may yet be no, so it is
  // set back to the truth whenever the truth changes and every time the
  // list opens.
  let ticked = $state('')
  $effect(() => {
    ticked = value
  })

  let open = $state(false)
  function opens(next: boolean) {
    if (next && refuse?.()) return
    open = next
  }

  // A list that closes lets go of the keyboard, so the letters typed next
  // are not taken as a pick of its own (FrameFairy issues #370 and #876).
  let escaped = false
  function letGo() {
    const how = escaped ? 'Escape closed' : 'closed'
    escaped = false
    if (document.activeElement !== button) return
    button?.blur()
    said(`pick: ${how} ${label} and let go of it`)
  }

  // The picked row stands in the middle of the list as it opens, the way a
  // pop-up menu on the Mac opens on what is chosen.
  function onPicked(list: HTMLElement) {
    const row = list.querySelector<HTMLElement>('[data-chosen]')
    if (row) list.scrollTop = row.offsetTop - (list.clientHeight - row.offsetHeight) / 2
  }
</script>

{#snippet face(props: Record<string, unknown>)}
  <button
    {...props}
    bind:this={button}
    type="button"
    class="pick"
    class:right={align === 'right'}
    class:warn={tone === 'warn'}
    class:err={tone === 'err'}
    aria-label={label}
    {title}
    {id}
  >
    <span class="said">
      {#if !fit}<span class="room" aria-hidden="true">{longest}</span>{/if}
      <span class:unpicked>{shown}</span>
    </span>
    <span class="updown"><Icon name="pick" size={12} /></span>
  </button>
{/snippet}

<Select.Root
  type="single"
  bind:value={ticked}
  items={options}
  {disabled}
  bind:open={() => open, opens}
  onOpenChange={(next) => {
    if (next !== open) return
    if (next) ticked = value
    else tick().then(letGo)
  }}
  onValueChange={(v) => {
    if (v !== value) onpick?.(v)
  }}
>
  <Select.Trigger>
    {#snippet child({ props })}{@render face(props)}{/snippet}
  </Select.Trigger>
  <Select.Portal>
    <!-- Right under the trigger and along the edge it reads from, so the
         list opens where the eye already is. -->
    <Select.Content sideOffset={drop} align={edge} collisionPadding={8} onEscapeKeydown={() => (escaped = true)}>
      {#snippet child({ props, wrapperProps, open })}
        {#if open}
          <div {...wrapperProps}>
            <div {...props} class="pick-list" use:onPicked>
              {#each options as option, i (option.value)}
                {#if option.group && option.group !== options[i - 1]?.group}
                  <div class="pick-group" class:first={i === 0}>{option.group}</div>
                {/if}
                <Select.Item value={option.value} label={option.label}>
                  {#snippet child({ props: item, selected })}
                    <div {...item} class="pick-row" class:right={fromRight}
                      data-chosen={option.value === value ? '' : undefined} title={option.label}>
                      {#if fromRight}
                        <span class="what">{option.label}</span>
                        <span class="tick">{#if selected}<Icon name="check" size={12} />{/if}</span>
                      {:else}
                        {#if option.mark}
                          <span class="tick marked" class:faint={option.mark.faint} title={option.mark.title}>
                            <Icon name={option.mark.icon} size={12} />
                          </span>
                        {:else}
                          <span class="tick">{#if selected && !marked}<Icon name="check" size={12} />{/if}</span>
                        {/if}
                        <span class="what">{option.label}</span>
                        {#if option.detail}
                          <span class="detail" class:warn={option.warn}>{option.detail}</span>
                        {/if}
                      {/if}
                    </div>
                  {/snippet}
                </Select.Item>
              {/each}
            </div>
          </div>
        {/if}
      {/snippet}
    </Select.Content>
  </Select.Portal>
</Select.Root>

<style>
  /* The trigger is a field to choose in, the height, border and corners of
     every field in the app, with what is picked and the mark that says it
     is a list. */
  .pick {
    display: flex; align-items: center; gap: 8px; width: 100%; min-width: 0;
    height: var(--control-h); padding: 0 8px 0 12px; max-width: var(--pick-max);
    border: var(--line-w) solid var(--line-strong); border-radius: 8px; background: var(--surface);
    color: var(--ink); text-align: left; cursor: pointer;
  }
  .pick:hover:not(:disabled) { background: var(--surface-hover); }
  .pick:active:not(:disabled) { background: var(--surface-press); }
  .pick:disabled { background: var(--disabled); color: var(--disabled-ink); border-color: transparent; cursor: default; }
  .pick.warn { border-color: var(--warn); }
  .pick.err { border-color: var(--bad); }

  /* No edge while the list is open: the lit row in the list already says
     where the keyboard is. */
  .pick[aria-expanded='true']:focus-visible { outline: none; background-image: none; }
  /* FrameFairy's ring, inside the edge so it lines up with the fields
     beside it. */
  .pick:focus-visible { outline: 2px solid var(--accent); outline-offset: -1px; background-image: none; }

  /* The two lie on top of each other, so the box is as wide as the longest
     thing the list can say and what is picked is drawn in it. */
  .said { display: grid; flex: 1; min-width: 0; }
  .said > span {
    grid-area: 1 / 1; align-self: center; min-width: 0;
    overflow: hidden; white-space: nowrap; text-overflow: ellipsis;
  }
  .room { visibility: hidden; }
  .unpicked { color: var(--ink-3); }
  .pick.right .said { text-align: right; }

  /* The mark is the colour of a unit, because it says what the field is
     rather than what it holds. */
  .updown { display: flex; align-items: center; flex: none; color: var(--ink-3); }

  /* The list lies over the page. It is as wide as what it holds and grows
     away from the edge it hangs on, so the list is never the reason a name
     is cut short. It is never narrower than its trigger, and never wider
     than the room the placement says there is. */
  .pick-list {
    width: max-content;
    min-width: var(--bits-floating-anchor-width);
    max-width: calc(var(--bits-floating-available-width, 100vw) - 8px);
    max-height: 320px; overflow-y: auto;
    background: var(--surface); border: var(--line-w) solid var(--line-strong); border-radius: 8px;
    color: var(--ink); font-size: 14px;
    /* Over the rail and an open sheet, under a toast and an info bubble.
       bits-ui lifts its wrapper to the list's own z-index. */
    position: relative; z-index: 50;
  }

  /* A row is as tall as a control whatever the list holds. */
  .pick-row {
    flex: none; display: flex; align-items: center; gap: 8px;
    height: var(--control-h); padding: 0 8px; cursor: pointer; user-select: none;
  }

  /* The row under the pointer and the row the keyboard is on are one
     thing, so bits-ui marks both the same. */
  .pick-row[data-highlighted] { background: var(--hover); }
  .pick-row[data-highlighted]:active { background: var(--press); }

  /* What is chosen is in the words every chosen thing wears, with the
     tick before it, the way a menu on the Mac marks it. The tint is for
     a choice that stays on screen. In a menu it would read as the row
     under the pointer. */
  .pick-row[data-chosen] { color: var(--chosen-ink); }
  .pick-row[data-disabled] { color: var(--ink-3); cursor: default; }

  /* The tick keeps its place whether or not it is there, so no row moves
     sideways as the chosen one changes. */
  .tick { display: flex; align-items: center; justify-content: center; flex: none; width: 12px; color: var(--chosen-ink); }
  .tick.marked { color: inherit; }
  .tick.faint { color: var(--ink-3); }

  .what { flex: 1; min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .pick-row.right .what { text-align: right; }

  /* The word beside a name, in the colour of a unit, apart from the name
     by the room a column needs, so the words line up down the list. */
  .detail { flex: none; margin-left: 24px; color: var(--ink-3); font-size: 12px; font-variant-numeric: tabular-nums; }
  .detail.warn { color: var(--warn); }

  /* The head of a group of rows, and the line that parts it from the
     group before. */
  .pick-group {
    flex: none; padding: 8px 8px 4px 28px; border-top: var(--line-w) solid var(--line);
    color: var(--ink-3); font-size: 12px; font-weight: 600; user-select: none;
  }
  .pick-group.first { border-top: none; padding-top: 4px; }
</style>
