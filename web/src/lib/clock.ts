// clock writes a length of time the way a player shows it, 0:07, 1:05 or
// 1:02:03, as FrameFairy's lib/clock.ts. A recording's tile in Report a
// Problem shows its length with it.
export function clock(seconds: number): string {
  if (!isFinite(seconds) || seconds < 0) seconds = 0
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  const mm = String(m).padStart(h ? 2 : 1, '0')
  const ss = String(s).padStart(2, '0')
  return h ? `${h}:${mm}:${ss}` : `${mm}:${ss}`
}
