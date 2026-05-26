package calendar

import (
	"fmt"
	"strings"
	"time"

	"m31labs.dev/gosx"
)

type RenderOptions struct {
	Class         string
	EmptyText     string
	AddHref       string
	AddLabel      string
	WeekdayFormat string
	ShowCapacity  bool
	DayHref       func(time.Time) string
	EventHref     func(Event) string
}

func RenderMonth(view MonthView, options RenderOptions) gosx.Node {
	className := strings.TrimSpace(options.Class)
	if className == "" {
		className = "calendar-widget"
	}
	addLabel := strings.TrimSpace(options.AddLabel)
	if addLabel == "" {
		addLabel = "New event"
	}
	header := []gosx.Node{
		gosx.El("h2", gosx.Attrs(gosx.Attr("class", className+"__title")), gosx.Text(view.Label)),
	}
	if strings.TrimSpace(options.AddHref) != "" {
		header = append(header, gosx.El("a", gosx.Attrs(
			gosx.Attr("class", className+"__add"),
			gosx.Attr("href", options.AddHref),
		), gosx.Text(addLabel)))
	}
	children := []gosx.Node{
		gosx.El("div", gosx.Attrs(gosx.Attr("class", className+"__header")), gosx.Fragment(header...)),
		renderWeekdays(className, view.Start, options),
	}
	weeks := make([]gosx.Node, 0, len(view.Weeks))
	for _, week := range view.Weeks {
		days := make([]gosx.Node, 0, len(week.Days))
		for _, day := range week.Days {
			days = append(days, renderDay(className, day, options))
		}
		weeks = append(weeks, gosx.El("div", gosx.Attrs(gosx.Attr("class", className+"__week")), gosx.Fragment(days...)))
	}
	children = append(children, gosx.El("div", gosx.Attrs(gosx.Attr("class", className+"__grid")), gosx.Fragment(weeks...)))
	if !view.HasEvents {
		emptyText := strings.TrimSpace(options.EmptyText)
		if emptyText == "" {
			emptyText = "No events scheduled."
		}
		children = append(children, gosx.El("p", gosx.Attrs(gosx.Attr("class", className+"__empty")), gosx.Text(emptyText)))
	}
	return gosx.El("section", gosx.Attrs(
		gosx.Attr("class", className),
		gosx.Attr("data-calendar-start", view.Start.Format("2006-01-02")),
		gosx.Attr("data-calendar-end", view.End.Format("2006-01-02")),
	), gosx.Fragment(children...))
}

func renderWeekdays(className string, start time.Time, options RenderOptions) gosx.Node {
	format := strings.TrimSpace(options.WeekdayFormat)
	if format == "" {
		format = "Mon"
	}
	labels := make([]gosx.Node, 0, 7)
	for i := 0; i < 7; i++ {
		day := start.AddDate(0, 0, i)
		labels = append(labels, gosx.El("span", gosx.Attrs(
			gosx.Attr("class", className+"__weekday"),
		), gosx.Text(day.Format(format))))
	}
	return gosx.El("div", gosx.Attrs(gosx.Attr("class", className+"__weekdays")), gosx.Fragment(labels...))
}

func renderDay(className string, day DayView, options RenderOptions) gosx.Node {
	classes := []string{className + "__day"}
	if !day.InMonth {
		classes = append(classes, className+"__day--outside")
	}
	if day.Today {
		classes = append(classes, className+"__day--today")
	}
	if day.HasEvents {
		classes = append(classes, className+"__day--has-events")
	}
	children := []gosx.Node{renderDayLabel(className, day, options)}
	if len(day.Events) > 0 {
		items := make([]gosx.Node, 0, len(day.Events))
		for _, event := range day.Events {
			items = append(items, renderEvent(className, event, options))
		}
		children = append(children, gosx.El("ol", gosx.Attrs(gosx.Attr("class", className+"__events")), gosx.Fragment(items...)))
	}
	if day.OverflowCount > 0 {
		children = append(children, gosx.El("span", gosx.Attrs(
			gosx.Attr("class", className+"__overflow"),
		), gosx.Text(fmt.Sprintf("+%d more", day.OverflowCount))))
	}
	return gosx.El("article", gosx.Attrs(
		gosx.Attr("class", strings.Join(classes, " ")),
		gosx.Attr("data-date", day.Date.Format("2006-01-02")),
	), gosx.Fragment(children...))
}

