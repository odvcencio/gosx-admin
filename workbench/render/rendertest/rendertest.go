// Package rendertest checks rendered HTML against the accessibility contract
// used by workbench/render.
package rendertest

import (
	"strings"
	"testing"

	xhtml "golang.org/x/net/html"
)

// Problem is one contract violation. Rule is the rule name, for example
// "one-h1". Detail says where and what, for example `input name="title" has
// no label`.
type Problem struct {
	Rule   string
	Detail string
}

// Options selects the rules to apply.
type Options struct {
	// Page treats the HTML as a whole page. It adds one-main, one-h1,
	// skip-link (the first link targets the main element's id), and landmarks
	// (at most one banner header and one contentinfo footer outside sectioning
	// content).
	Page bool
}

type locatedProblem struct {
	index int
	seq   int
	value Problem
}

// Check parses doc with golang.org/x/net/html and returns every problem in
// document order. Rules always applied:
//
//   - nav-name: every nav has aria-label or aria-labelledby, and names are unique;
//   - unique-id: no two elements share an id;
//   - label: every input (except hidden, submit, and button), select, and
//     textarea has a label[for], a wrapping label, aria-label, or
//     aria-labelledby;
//   - button-name and link-name: every button and a[href] has text or
//     aria-label;
//   - describedby: every aria-describedby and aria-labelledby id exists;
//   - img-alt: every img has an alt attribute;
//   - heading-order: no heading skips a level going deeper;
//   - table-headers: every table has a th;
//   - no-h1: when Page is false, the fragment has no h1.
//
// When Page is true, it also checks one-main, one-h1, a first-link skip target,
// and at most one banner header and one contentinfo footer outside sectioning
// content.
func Check(doc string, opts Options) []Problem {
	root, err := xhtml.Parse(strings.NewReader(doc))
	if err != nil {
		return []Problem{{Rule: "parse", Detail: err.Error()}}
	}
	var elements []*xhtml.Node
	order := map[*xhtml.Node]int{}
	ancestors := map[*xhtml.Node]*xhtml.Node{}
	var collect func(*xhtml.Node, *xhtml.Node)
	collect = func(n, up *xhtml.Node) {
		if up != nil {
			ancestors[n] = up
		}
		if n.Type == xhtml.ElementNode {
			order[n] = len(elements)
			elements = append(elements, n)
		}
		for node := n.FirstChild; node != nil; node = node.NextSibling {
			collect(node, n)
		}
	}
	collect(root, nil)

	ids := make(map[string]*xhtml.Node)
	var problems []locatedProblem
	seq := 0
	add := func(n *xhtml.Node, rule, detail string) {
		index := 0
		if n != nil {
			index = order[n]
		}
		problems = append(problems, locatedProblem{index: index, seq: seq, value: Problem{Rule: rule, Detail: detail}})
		seq++
	}
	for _, n := range elements {
		if id := attr(n, "id"); id != "" {
			if _, ok := ids[id]; ok {
				add(n, "unique-id", `id="`+id+`" appears more than once`)
			} else {
				ids[id] = n
			}
		}
	}

	navNames := map[string]bool{}
	var headings []*xhtml.Node
	var mains, h1s, bannerHeaders, contentFooters []*xhtml.Node
	for _, n := range elements {
		switch n.Data {
		case "nav":
			name := strings.TrimSpace(attr(n, "aria-label"))
			if name == "" {
				name = labelledName(n, ids)
			}
			if name == "" {
				add(n, "nav-name", "nav has no accessible name")
			} else if navNames[name] {
				add(n, "nav-name", `nav name "`+name+`" is not unique`)
			} else {
				navNames[name] = true
			}
		case "input", "select", "textarea":
			if !(n.Data == "input" && isUnlabeledInputType(attr(n, "type"))) && !hasLabel(n, elements, ids, ancestors) {
				add(n, "label", n.Data+` name="`+attr(n, "name")+`" has no label`)
			}
		case "button":
			if accessibleName(n, ids) == "" {
				add(n, "button-name", "button has no accessible name")
			}
		case "a":
			if hasAttr(n, "href") && accessibleName(n, ids) == "" {
				add(n, "link-name", `a[href="`+attr(n, "href")+`"] has no accessible name`)
			}
		case "img":
			if !hasAttr(n, "alt") {
				add(n, "img-alt", "img has no alt attribute")
			}
		case "table":
			if !hasTableHeader(n) {
				add(n, "table-headers", "table has no th")
			}
		case "main":
			mains = append(mains, n)
		case "h1":
			h1s = append(h1s, n)
		}
		if isHeading(n.Data) {
			headings = append(headings, n)
		}
		if hasAttr(n, "aria-describedby") {
			checkReferences(n, "aria-describedby", ids, add)
		}
		if hasAttr(n, "aria-labelledby") {
			checkReferences(n, "aria-labelledby", ids, add)
		}
		if opts.Page && outsideSectioning(n, ancestors) {
			if attr(n, "role") == "banner" || n.Data == "header" && attr(n, "role") == "" {
				bannerHeaders = append(bannerHeaders, n)
			}
			if attr(n, "role") == "contentinfo" || n.Data == "footer" && attr(n, "role") == "" {
				contentFooters = append(contentFooters, n)
			}
		}
	}

	previousLevel := 0
	for _, n := range headings {
		level := int(n.Data[1] - '0')
		if previousLevel > 0 && level > previousLevel+1 {
			add(n, "heading-order", "heading level skips from h"+string(rune('0'+previousLevel))+" to h"+string(rune('0'+level)))
		}
		previousLevel = level
	}
	if !opts.Page {
		for _, n := range h1s {
			add(n, "no-h1", "fragment contains an h1")
		}
	}
	if opts.Page {
		if len(mains) == 0 {
			add(root, "one-main", "page has no main element")
		} else if len(mains) > 1 {
			for _, n := range mains[1:] {
				add(n, "one-main", "page has more than one main element")
			}
		}
		if len(h1s) == 0 {
			add(root, "one-h1", "page has no h1")
		} else if len(h1s) > 1 {
			for _, n := range h1s[1:] {
				add(n, "one-h1", "page has more than one h1")
			}
		}
		firstLink := firstElement(elements, func(n *xhtml.Node) bool { return n.Data == "a" && hasAttr(n, "href") })
		if firstLink == nil {
			add(root, "skip-link", "page has no link before its main content")
		} else if len(mains) == 0 || attr(firstLink, "href") != "#"+attr(mains[0], "id") || attr(mains[0], "id") == "" {
			add(firstLink, "skip-link", "first link does not target the main element")
		}
		if len(bannerHeaders) > 1 {
			for _, n := range bannerHeaders[1:] {
				add(n, "landmarks", "page has more than one banner header")
			}
		}
		if len(contentFooters) > 1 {
			for _, n := range contentFooters[1:] {
				add(n, "landmarks", "page has more than one contentinfo footer")
			}
		}
	}

	sortProblems(problems)
	out := make([]Problem, len(problems))
	for i, p := range problems {
		out[i] = p.value
	}
	return out
}

