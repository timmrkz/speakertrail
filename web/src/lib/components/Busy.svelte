<script lang="ts">
  // Work running in the control it was started from, drawn the way
  // FrameFairy draws it, so running work looks the same wherever it is:
  //
  //   the beam   light travelling round the edge of the control, for as
  //              long as the work runs
  //   the motes  specks of that light drifting up through the control,
  //              behind its words
  //   the fill   how far the work has come, when that is known, with a
  //              light passing over what is done
  //
  // The control needs nothing of its own: app.css gives any control
  // holding a beam a stacking context, so the beam lies over the control's
  // background and its line and under its words, and says in --beam-edge
  // how far outside the control the line round it lies. The beam runs on
  // that line, not a pixel inside it, and the fill and the motes stay
  // inside it, with corners that are the inside of its corners. It is the
  // one way work in hand is drawn on a control, so it is used, never
  // copied.
  //
  // Given drain, the fill starts full and runs down to nothing in that
  // many seconds, with no beam, and then onend: the time something
  // removed can still be put back, see Removed.svelte. FrameFairy draws
  // that time with the same fill.
  let {
    fraction = -1,
    motes = true,
    drain = 0,
    onend,
  }: { fraction?: number; motes?: boolean; drain?: number; onend?: () => void } = $props()

  // Where the motes rise and how long each takes. Fixed rather than drawn
  // at random, so they never jump about between renders.
  const specks = [
    { at: 14, wait: 0, over: 2.6, sway: 5 },
    { at: 33, wait: 900, over: 3.1, sway: -4 },
    { at: 52, wait: 400, over: 2.4, sway: 6 },
    { at: 71, wait: 1500, over: 2.9, sway: -6 },
    { at: 88, wait: 1100, over: 2.7, sway: 4 },
  ]
</script>

