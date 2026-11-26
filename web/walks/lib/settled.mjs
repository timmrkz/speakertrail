// What a walk waits on where the interface sends no event of its own for
// the step. Each is an event of the page, never a time: a walk never
// waits on the clock, see the testing skill, and the lint in
// eslint.config.mjs refuses waitForTimeout, networkidle and deadlines.

// drawn resolves once the page has drawn a frame after this moment, so
// whatever a step changed, in its handler or in the microtasks after it,
// like Svelte's updates, is on screen. Not under a page clock a walk
// holds, page.clock.pauseAt, which holds the frames too.
export function drawn(page) {
  return page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(() => done()))))
}

// asking records, from the next page the walk opens, the path of every
// request the page sends, at the moment it sends it. It gives what was
// asked so far, so a walk proves that a step sent nothing by what the page
// did rather than by waiting a while for a request that might still come.
export async function asking(page) {
  await page.addInitScript(() => {
    const send = window.fetch
    window.__asked = []
    window.fetch = function (input, init) {
      window.__asked.push(new URL(input instanceof Request ? input.url : String(input), location.href).pathname)
      return send.call(this, input, init)
    }
  })
  return () => page.evaluate(() => window.__asked.slice())
}

// reads records, from the next page the walk opens, every answer the
// page has read, by path. It gives answered(path, n), which resolves once
// the page has read n answers to path and run what it does with them:
// the JSON is parsed, the code that awaited it has run, and so have the
// microtasks after it, like Svelte's updates.
export async function reads(page) {
  await page.addInitScript(() => {
    window.__read = {}
    // The app reads an answer's body as text or as JSON, see send in
    // src/lib/api.ts.
    for (const how of ['text', 'json']) {
      const body = Response.prototype[how]
      Response.prototype[how] = function () {
        const out = body.call(this)
        const path = new URL(this.url).pathname
        const count = () =>
          setTimeout(() => {
            window.__read[path] = (window.__read[path] ?? 0) + 1
            dispatchEvent(new Event('walk-read'))
          })
        out.then(count, count)
        return out
      }
    }
  })
  return (path, n = 1) =>
    page.evaluate(
      ([path, n]) =>
        new Promise((done) => {
          const check = () => {
            if ((window.__read[path] ?? 0) < n) return
            removeEventListener('walk-read', check)
            done()
          }
          addEventListener('walk-read', check)
          check()
        }),
      [path, n],
    )
}

// quiet resolves once the page asks nothing more: every request it sent
// is answered, body and all, and two drawn frames after started none. It
// gives the paths asked since the call before, so a walk proves a step
// asked the app nothing, or something, by what the page did. A walk calls
// it after a step whose requests it cannot name, like Undo that a field
// may keep or hand on to the app. It counts from its first call on a
// page, so a walk calls it once before the step.
export function quiet(page) {
  return page.evaluate(
    () =>
      new Promise((done) => {
        if (!window.__flight) {
          const flight = { open: 0, sent: 0, asked: [] }
          window.__flight = flight
          const send = window.fetch
          window.fetch = function (input, init) {
            flight.open++
            flight.sent++
            flight.asked.push(new URL(input instanceof Request ? input.url : String(input), location.href).pathname)
            const out = send.call(this, input, init)
            const end = () => {
              flight.open--
              dispatchEvent(new Event('walk-flight'))
            }
            out.then((r) => r.clone().arrayBuffer()).then(end, end)
            return out
          }
        }
        const flight = window.__flight
        const check = () => {
          if (flight.open > 0) {
            addEventListener('walk-flight', check, { once: true })
            return
          }
          const sent = flight.sent
          requestAnimationFrame(() =>
            requestAnimationFrame(() => {
              if (flight.sent !== sent || flight.open > 0) return check()
              const asked = flight.asked
              flight.asked = []
              done(asked)
            }),
          )
        }
        check()
      }),
  )
}