// AssertAccessible fails t with one error per Problem.
func AssertAccessible(t testing.TB, doc string, opts Options) {
	t.Helper()
	for _, problem := range Check(doc, opts) {
		t.Errorf("accessibility rule %s: %s", problem.Rule, problem.Detail)
	}
}

func sortProblems(problems []locatedProblem) {
	for i := 1; i < len(problems); i++ {
		for j := i; j > 0 && (problems[j].index < problems[j-1].index || problems[j].index == problems[j-1].index && problems[j].seq < problems[j-1].seq); j-- {
			problems[j], problems[j-1] = problems[j-1], problems[j]
		}
	}
}

func attr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *xhtml.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func isUnlabeledInputType(kind string) bool {
	switch strings.ToLower(kind) {
	case "hidden", "submit", "button":
		return true
	default:
		return false
	}
}

func hasLabel(control *xhtml.Node, elements []*xhtml.Node, ids map[string]*xhtml.Node, ancestors map[*xhtml.Node]*xhtml.Node) bool {
	if strings.TrimSpace(attr(control, "aria-label")) != "" || labelledName(control, ids) != "" {
		return true
	}
	for node := control; node != nil; node = ancestors[node] {
		if node.Type == xhtml.ElementNode && node.Data == "label" {
			return true
		}
	}
	id := attr(control, "id")
	if id == "" {
		return false
	}
	for _, n := range elements {
		if n.Data == "label" && attr(n, "for") == id {
			return true
		}
	}
	return false
}

func accessibleName(n *xhtml.Node, ids map[string]*xhtml.Node) string {
	if label := strings.TrimSpace(attr(n, "aria-label")); label != "" {
		return label
	}
	if name := labelledName(n, ids); name != "" {
		return name
	}
	return strings.TrimSpace(textContent(n))
}

func labelledName(n *xhtml.Node, ids map[string]*xhtml.Node) string {
	var b strings.Builder
	for _, id := range strings.Fields(attr(n, "aria-labelledby")) {
		if target := ids[id]; target != nil {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(strings.TrimSpace(textContent(target)))
		}
	}
	return strings.TrimSpace(b.String())
}

func checkReferences(n *xhtml.Node, name string, ids map[string]*xhtml.Node, add func(*xhtml.Node, string, string)) {
	for _, id := range strings.Fields(attr(n, name)) {
		if ids[id] == nil {
			add(n, "describedby", name+` references missing id="`+id+`"`)
		}
	}
}

func textContent(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.TextNode {
			b.WriteString(node.Data)
		}
		for node := node.FirstChild; node != nil; node = node.NextSibling {
			walk(node)
		}
	}
	walk(n)
	return b.String()
}

func isHeading(name string) bool {
	return len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6'
}

func hasTableHeader(table *xhtml.Node) bool {
	var walk func(*xhtml.Node) bool
	walk = func(n *xhtml.Node) bool {
		for node := n.FirstChild; node != nil; node = node.NextSibling {
			if node.Type == xhtml.ElementNode && node.Data == "th" {
				return true
			}
			if node.Type == xhtml.ElementNode && node.Data == "table" {
				continue
			}
			if walk(node) {
				return true
			}
		}
		return false
	}
	return walk(table)
}

func outsideSectioning(n *xhtml.Node, ancestors map[*xhtml.Node]*xhtml.Node) bool {
	for node := n; node != nil; node = ancestors[node] {
		if node.Type == xhtml.ElementNode {
			switch node.Data {
			case "article", "aside", "nav", "section":
				return false
			}
		}
	}
	return true
}

func firstElement(nodes []*xhtml.Node, match func(*xhtml.Node) bool) *xhtml.Node {
	for _, n := range nodes {
		if match(n) {
			return n
		}
	}
	return nil
}