<span class="beam" aria-hidden="true">
  {#if drain > 0}
    <span class="fill"><i class="drain" style="animation-duration: {drain}s" onanimationend={(e) => e.target === e.currentTarget && onend?.()}></i></span>
  {:else if fraction >= 0}
    <!-- As wide as the control and slid in from the left, so it is moved
         by the compositor and never laid out again. -->
    <span class="fill"><i style="transform: translateX({(Math.min(Math.max(fraction, 0), 1) - 1) * 100}%)"></i></span>
  {/if}
  {#if drain <= 0}<span class="ring"></span>{/if}
  {#if motes && drain <= 0}
    <span class="motes">
      {#each specks as m (m.at)}
        <i class="mote" style="left: {m.at}%; animation-delay: {m.wait}ms; animation-duration: {m.over}s; --sway: {m.sway}px"></i>
      {/each}
    </span>
  {/if}
</span>

<style>
  /* Behind the words of the control and over its background and its
     line. The control makes the stacking context this is negative in, so
     nothing else on the page is disturbed by it, and the browser paints
     a context's own border before anything inside it, so the beam lies on
     the line.

     It covers the control and the line round it, which lies --beam-edge
     outside what the control holds: the control's border. Its corners are
     the control's own, which are the corners of the outside of that line,
     so the light follows the line round the corners too. A box inside a
     control is only ever as big as the inside of its border, and a beam
     that size ran a pixel inside the line, which Tim saw, issue #29. */
  .beam { position: absolute; inset: calc(-1 * var(--beam-edge, 0px)); z-index: -1; border-radius: inherit; overflow: hidden; pointer-events: none; }
  /* A square turning behind the control with only a rim of it showing,
     so what is seen is light running round the edge. A turning square is
     a transform, which the compositor does without painting. The rim is
     as thick as the line it runs on, and the mask keeps the rim and
     throws the middle away. */
  .ring {
    position: absolute; inset: 0; border-radius: inherit; padding: max(1px, var(--beam-edge, 0px)); overflow: hidden;
    -webkit-mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
    -webkit-mask-composite: xor;
    mask: linear-gradient(#000 0 0) content-box, linear-gradient(#000 0 0);
    mask-composite: exclude;
  }
  .ring::before, .ring::after {
    content: ""; position: absolute; top: 50%; left: 50%; width: 200%; aspect-ratio: 1;
    transform: translate(-50%, -50%); animation-duration: 6.5s; animation-iteration-count: infinite;
  }
  /* The bright comet. It gets away, eases off through one long side and
     comes back through the next, never stopping and never going back. */
  .ring::before {
    background: conic-gradient(from 0deg, transparent 0%, transparent 82%, var(--accent) 90%, var(--accent-hi) 96%, var(--lit, var(--accent-lit)) 99%, transparent 100%);
    animation-name: turn;
    animation-timing-function: linear(0, 0.073 7%, 0.206 16%, 0.294 26%, 0.366 38%, 0.43 46%, 0.534 54%, 0.681 63%, 0.764 72%, 0.823 82%, 0.883 90%, 0.956 96%, 1);
  }
  /* A broad faint wash behind it, at three turns to the comet's five, so
     the rim never shows the same picture twice inside a round. */
  .ring::after {
    background: conic-gradient(from 0deg, transparent 0%, transparent 40%, var(--accent) 52%, var(--accent) 58%, transparent 71%, transparent 100%);
    animation-name: lap; animation-timing-function: linear; opacity: .5;
  }
  @keyframes turn { from { transform: translate(-50%, -50%) rotate(0turn); } to { transform: translate(-50%, -50%) rotate(5turn); } }
  @keyframes lap { from { transform: translate(-50%, -50%) rotate(0turn); } to { transform: translate(-50%, -50%) rotate(3turn); } }
  /* Inside the line, cut to the inside of its corners: a border as wide
     as the line, which nothing is drawn in, makes the inside of the box
     exactly what the line leaves, and the browser rounds the inside of a
     border by the corner less the line, so no corner of the fill or a
     mote is a hair rounder than the line round it. The same for the fill
     below. */
  .motes { position: absolute; inset: 0; border: var(--beam-edge, 0px) solid transparent; border-radius: inherit; overflow: hidden; }
  .mote {
    position: absolute; bottom: -3px; width: 3px; height: 3px; border-radius: 50%;
    background: var(--lit, var(--accent-lit)); box-shadow: 0 0 4px var(--lit, var(--accent-lit));
    opacity: 0; animation-name: drift; animation-timing-function: cubic-bezier(.35, 0, .5, 1); animation-iteration-count: infinite;
  }
  @keyframes drift {
    from { transform: translate(0, 0) scale(.5); opacity: 0; }
    18% { opacity: .85; }
    to { transform: translate(var(--sway), -32px) scale(1); opacity: 0; }
  }
  .fill { position: absolute; inset: 0; border: var(--beam-edge, 0px) solid transparent; border-radius: inherit; overflow: hidden; }
  .fill i {
    display: block; position: relative; width: 100%; height: 100%; overflow: hidden;
    background: linear-gradient(90deg, var(--wash-from, var(--accent-fill)) 0%, var(--wash-to, var(--accent-fill-hi)) 100%);
    box-shadow: inset -2px 0 var(--lit, var(--accent-lit)), 10px 0 14px -6px var(--lit, var(--accent-lit));
    transition: transform .25s cubic-bezier(.4, 0, .2, 1);
  }
  /* The time to put something back, running down from full, moved by
     the compositor like the fill. */
  .fill i.drain { animation-name: drain; animation-timing-function: linear; animation-fill-mode: forwards; }
  @keyframes drain { from { transform: translateX(0); } to { transform: translateX(-100%); } }
  .fill i::after {
    content: ""; position: absolute; inset: 0;
    background: linear-gradient(90deg, transparent 0%, rgba(255, 255, 255, .16) 50%, transparent 100%);
    animation: sheen 2.4s cubic-bezier(.45, 0, .2, 1) infinite;
  }
  @supports not ((mask-composite: exclude) or (-webkit-mask-composite: xor)) {
    .ring { display: none; }
    .beam { box-shadow: inset 0 0 0 1px var(--accent-hi); }
  }
  /* Nothing moves, and the rim is steady, so the control still says so. */
  @media (prefers-reduced-motion: reduce) {
    .ring::before { animation: none; background: var(--accent-hi); }
    .ring::after { animation: none; background: none; }
    .mote { display: none; }
  }
</style>
