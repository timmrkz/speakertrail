// What the walks in walks/ are held to, run by make interface. A walk never
// waits on the clock: it moves on the event of its step, or on the page's
// own, see walks/lib/settled.mjs and the testing skill. So it never sleeps,
// never waits for the network to be quiet, and gives no wait a deadline
// of its own: a wait that never ends fails at Playwright's default of
// 30 s, and a file that hangs at the runner's limit, --test-timeout in
// the Makefile, #96. Only ESLint's own rules, no plugin.
const why = 'a walk never waits on the clock: wait for the event of the step, or for the page, see walks/lib/settled.mjs'

export default [
  {
    files: ['walks/**/*.mjs'],
    languageOptions: { ecmaVersion: 'latest', sourceType: 'module' },
    linterOptions: { reportUnusedDisableDirectives: 'error' },
    rules: {
      'no-restricted-properties': ['error', { property: 'waitForTimeout', message: why }],
      'no-restricted-syntax': [
        'error',
        { selector: "Literal[value='networkidle']", message: why },
        { selector: "Property[key.name='timeout']", message: `${why}. Playwright's default of 30 s ends a wait that never ends, the runner's limit a walk that hangs` },
        { selector: "CallExpression[callee.name=/^set(Timeout|Interval)$/][arguments.length>1]", message: why },
        { selector: "CallExpression[callee.name='setInterval']", message: why },
      ],
    },
  },
]
