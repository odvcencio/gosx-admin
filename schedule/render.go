package schedule

import (
	"fmt"
	"strings"

	"m31labs.dev/gosx"
)

// RenderOptions controls the renderers.
type RenderOptions struct {
	// HeadingLevel is the level of the view title; day headings in the week
	// and agenda views use HeadingLevel+1. Default 2.
	HeadingLevel int
	// Labels default to EnglishLabels() field by field.
	Labels Labels
	// DayHref, EventHref, and PageHref return "" for no link. PageHref links
	// the previous and next pages.
	DayHref   func(Date) string
	EventHref func(Event) string
	PageHref  func(Date) string
	// CategoryLabel turns a Category into visible text. Nil hides it.
	CategoryLabel func(string) string
	// Occupancy supplies seat counts to show; nil hides them.
	Occupancy func(Event) (Occupancy, bool)
	// ID prefixes element ids. Default "gxa-cal".
	ID string
}

// RenderMonth renders a month grid:
//
//	<section class="gxa-cal gxa-cal--month" aria-labelledby="{id}-title">
//	  <hN id="{id}-title">February 2027</hN>
//	  <p class="gxa-cal__pager"><a rel="prev" href="…">Previous month</a> <a rel="next" href="…">Next month</a></p>
//	  <table class="gxa-cal__grid" role="table" aria-labelledby="{id}-title">
//	    <thead role="rowgroup"><tr role="row"><th role="columnheader" scope="col"><abbr title="Monday">Mon</abbr></th>…</tr></thead>
//	    <tbody role="rowgroup"><tr role="row">
//	      <td role="cell" class="gxa-cal__day [--outside] [--today] [--empty]">
//	        <p class="gxa-cal__date"><time datetime="2027-02-15" [aria-current="date"]>
//	          <span aria-hidden="true">15</span><span class="gxa-visually-hidden">Monday, February 15</span></time></p>
//	        <ul class="gxa-cal__events"><li class="gxa-cal__event [--cancelled]">
//	          <a href="…">Title</a> <span class="gxa-cal__time">All day</span>
//	          <span class="gxa-cal__status">Cancelled</span></li></ul>
//	        <p class="gxa-cal__more">+2 more</p>
//	      </td>…
//	    </tr></tbody>
//	  </table>
//	  <p class="gxa-cal__empty">No events.</p>   (only when !HasEvents)
//	</section>
//
// Below 40em the theme stacks days, hides empty and outside days, and shows
// each day's full label.
func RenderMonth(v MonthView, o RenderOptions) gosx.Node {
	labels := completeLabels(o.Labels)
	id, level := renderID(o.ID), headingLevel(o.HeadingLevel)
	titleID := id + "-title"
	nodes := []gosx.Node{heading(level, titleID, v.Title)}
	if pager, ok := renderPager(v.Prev, v.Next, labels.PrevMonth, labels.NextMonth, o.PageHref); ok {
		nodes = append(nodes, pager)
	}
	nodes = append(nodes, gosx.El("table", gosx.Attrs(gosx.Attr("class", "gxa-cal__grid"), gosx.Attr("role", "table"), gosx.Attr("aria-labelledby", titleID)),
		gosx.El("thead", gosx.Attrs(gosx.Attr("role", "rowgroup")), monthHeaders(v)),
		monthBody(v, labels, o)))
	if !v.HasEvents {
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-cal__empty")), gosx.Text(labels.Empty)))
	}
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-cal gxa-cal--month"), gosx.Attr("aria-labelledby", titleID)), gosx.Fragment(nodes...))
}

// RenderWeek renders <section class="gxa-cal gxa-cal--week"> with an
// <ol class="gxa-cal__days"> of days, each with an hN+1 day heading and a
// <ul> of events, or Labels.Empty for a day with none.
func RenderWeek(v WeekView, o RenderOptions) gosx.Node {
	labels := completeLabels(o.Labels)
	id, level := renderID(o.ID), headingLevel(o.HeadingLevel)
	titleID := id + "-title"
	nodes := []gosx.Node{heading(level, titleID, v.Title)}
	if pager, ok := renderPager(v.Prev, v.Next, labels.PrevWeek, labels.NextWeek, o.PageHref); ok {
		nodes = append(nodes, pager)
	}
	days := make([]gosx.Node, 0, len(v.Days))
	for i, day := range v.Days {
		days = append(days, renderListDay(day, fmt.Sprintf("%s-day-%d", id, i), level+1, labels, o, true))
	}
	nodes = append(nodes, gosx.El("ol", gosx.Attrs(gosx.Attr("class", "gxa-cal__days")), gosx.Fragment(days...)))
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-cal gxa-cal--week"), gosx.Attr("aria-labelledby", titleID)), gosx.Fragment(nodes...))
}

