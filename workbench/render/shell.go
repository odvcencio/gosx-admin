package render

import (
	"net/http"
	"sort"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/server"
)

// MainID is the id of the shell's main element and the skip link target.
const MainID = "main"

// StatusMessage is the result of the last action, shown once in the shell's
// status region.
type StatusMessage struct {
	Text string
	// Error renders the text with role="alert" instead of role="status".
	Error bool
}

// StatusFor returns the first flashed action message for r in action-name
// order. It reads the same action flash as ReadFeedback. Error is true when
// the result is not OK.
func StatusFor(r *http.Request) StatusMessage {
	states := action.States(r)
	names := make([]string, 0, len(states))
	for name := range states {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		feedback, ok := ReadFeedback(r, name)
		if ok && feedback.Message != "" {
			return StatusMessage{Text: feedback.Message, Error: !feedback.OK}
		}
	}
	return StatusMessage{}
}

// ShellProps configures Shell and Parts.
type ShellProps struct {
	// Brand is the product name or wordmark, linked to BrandHref.
	Brand     gosx.Node
	BrandHref string
	// Account is optional header content, such as the operator's role and a
	// SignOut form.
	Account gosx.Node
	Nav     Navigation
	// NavLabel names the desktop navigation landmark. Default "Admin".
	NavLabel string
	// PhoneNavLabel names the phone bar landmark. Default "Admin shortcuts".
	PhoneNavLabel string
	// SkipLabel is the skip link text. Default "Skip to content".
	SkipLabel string
	// Footer is optional contentinfo content.
	Footer gosx.Node
	Status StatusMessage
	// Class is added to the root element's class list.
	Class string
}

// ShellParts holds the shell pieces for layouts that write <main> themselves,
// such as a .gsx layout with a slot. Such a layout renders, in order:
// SkipLink, Header, DesktopNav, then <main id="main" class="gxa-main"
// tabindex="-1" data-gosx-main> with Status before page content, then Footer,
// then PhoneNav.
type ShellParts struct {
	SkipLink   gosx.Node
	Header     gosx.Node
	DesktopNav gosx.Node
	Status     gosx.Node
	Footer     gosx.Node
	PhoneNav   gosx.Node
}

// Shell renders the admin frame around content:
//
//	<div class="gxa-shell">
//	  <a class="gxa-skip" href="#main">Skip to content</a>
//	  <header class="gxa-header">…brand, account…</header>
//	  <nav class="gxa-nav" aria-label="Admin">…Items…</nav>
//	  <main id="main" class="gxa-main" tabindex="-1" data-gosx-main>
//	    <div class="gxa-status" data-gosx-toast-host>…Status…</div>
//	    …content…
//	  </main>
//	  <footer class="gxa-footer">…</footer>
//	  <nav class="gxa-phonebar" aria-label="Admin shortcuts">…Bar…</nav>
//	</div>
//
// It omits the header account slot, footer, or phone bar when they are empty.
func Shell(props ShellProps, content gosx.Node) gosx.Node {
	parts := Parts(props)
	attrs := gosx.Attrs(gosx.Attr("class", classNames("gxa-shell", props.Class)))
	return gosx.El("div", attrs,
		parts.SkipLink,
		parts.Header,
		parts.DesktopNav,
		gosx.El("main", gosx.Attrs(gosx.Attr("id", MainID), gosx.Attr("class", "gxa-main"), gosx.Attr("tabindex", "-1"), gosx.BoolAttr("data-gosx-main")), parts.Status, content),
		parts.Footer,
		parts.PhoneNav,
	)
}

