<script lang="ts">
  // The title bar of a window of the app: what it is dragged by, where
  // macOS puts its close, minimise and zoom buttons, and the name of what
  // is on screen beside them. The app's window and the report window wear
  // the same one, see lib/chrome.svelte.ts, which each window follows for
  // itself.
  import { titleBar } from '../chrome.svelte'

  let { title }: { title: string } = $props()
</script>

<!-- It is the whole width of the window, so the sidebar opens under it
     and never covers the name. -->
<!-- The bar is the title bar, the way FrameFairy's is. macOS lays it out
     and puts its three buttons on its middle, and the bar takes the
     height macOS gave it rather than asking for one. Then macOS centres
     the buttons in the bar, and the name is centred between the edge of
     the window macOS draws over its top and the line at its foot, see
     .name below. Where the system draws its own title bar there is
     nothing to agree with and the stylesheet keeps its own numbers. -->
<header class="titlebar" style={titleBar.style}>
  <h1 class="name"><span class="what">{title}</span></h1>
</header>

<style>
  /* The title bar: what the app is dragged by, and where macOS puts its
     three buttons. Exactly as tall as the title bar macOS laid out, which
     the bar's style sets from what macOS answers, or the token where the
     system draws its own bar. The line at the foot is drawn inside that
     height rather than under it, so the bar is the title bar and nothing
     else. Its own style carries the measurement, not the shell's as in
     FrameFairy, because here a sheet's bar is --bar-h tall too, and it
     keeps the token. */
  .titlebar {
    flex: none; position: relative; height: var(--bar-h); background: var(--side);
    backdrop-filter: var(--frost); -webkit-backdrop-filter: var(--frost);
    box-shadow: inset 0 -1px var(--line); --wails-draggable: drag;
  }

  /* The name stands in a box as tall as the bar, so it is centred on the
     bar's middle, and starts one space after the buttons. Nothing is
     moved by a transform, so nothing is put on a compositing surface of
     its own and the whole pixel rule holds. */
  .titlebar .name {
    position: absolute; top: 0; height: var(--bar-h);
    left: calc(var(--lights-r, 0px) + var(--gap)); right: var(--gap);
    display: flex; align-items: center; overflow: hidden;
  }

  /* The name itself, as FrameFairy has it. A line box keeps more room
     below the baseline than above the capitals, so a name centred by its
     line box reads high. Trimming the box to the capitals and the
     baseline puts the letters themselves on the middle. The box is also
     what is clipped, for the ellipsis, so it is padded back out evenly,
     deeper than any letter goes, and nothing is cut. */
  .titlebar .what {
    min-width: 0; font-size: 13px; font-weight: 600; line-height: 1;
    text-box: trim-both cap alphabetic; padding: 6px 0;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }

  /* Where the browser cannot trim a text box there is nothing to pad back
     out, and a plain line box centred on the buttons is a small fraction
     high rather than wrong. */
  @supports not (text-box: trim-both cap alphabetic) {
    .titlebar .what { line-height: normal; padding: 0; }
  }

  /* Where macOS puts its buttons in the bar, it draws the edge of the
     window over the top 2 points of the page, and the line at the foot
     takes the last point. What a person sees as the bar is what lies
     between the two, and the name is centred in that. FrameFairy read it
     off Tim's pictures, its #399. */
  .titlebar[style*="--lights-r"] .name { top: 2px; height: calc(var(--bar-h) - 3px); }

  /* Where the system draws its own title bar, and in fullscreen, the name
     keeps the left of the bar, because there is nothing at the left to
     make room for. */
  .titlebar:not([style*="--lights-r"]) .name { left: var(--gap); }
</style>
