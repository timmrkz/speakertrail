// Text pasted from a web page, the way the page showed it.
//
// A browser puts a copy on the clipboard twice, as HTML and as plain
// text, and a text field takes the plain text. Chrome's plain text ends
// every paragraph with an empty line. LinkedIn's message field writes
// each line as a paragraph and an empty line as an empty paragraph, so a
// message copied there came out in Notes with three empty lines where it
// had one (Tim, 23 November 2026, #83). A page that writes its empty
// lines as empty paragraphs draws no gap under the others, or its empty
// lines would show twice as tall. So from such a page a field with
// pasteAsShown reads the HTML: a paragraph is a line, and an empty
// paragraph is an empty line. A paste from any other page, or with no
// page at all, goes in as Chrome's plain text has it, as before.

import { said } from './said'

// The elements that start a line of their own.
const blocks = new Set([
  'ADDRESS', 'ARTICLE', 'ASIDE', 'BLOCKQUOTE', 'DD', 'DIV', 'DL', 'DT', 'FIELDSET', 'FIGURE', 'FOOTER', 'FORM',
  'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'HEADER', 'HR', 'LI', 'MAIN', 'NAV', 'OL', 'P', 'PRE', 'SECTION', 'TABLE', 'TR', 'UL',
])
// What a page holds that is never shown.
const unseen = new Set(['SCRIPT', 'STYLE', 'TEMPLATE', 'HEAD', 'TITLE', 'META', 'LINK', 'NOSCRIPT'])

// A paragraph without words, a line of its own that shows empty.
function empty(e: Element): boolean {
  return !(e.textContent ?? '').replace(/[\s ]/g, '')
}

// asShown is the text of html with its lines as the page drew them, or
// null for a page that does not write an empty line as an empty
// paragraph, whose plain text is right as it is. What the page holds is
// untrusted: it is parsed, never put on the page, and only its text is
// read.
export function asShown(html: string): string | null {
  const body = new DOMParser().parseFromString(html, 'text/html').body
  if (![...body.querySelectorAll('p, div')].some((e) => empty(e) && !e.querySelector('p, div'))) return null

  // The text in pieces, joined once at the end, so a long paste takes as
  // long as it is long. No piece is empty, and a line break is a piece
  // of its own.
  const parts: string[] = []
  const last = () => parts[parts.length - 1] ?? ''
  const trimEnd = () => {
    while (parts.length && last().endsWith(' ')) {
      const t = last().replace(/ +$/, '')
      if (t) parts[parts.length - 1] = t
      else parts.pop()
    }
  }
  // Ends the line, unless it has just ended.
  const newline = () => {
    trimEnd()
    if (parts.length && !last().endsWith('\n')) parts.push('\n')
  }
  const walk = (n: Node, pre: boolean) => {
    if (n.nodeType === Node.TEXT_NODE) {
      let t = (n.textContent ?? '').replace(/ /g, ' ')
      if (!pre) {
        t = t.replace(/[ \t\n\r\f]+/g, ' ')
        if (!parts.length || last().endsWith('\n') || last().endsWith(' ')) t = t.replace(/^ /, '')
      }
      if (t) parts.push(t)
      return
    }
    if (!(n instanceof HTMLElement) || unseen.has(n.tagName)) return
    if (n.tagName === 'BR') {
      trimEnd()
      parts.push('\n')
      return
    }
    const ws = n.style.whiteSpace
    const keep = n.tagName === 'PRE' || (ws ? ws.startsWith('pre') || ws === 'break-spaces' : pre)
    // Cells of a row stand apart by a tab, as in Chrome's plain text.
    if ((n.tagName === 'TD' || n.tagName === 'TH') && n.previousElementSibling) parts.push('\t')
    const block = blocks.has(n.tagName)
    if (block) newline()
    const start = parts.length
    for (const c of n.childNodes) walk(c, keep)
    if (!block) return
    // A line break last in a paragraph with words draws no line of its
    // own. It is there so the paragraph keeps its height while it is
    // being written.
    if (n.lastChild?.nodeName === 'BR' && parts.length - start >= 2 && last() === '\n') parts.pop()
    newline()
  }
  walk(body, false)
  return parts.join('').replace(/^\n+|\n+$/g, '')
}

// How many lines a text has, for the app's log, which never says the
// text itself.
const lines = (t: string) => t.split('\n').length

// pasteAsShown makes a text field take a paste from a page that writes
// empty lines as empty paragraphs the way the page showed it. Every
// other paste goes in as the Mac hands it.
export function pasteAsShown(field: HTMLTextAreaElement) {
  const paste = (e: ClipboardEvent) => {
    if (field.readOnly || field.disabled) return
    const plain = e.clipboardData?.getData('text/plain') ?? ''
    const html = e.clipboardData?.getData('text/html') ?? ''
    const text = html ? asShown(html) : null
    if (!text || text === plain) {
      said(`paste: took the plain text, ${lines(plain)} lines, ${html ? 'the page writes no empty paragraphs' : 'no page came with it'}`)
      return
    }
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