// Parts returns the same pieces Shell renders, without the root and main.
func Parts(props ShellProps) ShellParts {
	skip := props.SkipLabel
	if skip == "" {
		skip = "Skip to content"
	}
	navLabel := props.NavLabel
	if navLabel == "" {
		navLabel = "Admin"
	}
	phoneLabel := props.PhoneNavLabel
	if phoneLabel == "" {
		phoneLabel = "Admin shortcuts"
	}
	brand := server.Link(props.BrandHref, gosx.Attrs(gosx.Attr("class", "gxa-brand")), props.Brand)
	headerChildren := []gosx.Node{brand}
	if !nodeEmpty(props.Account) {
		headerChildren = append(headerChildren, gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-account")), props.Account))
	}
	header := gosx.El("header", gosx.Attrs(gosx.Attr("class", "gxa-header")), gosx.Fragment(headerChildren...))

	links := make([]gosx.Node, 0, len(props.Nav.Items))
	for _, item := range props.Nav.Items {
		links = append(links, gosx.El("li", nil, navLink(item, "gxa-nav__link")))
	}
	desktopNav := gosx.El("nav", gosx.Attrs(gosx.Attr("class", "gxa-nav"), gosx.Attr("aria-label", navLabel)),
		gosx.El("ul", gosx.Attrs(gosx.Attr("class", "gxa-nav__list")), gosx.Fragment(links...)))

	statusChildren := []gosx.Node(nil)
	if props.Status.Text != "" {
		role := "status"
		if props.Status.Error {
			role = "alert"
		}
		statusChildren = append(statusChildren, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-status__message"), gosx.Attr("role", role)), gosx.Text(props.Status.Text)))
	}
	status := gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-status"), gosx.BoolAttr("data-gosx-toast-host")), gosx.Fragment(statusChildren...))

	footer := gosx.Fragment()
	if !nodeEmpty(props.Footer) {
		footer = gosx.El("footer", gosx.Attrs(gosx.Attr("class", "gxa-footer")), props.Footer)
	}
	phoneNav := gosx.Fragment()
	if len(props.Nav.Bar) > 0 {
		links := make([]gosx.Node, 0, len(props.Nav.Bar))
		for _, item := range props.Nav.Bar {
			links = append(links, gosx.El("li", nil, navLink(item, "gxa-phonebar__link")))
		}
		phoneNav = gosx.El("nav", gosx.Attrs(gosx.Attr("class", "gxa-phonebar"), gosx.Attr("aria-label", phoneLabel)),
			gosx.El("ul", gosx.Attrs(gosx.Attr("class", "gxa-phonebar__list")), gosx.Fragment(links...)))
	}
	return ShellParts{
		SkipLink:   server.Link("#"+MainID, gosx.Attrs(gosx.Attr("class", "gxa-skip")), gosx.Text(skip)),
		Header:     header,
		DesktopNav: desktopNav,
		Status:     status,
		Footer:     footer,
		PhoneNav:   phoneNav,
	}
}

// MoreList renders nav.More as a list of links for the More page. The page
// supplies its own h1.
func MoreList(nav Navigation) gosx.Node {
	links := make([]gosx.Node, 0, len(nav.More))
	for _, item := range nav.More {
		links = append(links, gosx.El("li", nil, navLink(item, "gxa-more__link")))
	}
	return gosx.El("ul", gosx.Attrs(gosx.Attr("class", "gxa-more")), gosx.Fragment(links...))
}

// SignOut renders a native POST form with the CSRF field and one submit
// button. The default label is "Sign out".
func SignOut(url, csrf, label string) gosx.Node {
	if label == "" {
		label = "Sign out"
	}
	return server.Form(gosx.Attrs(gosx.Attr("class", "gxa-signout"), gosx.Attr("method", "post"), gosx.Attr("action", url)),
		hiddenInput(CSRFField, csrf),
		gosx.El("button", gosx.Attrs(gosx.Attr("class", "gxa-button gxa-button--secondary"), gosx.Attr("type", "submit")), gosx.Text(label)),
	)
}

func nodeEmpty(n gosx.Node) bool {
	got := gosx.RenderHTML(n)
	return got == "" || got == "<></>"
}

func classNames(names ...string) string {
	var out string
	for _, name := range names {
		if name == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += name
	}
	return out
}