func renderDayLabel(className string, day DayView, options RenderOptions) gosx.Node {
	attrs := []any{
		gosx.Attr("class", className+"__date"),
		gosx.Attr("datetime", day.Date.Format("2006-01-02")),
	}
	label := gosx.El("time", gosx.Attrs(attrs...), gosx.Text(day.Label))
	if options.DayHref == nil {
		return label
	}
	if href := strings.TrimSpace(options.DayHref(day.Date)); href != "" {
		return gosx.El("a", gosx.Attrs(
			gosx.Attr("class", className+"__date-link"),
			gosx.Attr("href", href),
		), label)
	}
	return label
}

func renderEvent(className string, event Event, options RenderOptions) gosx.Node {
	status := statusToken(event.Status)
	classes := className + "__event " + className + "__event--" + status
	attrs := []any{
		gosx.Attr("class", classes),
		gosx.Attr("data-event-id", event.ID),
		gosx.Attr("data-status", status),
		gosx.Attr("data-resource-kind", event.ResourceKind),
		gosx.Attr("data-resource-id", event.ResourceID),
	}
	if color := colorStyle(event.Color); color != "" {
		attrs = append(attrs, gosx.Attr("style", color))
	}
	children := []gosx.Node{
		gosx.El("span", gosx.Attrs(gosx.Attr("class", className+"__event-title")), gosx.Text(event.Title)),
	}
	meta := EventTimeLabel(event)
	if event.Location != "" {
		if meta != "" {
			meta += " - "
		}
		meta += event.Location
	}
	if options.ShowCapacity {
		if capacity := CapacityLabel(event); capacity != "" {
			if meta != "" {
				meta += " - "
			}
			meta += capacity
		}
	}
	if meta != "" {
		children = append(children, gosx.El("span", gosx.Attrs(gosx.Attr("class", className+"__event-meta")), gosx.Text(meta)))
	}
	body := gosx.Fragment(children...)
	href := strings.TrimSpace(event.Href)
	if options.EventHref != nil {
		if override := strings.TrimSpace(options.EventHref(event)); override != "" {
			href = override
		}
	}
	if href != "" {
		body = gosx.El("a", gosx.Attrs(gosx.Attr("href", href)), body)
	}
	return gosx.El("li", gosx.Attrs(attrs...), body)
}

func EventTimeLabel(event Event) string {
	if event.AllDay {
		return "All day"
	}
	if event.Start.IsZero() {
		return ""
	}
	startTime, endTime := eventDisplayTimes(event)
	start := startTime.Format("3:04 PM")
	if event.End.IsZero() || event.End.Equal(event.Start) {
		return start
	}
	return start + "-" + endTime.Format("3:04 PM")
}

func CapacityLabel(event Event) string {
	if event.Capacity <= 0 {
		return ""
	}
	if event.Registered < 0 {
		event.Registered = 0
	}
	return fmt.Sprintf("%d/%d", event.Registered, event.Capacity)
}

func eventDisplayTimes(event Event) (time.Time, time.Time) {
	start := event.Start
	end := event.End
	if strings.TrimSpace(event.Timezone) == "" {
		return start, end
	}
	location, err := time.LoadLocation(strings.TrimSpace(event.Timezone))
	if err != nil {
		return start, end
	}
	return start.In(location), end.In(location)
}

func statusToken(status Status) string {
	value := strings.TrimSpace(string(status))
	if value == "" {
		return string(StatusScheduled)
	}
	var out strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
			continue
		}
		if r == ' ' {
			out.WriteByte('-')
		}
	}
	if out.Len() == 0 {
		return string(StatusScheduled)
	}
	return out.String()
}

func colorStyle(color string) string {
	color = strings.TrimSpace(color)
	if color == "" || !strings.HasPrefix(color, "#") {
		return ""
	}
	hex := strings.TrimPrefix(color, "#")
	switch len(hex) {
	case 3, 4, 6, 8:
	default:
		return ""
	}
	for _, r := range hex {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return ""
		}
	}
	return "--calendar-event-color: " + color
}
