package render

import (
	"context"
	"strings"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/workbench"
	"m31labs.dev/gosx/server"
)

// MoreSlug is the slug of the synthetic More item in Navigation.Bar.
const MoreSlug = "gxa-more"

// Current says how a navigation item relates to the page being shown.
type Current int

const (
	// CurrentNone means the item is not the current page or section.
	CurrentNone Current = iota
	// CurrentPage means the item's Href equals the current path. It renders
	// as aria-current="page".
	CurrentPage
	// CurrentSection means the current path is below the item's Href, or the
	// current page appears in Navigation.More. It renders as aria-current="true".
	CurrentSection
)

// Badge is a count shown beside a navigation label.
type Badge struct {
	// Count hides the badge when it is zero or less.
	Count int
	// Label is announced after the item label. The default is "{Count} new".
	Label string
}

// NavItem is one destination in the shell.
type NavItem struct {
	// Slug is a workbench resource or tool slug, or MoreSlug.
	Slug  string
	Label string
	// Href is the resource or tool route, or NavOptions.MoreHref.
	Href    string
	Badge   Badge
	Current Current
}

// NavOptions controls BuildNavigation.
type NavOptions struct {
	// CurrentPath is the request path without query or fragment.
	CurrentPath string
	// Order lists slugs in display order. Unknown slugs are ignored. Readable
	// destinations missing from Order follow workspace order: tools first,
	// then resources.
	Order []string
	// Phone lists slugs to prefer in the phone bar, in order.
	Phone []string
	// Badges maps a slug, or MoreSlug, to its badge.
	Badges map[string]Badge
	// MoreHref is required when more than four destinations are readable.
	MoreHref string
	// MoreLabel names the More item. The default is "More".
	MoreLabel string
	// Access filters destinations through Access.CanRead.
	Access Access
}

// Navigation is the request-scoped navigation model for the shell.
type Navigation struct {
	// Items holds every readable destination in display order. Desktop
	// navigation renders Items.
	Items []NavItem
	// Bar holds at most four items for the phone bar.
	Bar []NavItem
	// More holds Items not in Bar, in Items order. MoreList renders it.
	More []NavItem
}

