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
  // The control needs nothing of its own: app.css gives any button holding
  // a beam its rounded clip and a stacking context, so the beam lies over
  // the control's background and under its words. It is the one way work
  // in hand is drawn on a control, so it is used, never copied.
  let { fraction = -1, motes = true }: { fraction?: number; motes?: boolean } = $props()

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
  {#if fraction >= 0}
    <!-- As wide as the control and slid in from the left, so it is moved
         by the compositor and never laid out again. -->
    <span class="fill"><i style="transform: translateX({(Math.min(Math.max(fraction, 0), 1) - 1) * 100}%)"></i></span>
  {/if}
  <span class="ring"></span>
  {#if motes}
    {#each specks as m (m.at)}
      <i class="mote" style="left: {m.at}%; animation-delay: {m.wait}ms; animation-duration: {m.over}s; --sway: {m.sway}px"></i>
    {/each}
  {/if}
</span>

<style>
  .beam { position: absolute; inset: 0; z-index: -1; border-radius: inherit; overflow: hidden; pointer-events: none; }
  /* A square turning behind the control with only a 1 px rim of it
     showing, so what is seen is light running round the edge. A turning
     square is a transform, which the compositor does without painting. */
  .ring {
    position: absolute; inset: 0; border-radius: inherit; padding: 1px; overflow: hidden;
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
  .fill { position: absolute; inset: 0; border-radius: inherit; overflow: hidden; }
  .fill i {
    display: block; position: relative; width: 100%; height: 100%; overflow: hidden;
    background: linear-gradient(90deg, var(--wash-from, var(--accent-fill)) 0%, var(--wash-to, var(--accent-fill-hi)) 100%);
    box-shadow: inset -2px 0 var(--lit, var(--accent-lit)), 10px 0 14px -6px var(--lit, var(--accent-lit));
    transition: transform .25s cubic-bezier(.4, 0, .2, 1);
  }
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
