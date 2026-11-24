<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte'
  import { api, onDictation, onLoudness, onReportDraft, onUndo, pictureUrl, type Dictation, type ReportDraft, type ReportPicture } from '../lib/api'
  import { asking, moveTab, typing } from '../lib/keys'
  import Removed from '../lib/components/Removed.svelte'
  import { chime, wake } from '../lib/chime'
  import Busy from '../lib/components/Busy.svelte'
  import Icon from '../lib/components/Icon.svelte'
  import Info from '../lib/components/Info.svelte'
  import Confirm from '../lib/components/Confirm.svelte'
  import { toast } from '../lib/toast.svelte'
  // A line in the app's log. said is the words being dictated here.
  import { said as log } from '../lib/said'
  import { changed, Dictating, edit, type Ask, type Span } from '../lib/dictating'
  import { Spring } from '../lib/spring'
  import { Undo } from '../lib/undo'
  import { caretBox, landing } from '../lib/caret'
  import { revealed } from '../lib/reveal'
  import { afterRest } from '../lib/rest'

  // Report a Problem, in the window Bugs in the sidebar opens: what Tim
  // says happened, the pictures he attaches, and one button that puts it
  // on the Desktop with the app's log, as the one file he sends. The Go
  // side writes it and shows it in Finder, see report.go, bugs.go and
  // docs/LOGGING.md. What is written and attached is a draft the Go side
  // keeps until Report, so closing the window loses nothing. Ported from
  // FrameFairy's screens/Report.svelte, without screen recordings, with
  // the reasons it gives.

  let what = $state('')
  let making = $state(false)
  // Why a report could not be made, or pictures attached, is a toast over
  // the window. Beside the buttons it wrapped over three lines and stood
  // where nothing else was about it.
  const oops = (text: string) => toast.show(text, 'bad')

  // Dictate: the words appear in the field while they are said, where the
  // cursor was, and the last of them change as more is heard until a
  // pause settles them. They can be corrected while the person talks on.
  // Enter, Escape or the button keeps all of it, written in as one step
  // Cmd+Z takes back. Escape removed it once, on claude.ai, and Tim lost a
  // long report in FrameFairy to an Escape meant for the app, with no Cmd+Z
  // to bring it back. On a Mac, Escape ends Dictation and keeps what was
  // said. The Go side records and writes down, see listen.go.
  //
  // The field is a textarea, and the browser draws its cursor, moves it
  // and scrolls it, as in every text field. What the dictation does to the
  // text is worked out in lib/dictating.ts, in letters and numbers, and
  // written into the field where it changed.
  let field: HTMLTextAreaElement
  let listening = $state(false)
  let hearing = $state(false)
  // The last words of a dictation being written down, which Report
  // waits for, so Tab and Enter right after Enter make a report with
  // them in it.
  let writing: Promise<void> = Promise.resolve()
  // The recording in hand, and how many times the Go side was told to
  // settle or cut what it is hearing, see tell.
  let recording = 0
  let asked = 0
  // Which press of Dictate is the latest, and the last recording this
  // page finished with. While the Mac asks for the microphone, Stop
  // answers before Listen does, so a press of Dictate can still be
  // waiting when the next one is made. Only the latest press takes
  // Listen's answer, and nothing from a finished recording is taken
  // while the next one has no number yet.
  let presses = 0
  let past = 0
  // The asks to settle or cut go to the Go side one at a time, in the
  // order they were made, and stopping waits for them. Each request runs
  // on a goroutine of its own there, so two Shift+Enters in quick
  // succession could arrive the other way round.
  let told: Promise<unknown> = Promise.resolve()
  function ask(call: () => Promise<unknown>) {
    told = told.then(call).catch(() => {})
  }
  // The dictation in the field, while there is one, and the pieces the
  // Go side last said it has written down.
  let dictation: Dictating | null = null
  let pieces: string[] = []
  // A letter being put together from more than one key, like ä on a
  // German keyboard. What the Go side says meanwhile waits for it, since
  // a change to the text would end it.
  let composing = false
  let waited: Dictation | null = null

  const sentence = (e: unknown) => {
    const said = e instanceof Error ? e.message : String(e)
    return said.charAt(0).toUpperCase() + said.slice(1)
  }

  // The field's text, as it is sent.
  const read = () => {
    what = field.value
    keepText()
  }

  // The field is kept as it changes, a moment after the last change, and
  // at once when the window goes.
  let saving: ReturnType<typeof setTimeout> | null = null
  let loaded = false
  function keepText() {
    if (!loaded) return
    if (saving) clearTimeout(saving)
    saving = afterRest(saveNow, 300)
  }
  function saveNow() {
    if (saving) clearTimeout(saving)
    saving = null
    if (loaded) api.saveReportText(what).catch(() => {})
  }

  // The pictures attached, in the order they came, and the ones removed
  // a moment ago, by id. A removed one keeps its place, says so and
  // offers itself back, and then the grid closes over it.
  let pictures = $state<ReportPicture[]>([])
  let removed = $state<Record<string, { picture: ReportPicture }>>({})
  let attaching = $state(false)
  const tiles = $derived.by(() => {
    const all = [...pictures.map((p) => ({ p, gone: false })), ...Object.values(removed).map((r) => ({ p: r.picture, gone: true }))]
    return all.sort((a, b) => order(a.p.id) - order(b.p.id))
  })
  const order = (id: string) => Number(id.slice(1)) || 0

  // Whether there is nothing to report: no words and no pictures. Report
  // and Clear are off then. Words still being written down after a
  // dictation may be on their way, so Report waits for them rather than
  // being off.
  const nothing = $derived(!what.trim() && pictures.length === 0 && !hearing)
  // People says the report takes People along, as people.json: every
  // person with what the person screen shows and the profile text,
  // without names or links, see store.Shared. A report of People alone
  // is something to report. Clear leaves it as it is, since it empties
  // what was written and attached.
  let people = $state(false)
  // Whether People was pressed since the window opened.
  let pressed = false
  // When each button of the row can do nothing. They stay in the Tab
  // order then, see the row.
  const reportOff = $derived((nothing && !people) || making || listening || attaching)
  const clearOff = $derived(nothing || making || listening || hearing || attaching)
  const dictateOff = $derived(hearing || making)
  const attachOff = $derived(attaching || making)

  // Clear empties the report, the words and every picture, once the box
  // over the page has asked.
  let clearing = $state(false)
  // What the last Clear took, while Cmd+Z can still bring it back and
  // Cmd+Y take it again: whether it took words, the pictures, and
  // whether it is undone now. The words go as one step of the field's
  // undo, and the pictures follow that step. Anything done after it ends
  // it, as in any editor.
  let cleared: { text: boolean; pics: ReportPicture[]; undone: boolean } | null = null
  const unclear = () => (cleared = null)
  // The field's undo, which the page keeps, see lib/undo.ts, and the text
  // as it was when the dictation in hand began.
  let steps = new Undo()
  let began = ''

  async function clearAll() {
    clearing = false
    log('report: cleared', { level: 'info' })
    // The box lets go of the focus as it closes, and the field can only
    // take it, and its undo the words, once it has.
    await tick()
    const pics = [...pictures]
    removed = {}
    pictures = []
    field.focus({ preventScroll: true })
    const text = !!field.value
    if (text) {
      steps.did(0, field.value.length, field.value, '', true)
      field.value = ''
    }
    what = ''
    // A save still to come would bring the words back.
    if (saving) clearTimeout(saving)
    saving = null
    cleared = { text, pics, undone: false }
    try {
      tookNewest(await newest(() => api.clearReport()))
    } catch (e) {
      oops(sentence(e))
    }
  }

  // Undo of Clear puts its pictures back one call at a time, and Redo
  // clears with a call of its own. Each waits for the one before to be
  // answered, or a Redo pressed while Undo was still putting pictures
  // back cleared between them. Every other change to the draft waits its
  // turn here too, see newest.
  let turn: Promise<void> = Promise.resolve()

  // Undo and Redo in the Edit menu, see menu.go, and the field's own
  // Cmd+Z. After Clear they bring everything back and clear it again.
  // Otherwise they go to the field, or Cmd+Z in it would do nothing.
  async function undo(how: 'undo' | 'redo') {
    if (asking()) return
    const c = cleared
    if (!c || c.undone !== (how === 'redo')) {
      if (document.activeElement === field || !typing()) step(how)
      else document.execCommand(how)
      return
    }
    c.undone = how === 'undo'
    log(`report: ${how === 'undo' ? 'Undo brings back' : 'Redo clears again'} the words and ${c.pics.length} pictures`, { level: 'info' })
    field.focus({ preventScroll: true })
    if (c.text) step(how)
    const before = turn
    let done = () => {}
    turn = new Promise((r) => (done = r))
    try {
      if (how === 'redo') {
        if (saving) clearTimeout(saving)
        saving = null
        pictures = pictures.filter((p) => !c.pics.some((q) => q.id === p.id))
        await before
        try {
          took(await api.clearReport())
        } catch (e) {
          oops(sentence(e))
        }
        return
      }
      pictures = [...pictures, ...c.pics].sort((a, b) => order(a.id) - order(b.id))
      await before
      let last: ReportDraft | null = null
      for (const p of c.pics) {
        try {
          last = await api.putBackPicture(p.id)
        } catch (e) {
          oops(sentence(e))
        }
      }
      // A Redo pressed meanwhile has taken the pictures away again, and
      // shows what its clear leaves.
      if (last && c.undone) took(last)
    } finally {
      done()
    }
  }

  onMount(() => onUndo(undo))

  // took shows what the Go side says the draft holds now, with why a
  // picture was left out.
  function took(d: ReportDraft) {
    pictures = d.pictures ?? []
    people = !!d.people
    for (const id of Object.keys(removed)) if (pictures.some((p) => p.id === id)) delete removed[id]
    if (d.error) oops(d.error.endsWith('.') ? d.error : d.error + '.')
  }

  // Each change to the pictures is answered with the whole draft as it
  // was then, which the page has already shown. An answer that comes
  // while a later change is on its way would take that change back: a
  // picture put back a moment after it was removed went again until the
  // answer to the put back came, and a Clear pressed meanwhile had nothing
  // to bring back. FrameFairy has this too. So only the answer to the
  // latest change is shown. And each change is asked once the one before
  // is answered, in the turn Undo and Redo keep, since the app answers
  // each request on its own goroutine and two asked at once could be done
  // in the other order, a remove after the put back that came later.
  let draftsAsked = 0
  function newest(ask: () => Promise<ReportDraft>): Promise<ReportDraft | null> {
    const n = ++draftsAsked
    const call = turn.then(ask)
    turn = call.then(
      () => {},
      () => {},
    )
    return call.then((d) => (n === draftsAsked ? d : null))
  }
  const tookNewest = (d: ReportDraft | null) => d && took(d)

  // A drop says what came of it on its own.
  function dropped(d: ReportDraft) {
    unclear()
    took(d)
  }

  async function attach() {
    if (attachOff) return
    unclear()
    attaching = true
    try {
      tookNewest(await newest(() => api.attachPictures()))
    } catch (e) {
      oops(sentence(e))
    } finally {
      attaching = false
      home()
    }
  }

  // People goes on or off at once, and the next press takes it back. The
  // Go side keeps it with the draft, in the turn every change to the
  // draft waits for.
  function togglePeople() {
    if (making) return
    unclear()
    pressed = true
    people = !people
    const on = people
    newest(() => api.takePeople(on)).then(tookNewest, (e) => {
      people = !on
      oops(sentence(e))
    })
  }

  // stay keeps the focus in the field when Dictate or Attach is pressed
  // with the mouse. WebKit takes it from the field to nothing and the page
  // gave it back, so the field's mark went out and came back at every
  // press of the microphone. The keyboard still reaches both.
  function stay(e: MouseEvent) {
    if (document.activeElement === field) e.preventDefault()
  }

  // home gives the focus back to the field when nothing has it, the way
  // a Mac app gives it back to where it was when a panel closes.
  function home() {
    const at = document.activeElement
    if (!at || at === document.body) field.focus({ preventScroll: true })
  }

  function removePicture(p: ReportPicture) {
    unclear()
    removed[p.id] = { picture: p }
    pictures = pictures.filter((q) => q.id !== p.id)
    newest(() => api.removePicture(p.id)).then(tookNewest, (e) => oops(sentence(e)))
  }

  // The picture is back in its place at once, the way a removed one goes
  // at once. Turned away, at ten pictures, it stands as removed again.
  function putBack(id: string) {
    const r = removed[id]
    if (!r) return
    unclear()
    pictures = [...pictures, r.picture]
    delete removed[id]
    const again = () => {
      if (!pictures.some((p) => p.id === id)) removed[id] = r
    }
    newest(() => api.putBackPicture(id)).then(
      (d) => {
        // A later change, like Clear, has the say over this picture.
        if (!d) return
        took(d)
        again()
      },
      (e) => {
        pictures = pictures.filter((p) => p.id !== id)
        again()
        oops(sentence(e))
      },
    )
  }

  // A picture pasted into the field is attached, the way Mail takes one.
  // Text pastes as text.
  function pasted(e: ClipboardEvent) {
    const files = [...(e.clipboardData?.files ?? [])].filter((f) => f.type.startsWith('image/'))
    if (files.length === 0) return
    e.preventDefault()
    unclear()
    for (const f of files) {
      const reader = new FileReader()
      reader.onload = () => {
        const data = String(reader.result).replace(/^data:[^,]*,/, '')
        newest(() => api.attachPasted(f.name || 'Pasted picture.png', data)).then(tookNewest, (err) => oops(sentence(err)))
      }
      reader.readAsDataURL(f)
    }
  }

  // Delete or Backspace on a tile removes its picture.
  function tileKey(e: KeyboardEvent, p: ReportPicture) {
    if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault()
      removePicture(p)
    }
  }

  // How loud the microphone is, which the button moves with, and why the
  // last recording stopped by itself.
  let loud = $state(0)
  let note = $state('')
  // Why the microphone hears nothing, while it records.
  let unheard = $state('')

  // How far the microphone has grown, from 0 at rest to 1 for a raised
  // voice. It hangs on a spring that each reading, ten a second, pulls to
  // the loudness: it swells with a word, goes a little past it and
  // settles, and falls back between words with a small bounce, see
  // lib/spring.ts. Worked out every frame and written straight to the
  // icon as a scale: no layout, no transition, nothing else moves.
  let mic = $state<HTMLSpanElement>()
  const grown = new Spring(0.25, 0.5)
  let frame = 0
  let then = 0
  const still = matchMedia('(prefers-reduced-motion: reduce)')
  function grow(now: number) {
    const target = listening && !still.matches ? loud : 0
    const rests = grown.step(target, then ? Math.min(now - then, 100) : 16)
    if (mic) mic.style.transform = `scale(${(16 + 8 * grown.at) / 28})`
    then = now
    frame = rests ? 0 : requestAnimationFrame(grow)
    if (!frame) then = 0
  }
  $effect(() => {
    void [loud, listening, mic]
    if (!frame) frame = requestAnimationFrame(grow)
  })
  onDestroy(() => cancelAnimationFrame(frame))

  // The selection in the field, from where it starts to where it ends.
  const selection = (): Span => [field.selectionStart, field.selectionEnd]

  // The end of the selection that moves, where the cursor is drawn.
  const head = () => (field.selectionDirection === 'backward' ? field.selectionStart : field.selectionEnd)

  // reveal scrolls the field the least that shows the line of letter at,
  // or lines lines under it, with the padding round it, from where the
  // field was at top, see lib/reveal.ts. The page does it before the
  // browser would, so the browser finds the cursor in view and scrolls
  // nothing itself: WebKit on a Mac puts a cursor out of view in the
  // middle of the field.
  function reveal(at: number, top = field.scrollTop, lines = 0) {
    scroll(place(at, top, lines), top)
  }

  // place is where reveal would put the field's top, and top itself when
  // that line is in view already.
  function place(at: number, top = field.scrollTop, lines = 0): number {
    const cs = getComputedStyle(field)
    const line = parseFloat(cs.lineHeight)
    const [over, under] = caretBox(field, at)
    const f = { top, height: field.clientHeight, end: field.scrollHeight, pad: parseFloat(cs.paddingTop), foot: parseFloat(cs.paddingBottom), line }
    return revealed(f, over + lines * line, under + lines * line)
  }

  // scroll puts the field's top at top, and has WebKit draw the cursor
  // again. WebKit draws it again only where its place on the screen
  // changed, so a cursor that moved a line while the field scrolled by
  // the same line stayed drawn on the line it left. The cursor's colour
  // said another way has the field drawn again, once a frame.
  function scroll(top: number, from = field.scrollTop) {
    if (top !== field.scrollTop) field.scrollTop = top
    if (top === from || redrawing) return
    redrawing = true
    field.style.caretColor = field.style.caretColor ? '' : 'currentcolor'
    requestAnimationFrame(() => (redrawing = false))
  }
  let redrawing = false

  // show writes the dictation's text into the field where it changed, and
  // puts the selection where it is now. The field follows the words at
  // the end of the report the way a terminal follows its output: while its
  // last line is in view, it keeps it in view as the words come. Scrolled
  // up, it stays where it was put. setRangeText forgets which way a
  // selection runs, so it is read first.
  function show(d: Dictating, sel: Span) {
    const cs = getComputedStyle(field)
    const follow = field.scrollHeight - field.clientHeight - field.scrollTop <= parseFloat(cs.paddingBottom) + parseFloat(cs.lineHeight) / 2
    const top = field.scrollTop
    const seen = place(head()) === top
    const dir = field.selectionDirection
    if (field.value !== d.text) {
      const [a, b, put] = changed(field.value, d.text)
      field.setRangeText(put, a, b)
    }
    if (field.selectionStart !== sel[0] || field.selectionEnd !== sel[1] || field.selectionDirection !== dir) field.setSelectionRange(sel[0], sel[1], dir)
    if (follow && d.end === d.text.length) scroll(place(d.end), top)
    else if (seen && sel[0] === d.end && sel[1] === d.end) reveal(d.end, top)
  }

  // tell passes on to the Go side what the person did to the words still
  // being heard, and what it says until it has taken that in is passed
  // over, since it would show those words a second time.
  function tell(a: Ask | null) {
    if (!a || !recording) return
    const r = recording
    asked++
    if ('cut' in a) ask(() => api.breakListening(r, a.cut))
    else ask(() => api.settleListening(r, a.settle, a.words))
  }

  // take shows what the Go side says: what it has written down and what it
  // is still hearing. A cursor put in the words being heard, by a click or
  // a key, settles them first, so they are not changed under it.
  function take(m: Dictation) {
    const d = dictation
    if (!d) return
    if (composing) return void (waited = m)
    pieces = m.said ?? []
    const before = selection()
    if (d.inside(before)) return tell(d.settle())
    const sel = d.hear(pieces, m.hearing, before)
    for (const [a, b, n] of d.moves.splice(0)) steps.moved(a, b, n)
    show(d, sel)
    read()
  }

  // composed is the end of a letter put together from more than one key,
  // and shows what the Go side said meanwhile.
  function composed() {
    composing = false
    const m = waited
    waited = null
    if (m) take(m)
  }

  // editing hears a key in the field before the browser makes the edit.
  // Cmd+Z and Cmd+Y are the field's undo, see undo, and a word
  // typed right after a word that was said gets a space in front of it,
  // as the letter is typed, so the two are one step.
  function editing(e: InputEvent) {
    held = field.scrollTop
    if (e.inputType === 'historyUndo' || e.inputType === 'historyRedo') {
      e.preventDefault()
      return void undo(e.inputType === 'historyUndo' ? 'undo' : 'redo')
    }
    if (!dictation || e.isComposing || e.inputType !== 'insertText' || !e.data) return
    const put = dictation.typing(selection(), e.data)
    if (put === e.data) return
    e.preventDefault()
    document.execCommand('insertText', false, put)
  }

  // typed follows an edit the person made, which the browser has made in
  // the field. It is a step of the field's undo, and the dictation in
  // hand is told of it.
  function typed() {
    unclear()
    const was = dictation?.text ?? what
    const [a, b, put] = edit(was, field.value, field.selectionEnd)
    steps.did(a, b, was.slice(a, b), put)
    follow(a, b, put)
    reveal(head(), held ?? field.scrollTop)
    held = null
    read()
  }

  // Where the field was scrolled to before the edit the browser is about
  // to make, which may scroll it.
  let held: number | null = null

  // moving puts the line an arrow key takes the cursor to in view before
  // the browser moves it there, so the browser has no need to scroll.
  function moving(e: KeyboardEvent) {
    if (e.isComposing || composing) return
    const to = landing(field.value, field.selectionStart, field.selectionEnd, field.selectionDirection === 'backward', e)
    if (to) reveal(to[0], field.scrollTop, to[1])
  }

  // follow tells the dictation in hand of a change in the field, made by
  // a key or by Undo, and the Go side what it means for the words still
  // being heard, see edited in lib/dictating.ts.
  function follow(a: number, b: number, put: string) {
    if (!dictation) return
    for (const x of dictation.edited(a, b, put, listening)) tell(x)
    dictation.moves.length = 0
  }

  // step takes the field's last step back, or does again the last one
  // taken back, with the cursor after it.
  function step(how: 'undo' | 'redo') {
    const c = how === 'undo' ? steps.undo(field.value) : steps.redo(field.value)
    if (!c) return
    const top = field.scrollTop
    field.setRangeText(c[2], c[0], c[1], 'end')
    follow(...c)
    scroll(place(field.selectionEnd), top)
    read()
  }

  // finish ends the dictation with every piece the speech model wrote
  // down, and the selection stays where the person has it. What was said,
  // with whatever was typed and corrected meanwhile, is one step Cmd+Z
  // takes back.
  function finish(said: string[]) {
    const d = dictation
    dictation = null
    waited = null
    if (!d) return read()
    show(d, d.hear(said, '', selection(), true))
    steps.end(began, d.text)
    home()
    read()
  }

  onMount(() =>
    onLoudness((n) => {
      if (listening && n.recording > past && (!recording || n.recording === recording)) loud = n.level
    }),
  )

  onMount(() =>
    onDictation((d) => {
      if (!listening || d.recording <= past || (recording && d.recording !== recording)) return
      if (d.error || d.ended) {
        if (d.error) oops(sentence(d.error))
        if (d.ended) note = sentence(d.ended) + '.'
        listening = false
        chime('off')
        loud = 0
        finish(d.said ?? pieces)
        return
      }
      unheard = d.unheard ? sentence(d.unheard) : ''
      if ((d.asked ?? 0) < asked) return
      take(d)
    }),
  )

  async function dictate() {
    if (dictateOff) return
    if (listening) return keep()
    unclear()
    note = ''
    unheard = ''
    listening = true
    recording = 0
    asked = 0
    pieces = []
    told = Promise.resolve()
    // What is said goes where the cursor is, or where it was when the
    // button was pressed, which a textarea keeps.
    const at = field.selectionEnd
    dictation = new Dictating(field.value, at)
    began = field.value
    steps.begin()
    field.focus({ preventScroll: true })
    field.setSelectionRange(at, at)
    wake()
    const press = ++presses
    try {
      const r = await api.listen()
      if (press !== presses) return
      recording = r
      // The microphone is open: what is said from here is written down.
      if (listening) chime('on')
    } catch (e) {
      if (press !== presses) return
      listening = false
      finish([])
      oops(sentence(e))
    }
  }

  // keep stops listening and keeps everything that was said, the rest
  // written in once the speech model has it.
  async function keep() {
    if (!listening) return
    listening = false
    chime('off')
    loud = 0
    hearing = true
    let written = () => {}
    writing = new Promise((done) => (written = done))
    try {
      await told
      // It may have stopped by itself just before, and then says why.
      const d = await api.stopListening()
      past = Math.max(past, d.recording)
      if (d.recording === recording) {
        pieces = d.said ?? pieces
        if (d.ended) note = sentence(d.ended) + '.'
      }
    } catch (e) {
      oops(sentence(e))
    } finally {
      finish(pieces)
      hearing = false
      written()
    }
  }

  // Enter and Escape keep what was said, the way the button does.
  // Removing it is Cmd+Z after, one step, so no key throws a report away
  // for good.
  function key(e: KeyboardEvent) {
    if (e.key === 'Tab' && !e.isComposing && !e.altKey && !e.metaKey && !e.ctrlKey) return void tab(e)
    if (!listening) return
    if (e.key === 'Enter' && !e.isComposing && !e.shiftKey && !e.altKey && !e.metaKey && !e.ctrlKey) {
      e.preventDefault()
      keep()
    } else if (e.key === 'Escape' && !e.isComposing) {
      e.preventDefault()
      e.stopPropagation()
      keep()
    }
  }

  // Tab walks the window in the order it is written, see moveTab and
  // the row of buttons. While dictating Tab does nothing: only Enter,
  // Escape and the button end a dictation.
  let page: HTMLElement
  const named = (e: Element | null) =>
    e === field ? 'the field' : e?.matches('figure') ? 'a picture' : (e?.getAttribute('aria-label') ?? e?.textContent?.trim() ?? 'a button')
  function tab(e: KeyboardEvent) {
    if (listening) {
      e.preventDefault()
      return
    }
    const here = document.activeElement
    if (!page || asking() || (here && here !== document.body && !page.contains(here))) return
    const next = moveTab(page, e)
    if (next && next !== here) log(`report: ${e.shiftKey ? 'Shift+Tab' : 'Tab'} moved the focus from ${named(here)} to ${named(next)}`)
  }

  // A recording is not left running behind another page.
  onDestroy(() => {
    if (listening) api.cancelListening().catch(() => {})
  })

  // The draft from before, the window's last time open or the app's last
  // run, and what a drop on the window attaches.
  onMount(() => {
    api
      .reportDraft()
      .then((d) => {
        if (d.text && !what) {
          field.value = d.text
          what = d.text
        }
        // People pressed before the draft came keeps what the press
        // said, since its own answer may have come first.
        took({ ...d, error: '', people: pressed ? people : d.people })
      })
      .catch(() => {})
      .finally(() => (loaded = true))
    const noDraft = onReportDraft(dropped)
    const leave = () => saveNow()
    window.addEventListener('pagehide', leave)
    return () => {
      noDraft()
      window.removeEventListener('pagehide', leave)
      saveNow()
    }
  })

  async function make() {
    if (reportOff) return
    making = true
    note = ''
    // Pressed while the last words of a dictation are written down, it
    // shows it is making the report at once and waits for them, so they
    // are in it.
    if (hearing) log('report: Report waits for the last words of the dictation', { level: 'info' })
    await writing
    // Nothing was said after all, so there is nothing to report.
    if (!what.trim() && pictures.length === 0 && !people) {
      making = false
      return
    }
    // Nothing is kept from here: the Go side lets go of the draft and
    // closes the window, which would otherwise keep the text again as it
    // went.
    if (saving) clearTimeout(saving)
    saving = null
    loaded = false
    try {
      // Finder shows the report, and the Go side closes this window.
      // People goes along when the draft says so, so a press of People
      // still on its way is waited for.
      await turn
      await api.reportProblem(what)
      // The report holds it all, and the next one starts empty.
      field.value = ''
      what = ''
      steps = new Undo()
      pictures = []
      removed = {}
      people = false
    } catch (e) {
      oops(sentence(e))
    } finally {
      making = false
      loaded = true
    }
  }