// RenderAgenda renders <section class="gxa-cal gxa-cal--agenda"> with an
// <ol class="gxa-cal__days"> of the days that have events, or one
// <p class="gxa-cal__empty"> when none do.
func RenderAgenda(v AgendaView, o RenderOptions) gosx.Node {
	labels := completeLabels(o.Labels)
	id, level := renderID(o.ID), headingLevel(o.HeadingLevel)
	titleID := id + "-title"
	nodes := []gosx.Node{heading(level, titleID, v.Title)}
	if len(v.Days) == 0 {
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-cal__empty")), gosx.Text(labels.Empty)))
	} else {
		days := make([]gosx.Node, 0, len(v.Days))
		for i, day := range v.Days {
			days = append(days, renderListDay(day, fmt.Sprintf("%s-day-%d", id, i), level+1, labels, o, false))
		}
		nodes = append(nodes, gosx.El("ol", gosx.Attrs(gosx.Attr("class", "gxa-cal__days")), gosx.Fragment(days...)))
	}
	return gosx.El("section", gosx.Attrs(gosx.Attr("class", "gxa-cal gxa-cal--agenda"), gosx.Attr("aria-labelledby", titleID)), gosx.Fragment(nodes...))
}

func monthHeaders(v MonthView) gosx.Node {
	cols := make([]gosx.Node, 0, len(v.Weekdays))
	for i, name := range v.Weekdays {
		short := name
		if i < len(v.WeekdaysShort) {
			short = v.WeekdaysShort[i]
		}
		cols = append(cols, gosx.El("th", gosx.Attrs(gosx.Attr("role", "columnheader"), gosx.Attr("scope", "col")),
			gosx.El("abbr", gosx.Attrs(gosx.Attr("title", name)), gosx.Text(short))))
	}
	return gosx.El("tr", gosx.Attrs(gosx.Attr("role", "row")), gosx.Fragment(cols...))
}

func monthBody(v MonthView, labels Labels, o RenderOptions) gosx.Node {
	rows := make([]gosx.Node, 0, len(v.Weeks))
	for _, week := range v.Weeks {
		cells := make([]gosx.Node, 0, len(week))
		for _, day := range week {
			cells = append(cells, renderMonthCell(day, labels, o))
		}
		rows = append(rows, gosx.El("tr", gosx.Attrs(gosx.Attr("role", "row")), gosx.Fragment(cells...)))
	}
	return gosx.El("tbody", gosx.Attrs(gosx.Attr("role", "rowgroup")), gosx.Fragment(rows...))
}

