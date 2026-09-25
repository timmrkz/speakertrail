package fetch

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// VisibleText returns the text a reader sees on an HTML page, one block per
// line.
func VisibleText(doc string) string {
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Noscript, atom.Template, atom.Svg, atom.Head, atom.Iframe:
				return
			}
			if hidden(n) {
				return
			}
		}
		if n.Type == html.TextNode {
			if t := strings.Join(strings.Fields(n.Data), " "); t != "" {
				if b.Len() > 0 {
					last := b.String()[b.Len()-1]
					if last != '\n' && last != ' ' {
						b.WriteByte(' ')
					}
				}
				b.WriteString(t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && isBlock(n.DataAtom) && b.Len() > 0 && b.String()[b.Len()-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	walk(root)
	return strings.TrimSpace(b.String())
}

func hidden(n *html.Node) bool {
	for _, a := range n.Attr {
		switch a.Key {
		case "hidden":
			return true
		case "aria-hidden":
			return a.Val == "true"
		case "style":
			s := strings.ReplaceAll(strings.ToLower(a.Val), " ", "")
			if strings.Contains(s, "display:none") || strings.Contains(s, "visibility:hidden") {
				return true
			}
		}
	}
	return false
}

func isBlock(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Section, atom.Article, atom.Header, atom.Footer, atom.Main, atom.Aside, atom.Nav,
		atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Li, atom.Ul, atom.Ol, atom.Tr, atom.Table,
		atom.Br, atom.Dd, atom.Dt, atom.Dl, atom.Blockquote, atom.Pre, atom.Figure, atom.Figcaption, atom.Form,
		atom.Address:
		return true
	}
	return false
}