</script>

<svelte:window onkeydowncapture={key} />

<!-- What Tim writes may name people, so the log names nothing in here
     but the buttons, see control in lib/said.ts. -->
<section class="report" data-personal bind:this={page}>
  <div class="card">
    <!-- A plain textarea: the browser draws the cursor, moves it and
         scrolls, and lib/dictating.ts says what a dictation does to the
         text. -->
    <textarea
      class="telling"
      bind:this={field}
      readonly={making}
      aria-label="What happened"
      placeholder={listening ? '' : 'What happened, and what were you doing when it did?'}
      spellcheck="true"
      oninput={typed}
      onbeforeinput={editing}
      onkeydown={moving}
      oncompositionstart={() => (composing = true)}
      oncompositionend={composed}
      onpaste={pasted}></textarea>
    <!-- The row is written in the order Tab takes, Report first, so Enter,
         Tab, Enter ends a dictation and makes the report, and drawn from
         right to left, Dictate and Attach on the left as a toolbar has
         them. Tim set this order testing #46. A button that cannot be
         pressed now says so with aria-disabled rather than disabled, so
         it keeps its place in the order whatever is in the field. -->
    <div class="actions">
      <button
        class="btn primary make"
        type="button"
        onclick={make}
        aria-disabled={reportOff}
        title="Saves the report on the Desktop, shows it in Finder and closes this window">
        {#if making}<Busy />{/if}
        {making ? 'Making…' : 'Report'}
      </button>
      <!-- Clear asks first and Cmd+Z undoes it, so a slip of the hand
           costs nothing. -->
      <button
        class="btn danger clear"
        type="button"
        onclick={() => { if (!clearOff) clearing = true }}
        aria-disabled={clearOff}
        title="Empties this report, what you wrote and everything attached, after asking">
        Clear
      </button>
      <Info label="How a report works" side="right">
        Say what happened with the microphone, or type it. Enter or Escape keeps what you said, and Cmd+Z undoes it. Attach screenshots
        with the paperclip, by dropping them on this window or by pasting them, up to ten of 10 MB each. What you write and attach stays
        here when you close the window, and Clear empties it all after asking, which Cmd+Z undoes. Report saves one zip file on the
        Desktop, shows it in Finder and closes this window. It holds what you wrote, the pictures without what else their files hold,
        like a place or a camera, the app's log, which build this is, what Mac it runs on and your settings. With the People button on,
        it holds your People too, with what each person screen shows and the profile texts, with each person's own name written
        [name] and every link [link]. Other names in a profile text go along. Never your notes, first messages or keys, and your home folder is written as ~. You can look inside before
        you send it.
      </Info>
      <!-- Only what the microphone hears, and why a recording stopped by
           itself. What the buttons are doing shows on the buttons, how the
           page works is in the info mark, and what went wrong is a toast. -->
      <div class="words">
        {#if listening && unheard}
          <span class="error">{unheard}</span>
        {:else if !listening && !hearing && note}
          <span>{note}</span>
        {/if}
      </div>
      <!-- People goes along as people.json, without names or links. On,
           it wears green, as every toggle that is on does. -->
      <button
        class="btn icon good people"
        type="button"
        onmousedown={stay}
        onclick={togglePeople}
        aria-pressed={people}
        aria-disabled={making}
        aria-label="Take People along"
        title={people
          ? 'People go along with this report, with their profile texts, but without their names or profile links. Click to leave them out'
          : 'Take People along: what each person screen shows and the profile texts, without their names or profile links'}>
        <Icon name="people" />
      </button>
      <button
        class="btn icon attach"
        type="button"
        onmousedown={stay}
        onclick={attach}
        aria-disabled={attachOff}
        aria-label="Attach screenshots"
        title="Attach screenshots. You can also drop them on this window or paste them">
        {#if attaching}<Busy />{/if}
        <Icon name="attach" />
      </button>
      <!-- Square, the microphone alone, the way a toolbar shows it: what
           it does is in its title. Its name says what a press does now,
           Dictate, Stop or Writing, for the keyboard and a screen reader. -->
      <button
        class="btn icon dictate"
        type="button"
        onmousedown={stay}
        class:listening
        onclick={dictate}
        aria-disabled={dictateOff}
        aria-label={listening ? 'Stop' : hearing ? 'Writing' : 'Dictate'}
        title={listening
          ? 'Stop. Keeps what you said, and Enter or Escape does the same. Cmd+Z undoes it'
          : hearing
            ? 'Writing down what you said'
            : 'Dictate. Say what happened. The speech model on this Mac writes it into the field as you talk, and you can correct it while it does'}>
        {#if listening || hearing}<Busy />{/if}
        <span class="grows" bind:this={mic}><Icon name="mic" size={28} line={1.5 * (28 / 16)} /></span>
      </button>
    </div>
  </div>
  {#if tiles.length > 0}
    <!-- The pictures attached, in the order they came, each with a bin
         on hover. A removed one keeps its place for a moment, says so and
         offers itself back. -->
    <div class="shots">
      <ul class="pictures" aria-label="Pictures">
        {#each tiles as t (t.p.id)}
          <li class="tile" data-id={t.p.id}>
            {#if t.gone}
              <Removed onputback={() => putBack(t.p.id)} onend={() => delete removed[t.p.id]} />
            {:else}
              <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
              <figure tabindex="0" title={t.p.name} aria-label="{t.p.name}, {t.p.width} by {t.p.height}" onkeydown={(e) => tileKey(e, t.p)}>
                <img src={pictureUrl(t.p.id)} alt={t.p.name} draggable="false" />
              </figure>
              <button class="btn icon quiet bin" type="button" tabindex="-1" title="Remove" aria-label="Remove {t.p.name}" onclick={() => removePicture(t.p)}>
                <Icon name="trash" />
              </button>
            {/if}
          </li>
        {/each}
      </ul>
    </div>
  {/if}
  {#if clearing}
    <Confirm title="Clear the report?" oncancel={() => (clearing = false)}>
      <p>Everything you wrote and attached will be deleted. You can undo it with Cmd+Z.</p>
      {#snippet actions()}
        <button class="btn" type="button" onclick={() => (clearing = false)}>Cancel</button>
        <button class="btn danger" type="button" onclick={clearAll}>Clear</button>
      {/snippet}
    </Confirm>
  {/if}
</section>

<style>
  /* The page in the middle of the window, at a width that reads. The card
     and the pictures keep their own height, and the page scrolls once the
     pictures reach past the window's foot. */
  .report {
    padding: var(--gap); display: flex; flex-direction: column; gap: var(--gap);
    width: 100%; max-width: 880px; margin: 0 auto; min-height: 0;
  }
  .report > :global(*) { flex: none; }

  .card {
    display: grid; gap: var(--gap); padding: var(--gap);
    background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius);
  }

  /* A field of ten whole lines of 19 px, with the padding of a control
     round them. */
  .telling {
    width: 100%; height: calc(10 * 19px + 2 * 8px + 2px); box-sizing: border-box; overflow-y: auto;
    white-space: pre-wrap; overflow-wrap: anywhere; font: inherit; font-size: 14px; line-height: 19px; color: var(--ink);
    background: var(--surface); border: 1px solid var(--line-strong); border-radius: 8px; padding: 8px 12px; margin: 0; resize: none;
  }
  .telling:hover, .telling:focus { border-color: var(--line-hover); }
  .telling::placeholder { color: var(--ink-3); }

  /* One row of controls of one height: the icons on the left, what the
     microphone hears in the middle, the words on the right. */
  .actions { display: flex; flex-direction: row-reverse; align-items: center; gap: var(--gap-control); min-width: 0; }
  .words { flex: 1; min-width: 0; font-size: 13px; color: var(--ink-2); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .words .error { color: var(--bad); }

  /* The microphone moves with the voice it hears. It grows from 16 px to
     24 px about its middle, in a box of 28 px that never changes, so
     nothing in the row moves. It is drawn at 28 px and scaled down to
     rest, with a line as much thicker, so at rest it is the icon every
     other button has, and grown it is never a small picture blown up,
     which WebKit draws blurred. */
  .dictate .grows {
    display: block; flex: none; width: 28px; height: 28px;
    transform: scale(calc(16 / 28)); transform-origin: 50% 50%;
  }
  .btn.icon :global(svg) { width: auto; height: auto; }
  .dictate.listening { color: var(--accent); }

  /* The pictures under the card, in tiles of one size with a space
     between them: three across, four once the page is wide enough. Every
     tile has the shape 16 to 10, the picture whole inside it, on whole
     pixels. */
  .shots { container: shots / inline-size; }
  .pictures {
    --cols: 3;
    --tile-w: round(down, calc((100cqw - (var(--cols) - 1) * var(--gap)) / var(--cols)), 1px);
    list-style: none; margin: 0; padding: 0; display: grid;
    grid-template-columns: repeat(var(--cols), var(--tile-w));
    grid-auto-rows: round(down, calc(var(--tile-w) * 10 / 16), 1px);
    gap: var(--gap);
  }
  @container shots (min-width: 636px) { .pictures { --cols: 4; } }
  .tile { position: relative; min-width: 0; }
  .tile figure {
    margin: 0; position: absolute; inset: 0; background: var(--surface);
    border: 1px solid var(--line); border-radius: 8px; overflow: hidden;
  }
  /* The picture is fitted into the tile whole, in its middle. */
  .tile img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: contain; display: block; }
  /* The bin in the tile's top right corner, only while the pointer is on
     the tile or the keyboard is in it. As FrameFairy has it: the quiet
     trash can of every list, 24 px square and 6 px in from the corner,
     with no box of its own, since on a solid square it looked pasted on,
     Tim found there. A shade falls from that corner instead, so it reads
     on any picture, and the tile's edge lifts the way a row's does. */
  .tile .bin {
    position: absolute; top: 6px; right: 6px; width: 24px; height: 24px; opacity: 0; pointer-events: none;
    color: var(--ink);
  }
  .tile figure::after {
    content: ''; position: absolute; inset: 0; opacity: 0; pointer-events: none;
    background: radial-gradient(circle at 100% 0, var(--scrim), transparent calc(3 * var(--control-h)));
  }
  .tile:hover figure, .tile:focus-within figure { border-color: var(--line-strong); }
  .tile:hover figure::after, .tile:focus-within figure::after,
  .tile:hover .bin, .tile:focus-within .bin { opacity: 1; pointer-events: auto; }
  .tile .bin:hover { color: var(--bad); background: var(--bad-soft); }
</style>
