// A wait for something to come to rest before the work it holds back:
// the hand that types or swipes, or a row that slides. What then runs is
// what would run a moment later all the same. The mark on it lets a walk
// run it at once rather than spend its time on the clock, the way
// FrameFairy's preview/settled.ts does. It waits as setTimeout does, and
// clearTimeout calls it off. Ported from FrameFairy.
export function afterRest(fn: () => void, ms: number): number {
  const run = () => fn()
  ;(run as unknown as { rest: boolean }).rest = true
  return window.setTimeout(run, ms)
}