func renderMonthCell(day DayCell, labels Labels, o RenderOptions) gosx.Node {
	classes := []string{"gxa-cal__day"}
	if !day.InMonth {
		classes = append(classes, "gxa-cal__day--outside")
	}
	if day.Today {
		classes = append(classes, "gxa-cal__day--today")
	}
	if len(day.Items) == 0 {
		classes = append(classes, "gxa-cal__day--empty")
	}
	timeAttrs := []any{gosx.Attr("datetime", day.Date.String())}
	if day.Today {
		timeAttrs = append(timeAttrs, gosx.Attr("aria-current", "date"))
	}
	dayNum := gosx.Text(fmt.Sprint(day.Date.Day))
	if o.DayHref != nil {
		if href := o.DayHref(day.Date); href != "" {
			dayNum = gosx.El("a", gosx.Attrs(gosx.Attr("href", href), gosx.Attr("class", "gxa-cal__day-link")), dayNum)
		}
	}
	timeChildren := []gosx.Node{gosx.El("span", gosx.Attrs(gosx.Attr("aria-hidden", "true")), dayNum), gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-visually-hidden gxa-cal__full-label")), gosx.Text(day.Label))}
	date := gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-cal__date")), gosx.El("time", gosx.Attrs(timeAttrs...), gosx.Fragment(timeChildren...)))
	nodes := []gosx.Node{date}
	if len(day.Items) > 0 {
		nodes = append(nodes, eventList(day.Items, o))
	}
	if day.More > 0 {
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-cal__more")), gosx.Text(fmt.Sprintf(labels.MoreFormat, day.More))))
	}
	return gosx.El("td", gosx.Attrs(gosx.Attr("role", "cell"), gosx.Attr("class", strings.Join(classes, " "))), gosx.Fragment(nodes...))
}

func renderListDay(day DayCell, id string, level int, labels Labels, o RenderOptions, showEmpty bool) gosx.Node {
	if level > 6 {
		level = 6
	}
	headingID := id + "-title"
	timeAttrs := []any{gosx.Attr("datetime", day.Date.String())}
	if day.Today {
		timeAttrs = append(timeAttrs, gosx.Attr("aria-current", "date"))
	}
	label := gosx.El("time", gosx.Attrs(timeAttrs...), gosx.Text(day.Label))
	if o.DayHref != nil {
		if href := o.DayHref(day.Date); href != "" {
			label = gosx.El("a", gosx.Attrs(gosx.Attr("href", href), gosx.Attr("class", "gxa-cal__day-link")), label)
		}
	}
	nodes := []gosx.Node{gosx.El(fmt.Sprintf("h%d", level), gosx.Attrs(gosx.Attr("id", headingID)), label)}
	if len(day.Items) > 0 {
		nodes = append(nodes, eventList(day.Items, o))
	} else if showEmpty {
		nodes = append(nodes, gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-cal__empty-day")), gosx.Text(labels.Empty)))
	}
	classes := []string{"gxa-cal__day"}
	if day.Today {
		classes = append(classes, "gxa-cal__day--today")
	}
	if len(day.Items) == 0 {
		classes = append(classes, "gxa-cal__day--empty")
	}
	return gosx.El("li", gosx.Attrs(gosx.Attr("class", strings.Join(classes, " ")), gosx.Attr("aria-labelledby", headingID)), gosx.Fragment(nodes...))
}

func eventList(items []Occurrence, o RenderOptions) gosx.Node {
	entries := make([]gosx.Node, 0, len(items))
	for _, item := range items {
		e := item.Event
		classes := []string{"gxa-cal__event"}
		if e.Status == EventCancelled {
			classes = append(classes, "gxa-cal__event--cancelled")
		}
		if e.Status == EventDraft {
			classes = append(classes, "gxa-cal__event--draft")
		}
		parts := make([]gosx.Node, 0, 6)
		title := gosx.Text(e.Title)
		if o.EventHref != nil {
			if href := o.EventHref(e); href != "" {
				title = gosx.El("a", gosx.Attrs(gosx.Attr("href", href)), title)
			}
		}
		parts = append(parts, title)
		if item.TimeLabel != "" {
			parts = append(parts, gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-cal__time")), gosx.Text(item.TimeLabel)))
		}
		if item.StatusLabel != "" {
			parts = append(parts, gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-cal__status")), gosx.Text(item.StatusLabel)))
		}
		if o.CategoryLabel != nil {
			if category := o.CategoryLabel(e.Category); category != "" {
				parts = append(parts, gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-cal__category")), gosx.Text(category)))
			}
		}
		if o.Occupancy != nil {
			if occupancy, ok := o.Occupancy(e); ok {
				parts = append(parts, occupancyNode(occupancy))
			}
		}
		entries = append(entries, gosx.El("li", gosx.Attrs(gosx.Attr("class", strings.Join(classes, " "))), gosx.Fragment(parts...)))
	}
	return gosx.El("ul", gosx.Attrs(gosx.Attr("class", "gxa-cal__events")), gosx.Fragment(entries...))
}

func occupancyNode(o Occupancy) gosx.Node {
	text := fmt.Sprintf("%d", o.Used)
	label := fmt.Sprintf("%d seats used", o.Used)
	if !o.Unlimited {
		text = fmt.Sprintf("%d / %d", o.Used, o.Capacity)
		label = fmt.Sprintf("%d of %d seats used", o.Used, o.Capacity)
	}
	return gosx.El("span", gosx.Attrs(gosx.Attr("class", "gxa-cal__occupancy"), gosx.Attr("aria-label", label)), gosx.Text(text))
}

func renderPager(prev, next Date, prevLabel, nextLabel string, href func(Date) string) (gosx.Node, bool) {
	if href == nil {
		return gosx.Node{}, false
	}
	nodes := make([]gosx.Node, 0, 2)
	if url := href(prev); url != "" {
		nodes = append(nodes, gosx.El("a", gosx.Attrs(gosx.Attr("rel", "prev"), gosx.Attr("href", url)), gosx.Text(prevLabel)))
	}
	if url := href(next); url != "" {
		nodes = append(nodes, gosx.El("a", gosx.Attrs(gosx.Attr("rel", "next"), gosx.Attr("href", url)), gosx.Text(nextLabel)))
	}
	if len(nodes) == 0 {
		return gosx.Node{}, false
	}
	return gosx.El("p", gosx.Attrs(gosx.Attr("class", "gxa-cal__pager")), gosx.Fragment(nodes...)), true
}

func heading(level int, id, title string) gosx.Node {
	return gosx.El(fmt.Sprintf("h%d", level), gosx.Attrs(gosx.Attr("id", id)), gosx.Text(title))
}

func headingLevel(level int) int {
	if level < 1 || level > 6 {
		return 2
	}
	return level
}

func renderID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "gxa-cal"
	}
	return id
}