// BuildNavigation turns a workspace into request-scoped navigation.
//
//  1. Each tool and resource with a non-empty Route that Access.CanRead allows
//     becomes an item. NavOptions.Order sets display order; readable
//     destinations missing from Order follow in workspace order, tools first.
//  2. The current item is the one whose Href equals CurrentPath or is a
//     path-segment prefix of it (Href + "/"), choosing the longest Href. "/"
//     matches only itself. It gets CurrentPage on an exact match and
//     CurrentSection otherwise.
//  3. With four items or fewer, Bar holds them all and More is empty. Above
//     four, Bar holds three items selected first from Phone and then from
//     Items order, plus a More item using MoreHref. More holds the remaining
//     items in Items order. The More item is CurrentPage at MoreHref and
//     CurrentSection when one of its More items is current.
func BuildNavigation(ctx context.Context, ws workbench.Workspace, opts NavOptions) Navigation {
	bySlug := make(map[string]NavItem, len(ws.Tools)+len(ws.Resources))
	var workspaceOrder []string
	for _, tool := range ws.Tools {
		if tool.Route == "" || !opts.Access.CanRead(ctx, tool.Slug) {
			continue
		}
		if _, exists := bySlug[tool.Slug]; exists {
			continue
		}
		bySlug[tool.Slug] = NavItem{Slug: tool.Slug, Label: tool.Label, Href: tool.Route, Badge: opts.Badges[tool.Slug]}
		workspaceOrder = append(workspaceOrder, tool.Slug)
	}
	for _, resource := range ws.Resources {
		if resource.Route == "" || !opts.Access.CanRead(ctx, resource.Slug) {
			continue
		}
		if _, exists := bySlug[resource.Slug]; exists {
			continue
		}
		bySlug[resource.Slug] = NavItem{Slug: resource.Slug, Label: resource.Label, Href: resource.Route, Badge: opts.Badges[resource.Slug]}
		workspaceOrder = append(workspaceOrder, resource.Slug)
	}

	items := make([]NavItem, 0, len(bySlug))
	used := make(map[string]bool, len(bySlug))
	for _, slug := range opts.Order {
		if item, ok := bySlug[slug]; ok && !used[slug] {
			items = append(items, item)
			used[slug] = true
		}
	}
	for _, slug := range workspaceOrder {
		if !used[slug] {
			items = append(items, bySlug[slug])
			used[slug] = true
		}
	}

	currentIndex, currentLen := -1, -1
	for i := range items {
		if pathMatches(items[i].Href, opts.CurrentPath) && len(items[i].Href) > currentLen {
			currentIndex, currentLen = i, len(items[i].Href)
		}
	}
	if currentIndex >= 0 {
		if items[currentIndex].Href == opts.CurrentPath {
			items[currentIndex].Current = CurrentPage
		} else {
			items[currentIndex].Current = CurrentSection
		}
	}

	nav := Navigation{Items: items}
	if len(items) <= 4 {
		nav.Bar = append(nav.Bar, items...)
		return nav
	}

	selected := make(map[string]bool, 3)
	for _, slug := range opts.Phone {
		if len(nav.Bar) == 3 {
			break
		}
		if item, ok := findNavItem(items, slug); ok && !selected[slug] {
			nav.Bar = append(nav.Bar, item)
			selected[slug] = true
		}
	}
	for _, item := range items {
		if len(nav.Bar) == 3 {
			break
		}
		if !selected[item.Slug] {
			nav.Bar = append(nav.Bar, item)
			selected[item.Slug] = true
		}
	}
	for _, item := range items {
		if !selected[item.Slug] {
			nav.More = append(nav.More, item)
		}
	}
	moreLabel := opts.MoreLabel
	if moreLabel == "" {
		moreLabel = "More"
	}
	more := NavItem{Slug: MoreSlug, Label: moreLabel, Href: opts.MoreHref, Badge: opts.Badges[MoreSlug]}
	if opts.CurrentPath == opts.MoreHref && opts.MoreHref != "" {
		more.Current = CurrentPage
	} else {
		for _, item := range nav.More {
			if item.Current != CurrentNone {
				more.Current = CurrentSection
				break
			}
		}
	}
	nav.Bar = append(nav.Bar, more)
	return nav
}

func pathMatches(href, path string) bool {
	if href == "/" {
		return path == "/"
	}
	return path == href || strings.HasPrefix(path, href+"/")
}

func findNavItem(items []NavItem, slug string) (NavItem, bool) {
	for _, item := range items {
		if item.Slug == slug {
			return item, true
		}
	}
	return NavItem{}, false
}

func navLink(item NavItem, class string) gosx.Node {
	attrs := gosx.Attrs(gosx.Attr("class", class))
	if item.Current == CurrentPage {
		attrs = append(attrs, gosx.Attr("aria-current", "page"))
	} else if item.Current == CurrentSection {
		attrs = append(attrs, gosx.Attr("aria-current", "true"))
	}
	nodes := []gosx.Node{gosx.Text(item.Label)}
	if item.Badge.Count > 0 {
		label := item.Badge.Label
		if label == "" {
			label = strings.TrimSpace(strings.Join([]string{itoa(item.Badge.Count), "new"}, " "))
		}
		nodes = append(nodes, gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-badge")),
			gosx.El("span", gosx.Attrs(gosx.Attr("aria-hidden", "true")), gosx.Text(itoa(item.Badge.Count))),
			gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-visually-hidden")), gosx.Text(", "+label)),
		))
	}
	return server.Link(item.Href, append([]any{attrs}, gosx.Fragment(nodes...))...)
}

func nodesToAny(nodes []gosx.Node) []any {
	out := make([]any, len(nodes))
	for i := range nodes {
		out[i] = nodes[i]
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
