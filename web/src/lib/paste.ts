// Text pasted from a web page, the way the page showed it.
//
// A browser puts a copy on the clipboard twice, as HTML and as plain
// text, and a text field takes the plain text. Chrome's plain text ends
// every paragraph with an empty line. LinkedIn's message field writes
// each line as a paragraph and an empty line as an empty paragraph, so a
// message copied there came out in Notes with three empty lines where it
// had one (Tim, 23 November 2026, #83). A field with pasteAsShown reads
// the HTML instead: a paragraph is a line, and an empty paragraph is an
// empty line. Only a page that draws a gap under its paragraphs gets an
// empty line after each, see gap.

import { said } from './said'

// The elements that start a line of their own.
const blocks = new Set([
  'ADDRESS', 'ARTICLE', 'ASIDE', 'BLOCKQUOTE', 'DD', 'DIV', 'DL', 'DT', 'FIELDSET', 'FIGURE', 'FOOTER', 'FORM',
  'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'HEADER', 'HR', 'LI', 'MAIN', 'NAV', 'OL', 'P', 'PRE', 'SECTION', 'TABLE', 'TR', 'UL',
])
// The ones a page draws a gap under, unless it says otherwise.
const spaced = new Set(['P', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6'])
// What a page holds that is never shown.
const unseen = new Set(['SCRIPT', 'STYLE', 'TEMPLATE', 'HEAD', 'TITLE', 'META', 'LINK', 'NOSCRIPT'])

// A paragraph without words, a line of its own that shows empty.
function empty(e: Element): boolean {
  return !(e.textContent ?? '').replace(/[\s ]/g, '')
}

// asShown is the text of html, with its lines as the page drew them. What
// the page holds is untrusted: it is parsed, never put on the page, and
// only its text is read.
export function asShown(html: string): string {
  const body = new DOMParser().parseFromString(html, 'text/html').body
  // A page that writes an empty line as an empty paragraph draws no gap
  // under its paragraphs, or its empty lines would show twice as tall.
  const emptyLines = [...body.querySelectorAll('p, div')].some((e) => empty(e) && !e.querySelector('p, div'))
  // gap says whether the page drew an empty line's room under a
  // paragraph. Chrome writes the margin a page gave it into its style,
  // and leaves it out where it is the browser's own.
  const gap = (e: HTMLElement) => {
    const m = e.style.marginBottom
    if (m) return parseFloat(m) > 0
    return !emptyLines
  }

  let out = ''
  // Ends the line, unless it has just ended.
  const newline = () => {
    out = out.replace(/ +$/, '')
    if (out && !out.endsWith('\n')) out += '\n'
  }
  const walk = (n: Node, pre: boolean) => {
    if (n.nodeType === Node.TEXT_NODE) {
      let t = (n.textContent ?? '').replace(/ /g, ' ')
      if (!pre) {
        t = t.replace(/[ \t\n\r\f]+/g, ' ')
        if (!out || out.endsWith('\n') || out.endsWith(' ')) t = t.replace(/^ /, '')
      }
      out += t
      return
    }
    if (!(n instanceof HTMLElement) || unseen.has(n.tagName)) return
    if (n.tagName === 'BR') {
      out = out.replace(/ +$/, '') + '\n'
      return
    }
    const ws = n.style.whiteSpace
    const keep = n.tagName === 'PRE' || (ws ? ws.startsWith('pre') || ws === 'break-spaces' : pre)
    // Cells of a row stand apart by a tab, as in Chrome's plain text.
    if ((n.tagName === 'TD' || n.tagName === 'TH') && n.previousElementSibling) out += '\t'
    const block = blocks.has(n.tagName)
    if (block) newline()
    const start = out.length
    for (const c of n.childNodes) walk(c, keep)
    if (!block) return
    // A line break last in a paragraph with words draws no line of its
    // own. It is there so the paragraph keeps its height while it is
    // being written.
    if (n.lastChild?.nodeName === 'BR' && out.length > start + 1 && out.endsWith('\n')) out = out.slice(0, -1)
    newline()
    if (spaced.has(n.tagName) && !empty(n) && gap(n) && !out.endsWith('\n\n')) out += '\n'
  }
  walk(body, false)
  return out.replace(/^\n+|\n+$/g, '')
}

// Counts the line breaks in a text, for the app's log, which never says
// the text itself.
const lines = (t: string) => t.split('\n').length

// pasteAsShown makes a text field take a paste from a web page the way
// the page showed it. A paste with no HTML, from another app or from a
// field, goes in as the Mac hands it.
export function pasteAsShown(field: HTMLTextAreaElement) {
  const paste = (e: ClipboardEvent) => {
    const html = e.clipboardData?.getData('text/html')
    if (!html) return
    const text = asShown(html)
    if (!text) return
    const plain = e.clipboardData?.getData('text/plain') ?? ''
    if (text === plain) return
    e.preventDefault()
    // insertText keeps the paste on the field's own undo, as the Mac's
    // paste does, and tells the field it changed.
    if (!document.execCommand('insertText', false, text)) {
      field.setRangeText(text, field.selectionStart, field.selectionEnd, 'end')
      field.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertFromPaste' }))
    }
    said(`paste: took the page's lines, ${lines(text)}, over its plain text's ${lines(plain)}`)
  }
  field.addEventListener('paste', paste)
  return { destroy: () => field.removeEventListener('paste', paste) }
}
