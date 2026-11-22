// fit makes a textarea as tall as what it holds, so it never scrolls.
// Tim asked for it for the first message on 21 November 2026: the
// messages are short, and he wants to read one whole. The action fits it
// when it comes and whenever the box gets narrower or wider, since the
// lines wrap anew. A new value is fitted by fitNow from an $effect of
// the component, which runs once the value is in the box. An action's
// update runs before that, so it measured the text before, one piece of
// a draft behind, and the box caught up only at the end (Tim, 22
// November 2026).
//
// A height of auto lets the box shrink to what it holds before it is
// measured. For that moment the page under it is shorter, so a pane
// scrolled near its end would jump back. The ancestors' scroll is kept
// and put back in the same frame, before anything is drawn.

export function height(el: HTMLTextAreaElement): number {
  // The edge is outside scrollHeight and inside the height the box is
  // given, since every box here is border-box.
  return el.scrollHeight + el.offsetHeight - el.clientHeight
}

export function fitNow(el: HTMLTextAreaElement) {
  const kept: [Element, number][] = []
  for (let p = el.parentElement; p; p = p.parentElement) if (p.scrollTop) kept.push([p, p.scrollTop])
  el.style.height = 'auto'
  el.style.height = `${height(el)}px`
  for (const [p, top] of kept) p.scrollTop = top
}

export function fit(el: HTMLTextAreaElement) {
  let width = el.clientWidth
  const watch = new ResizeObserver(() => {
    if (el.clientWidth === width) return
    width = el.clientWidth
    fitNow(el)
  })
  watch.observe(el)
  fitNow(el)
  return {
    destroy() {
      watch.disconnect()
    },
  }
}
