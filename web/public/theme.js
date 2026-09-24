// Applies a remembered light or dark choice before the page paints. It is
// a file, not an inline script, so the strict Content-Security-Policy holds.
try {
  var t = localStorage.getItem('st-theme')
  if (t === 'light' || t === 'dark') document.documentElement.dataset.theme = t
} catch (e) {}
