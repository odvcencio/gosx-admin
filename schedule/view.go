package schedule

import (
	"fmt"
	"sort"
	"time"
)

// Labels is the visible text of the views. EnglishLabels returns the
// defaults; a consumer supplies other languages.
type Labels struct {
	Months        [12]string // January first
	Weekdays      [7]string  // Sunday first
	WeekdaysShort [7]string  // Sunday first
	AllDay        string     // "All day"
	Cancelled     string     // "Cancelled"
	Draft         string     // "Draft"
	// MoreFormat is a fmt format with one %d, for example "+%d more".
	MoreFormat string
	Empty      string // "No events."
	PrevMonth  string // "Previous month"
	NextMonth  string // "Next month"
	PrevWeek   string // "Previous week"
	NextWeek   string // "Next week"
	// TimeFormat is a Go time layout. Default "3:04 PM".
	TimeFormat string
	// DayLabel formats a full day label. Nil uses "Monday, February 15".
	DayLabel func(Date) string
	// MonthTitle formats a month title. Nil uses "February 2027".
	MonthTitle func(year int, month time.Month) string
}

// EnglishLabels returns the default English labels.
func EnglishLabels() Labels {
	return Labels{
		Months:        [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		Weekdays:      [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
		WeekdaysShort: [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
		AllDay:        "All day", Cancelled: "Cancelled", Draft: "Draft", MoreFormat: "+%d more", Empty: "No events.",
		PrevMonth: "Previous month", NextMonth: "Next month", PrevWeek: "Previous week", NextWeek: "Next week", TimeFormat: "3:04 PM",
	}
}

// ViewOptions controls the view builders.
type ViewOptions struct {
	// Zone is the grid's zone for placing timed events on days. Nil is UTC.
	Zone *time.Location
	// Today marks the current day.
	Today     Date
	WeekStart time.Weekday
	// Limit caps events per day in the month view; 0 means no cap.
	Limit int
	// Labels default to EnglishLabels() field by field.
	Labels Labels
}

// Occurrence is one event on one day.
type Occurrence struct {
	Event Event
	Day   Date
	// First and Last mark the first and last day of a multi-day event.
	First bool
	Last  bool
	// TimeLabel is Labels.AllDay, or "10:00 AM–11:00 AM" in the event's own
	// zone, followed by the zone abbreviation when it differs from
	// ViewOptions.Zone at that instant.
	TimeLabel string
	// StatusLabel is "" for published events, else Labels.Cancelled or
	// Labels.Draft.
	StatusLabel string
}

// DayCell is one day in a view.
type DayCell struct {
	Date Date
	// Label is Labels.DayLabel(Date).
	Label   string
	InMonth bool
	Today   bool
	// Items are ordered all-day first, then by Start, then Title, then ID.
	Items []Occurrence
	// More counts items beyond ViewOptions.Limit.
	More int
}

// MonthView is a six-week grid around one month.
type MonthView struct {
	Title string
	// Month is the first day of the month.
	Month Date
	// Weekdays and WeekdaysShort are the column headers in grid order.
	Weekdays      []string
	WeekdaysShort []string
	// Weeks holds 6 rows of 7 days, starting on WeekStart.
	Weeks     [][]DayCell
	Prev      Date
	Next      Date
	HasEvents bool
}

// WeekView is seven days starting on WeekStart.
type WeekView struct {
	Title     string
	Start     Date
	Days      []DayCell
	Prev      Date
	Next      Date
	HasEvents bool
}

// AgendaView lists the days in [From, To) that have events.
type AgendaView struct {
	Title     string
	From      Date
	To        Date
	Days      []DayCell
	HasEvents bool
}

// Month builds the month view that contains month. An all-day event appears
// on each date from StartDate to LastDay. A timed event appears on each
// grid-zone date its [Start, End) overlaps.
func Month(events []Event, month Date, opts ViewOptions) MonthView {
	labels := completeLabels(opts.Labels)
	if !validDate(month) {
		now := DateOf(time.Now(), opts.Zone)
		month = Date{Year: now.Year, Month: now.Month, Day: 1}
	}
	month = Date{Year: month.Year, Month: month.Month, Day: 1}
	weekStart := normalizeWeekStart(opts.WeekStart)
	start := firstWeekDay(month, weekStart)
	weeks := make([][]DayCell, 6)
	hasEvents := false
	weekday, short := weekdayLabels(labels, weekStart)
	for wi := 0; wi < 6; wi++ {
		weeks[wi] = make([]DayCell, 7)
		for di := 0; di < 7; di++ {
			d := start.AddDays(wi*7 + di)
			cell := makeDayCell(d, opts, labels)
			cell.InMonth = d.Year == month.Year && d.Month == month.Month
			cell.Items = occurrencesForDay(events, d, opts, labels)
			if len(cell.Items) > 0 {
				hasEvents = true
			}
			if opts.Limit > 0 && len(cell.Items) > opts.Limit {
				cell.More = len(cell.Items) - opts.Limit
				cell.Items = cell.Items[:opts.Limit]
			}
			weeks[wi][di] = cell
		}
	}
	return MonthView{Title: monthTitle(month.Year, month.Month, labels), Month: month, Weekdays: weekday, WeekdaysShort: short,
		Weeks: weeks, Prev: addMonths(month, -1),
		Next: addMonths(month, 1), HasEvents: hasEvents}
}

// Week builds the week that contains day.
func Week(events []Event, day Date, opts ViewOptions) WeekView {
	labels := completeLabels(opts.Labels)
	if !validDate(day) {
		day = DateOf(time.Now(), opts.Zone)
	}
	start := firstWeekDay(day, normalizeWeekStart(opts.WeekStart))
	days := make([]DayCell, 7)
	hasEvents := false
	for i := range days {
		d := start.AddDays(i)
		days[i] = makeDayCell(d, opts, labels)
		days[i].Items = occurrencesForDay(events, d, opts, labels)
		if len(days[i].Items) > 0 {
			hasEvents = true
		}
	}
	return WeekView{Title: dateRangeTitle(start, start.AddDays(6), labels), Start: start, Days: days,
		Prev: start.AddDays(-7), Next: start.AddDays(7), HasEvents: hasEvents}
}

// Agenda builds the agenda for [from, to).
func Agenda(events []Event, from, to Date, opts ViewOptions) AgendaView {
	labels := completeLabels(opts.Labels)
	if !validDate(from) || !validDate(to) || !from.Before(to) {
		return AgendaView{Title: "", From: from, To: to}
	}
	byDay := make(map[Date][]Occurrence)
	for d := from; d.Before(to); d = d.AddDays(1) {
		byDay[d] = occurrencesForDay(events, d, opts, labels)
	}
	days := make([]DayCell, 0)
	for d := from; d.Before(to); d = d.AddDays(1) {
		items := byDay[d]
		if len(items) == 0 {
			continue
		}
		cell := makeDayCell(d, opts, labels)
		cell.Items = items
		days = append(days, cell)
	}
	title := dateRangeTitle(from, to.AddDays(-1), labels)
	return AgendaView{Title: title, From: from, To: to, Days: days, HasEvents: len(days) != 0}
}

func makeDayCell(d Date, opts ViewOptions, labels Labels) DayCell {
	return DayCell{Date: d, Label: labels.DayLabel(d), Today: !opts.Today.IsZero() && d == opts.Today}
}

func occurrencesForDay(events []Event, day Date, opts ViewOptions, labels Labels) []Occurrence {
	gridZone := opts.Zone
	if gridZone == nil {
		gridZone = time.UTC
	}
	dayStart := day.Start(gridZone)
	dayEnd := day.AddDays(1).Start(gridZone)
	items := make([]Occurrence, 0)
	for _, event := range events {
		var first, last bool
		var timeLabel string
		if event.AllDay {
			lastDay := event.LastDay()
			if event.StartDate.IsZero() || lastDay.IsZero() || day.Before(event.StartDate) || day.After(lastDay) {
				continue
			}
			first, last = day == event.StartDate, day == lastDay
			timeLabel = labels.AllDay
		} else {
			if event.Start.IsZero() || event.End.IsZero() || !event.Start.Before(dayEnd) || !event.End.After(dayStart) {
				continue
			}
			first, last = DateOf(event.Start, gridZone) == day, DateOf(event.End.Add(-time.Nanosecond), gridZone) == day
			timeLabel = occurrenceTimeLabel(event, gridZone, labels)
		}
		status := ""
		if event.Status == EventCancelled {
			status = labels.Cancelled
		} else if event.Status == EventDraft {
			status = labels.Draft
		}
		items = append(items, Occurrence{Event: cloneEvent(event), Day: day, First: first, Last: last, TimeLabel: timeLabel, StatusLabel: status})
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].Event, items[j].Event
		if a.AllDay != b.AllDay {
			return a.AllDay
		}
		if !a.Start.Equal(b.Start) {
			return a.Start.Before(b.Start)
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
	return items
}

func occurrenceTimeLabel(event Event, gridZone *time.Location, labels Labels) string {
	loc, err := event.TimeZone()
	if err != nil {
		loc = time.UTC
	}
	start, end := event.Start.In(loc), event.End.In(loc)
	format := labels.TimeFormat
	label := start.Format(format) + "–" + end.Format(format)
	_, eventOffset := start.Zone()
	_, gridOffset := event.Start.In(gridZone).Zone()
	if eventOffset != gridOffset {
		abbr, _ := start.Zone()
		if abbr != "" {
			label += " " + abbr
		}
	}
	return label
}

func completeLabels(in Labels) Labels {
	defaults := EnglishLabels()
	for i := range in.Months {
		if in.Months[i] == "" {
			in.Months[i] = defaults.Months[i]
		}
	}
	for i := range in.Weekdays {
		if in.Weekdays[i] == "" {
			in.Weekdays[i] = defaults.Weekdays[i]
		}
	}
	for i := range in.WeekdaysShort {
		if in.WeekdaysShort[i] == "" {
			in.WeekdaysShort[i] = defaults.WeekdaysShort[i]
		}
	}
	if in.AllDay == "" {
		in.AllDay = defaults.AllDay
	}
	if in.Cancelled == "" {
		in.Cancelled = defaults.Cancelled
	}
	if in.Draft == "" {
		in.Draft = defaults.Draft
	}
	if in.MoreFormat == "" {
		in.MoreFormat = defaults.MoreFormat
	}
	if in.Empty == "" {
		in.Empty = defaults.Empty
	}
	if in.PrevMonth == "" {
		in.PrevMonth = defaults.PrevMonth
	}
	if in.NextMonth == "" {
		in.NextMonth = defaults.NextMonth
	}
	if in.PrevWeek == "" {
		in.PrevWeek = defaults.PrevWeek
	}
	if in.NextWeek == "" {
		in.NextWeek = defaults.NextWeek
	}
	if in.TimeFormat == "" {
		in.TimeFormat = defaults.TimeFormat
	}
	if in.DayLabel == nil {
		in.DayLabel = func(d Date) string {
			return fmt.Sprintf("%s, %s %d", in.Weekdays[int(d.Weekday())], in.Months[int(d.Month)-1], d.Day)
		}
	}
	if in.MonthTitle == nil {
		in.MonthTitle = func(y int, m time.Month) string { return fmt.Sprintf("%s %d", in.Months[int(m)-1], y) }
	}
	return in
}

func weekdayLabels(labels Labels, start time.Weekday) ([]string, []string) {
	full, short := make([]string, 7), make([]string, 7)
	for i := 0; i < 7; i++ {
		idx := (int(start) + i) % 7
		full[i], short[i] = labels.Weekdays[idx], labels.WeekdaysShort[idx]
	}
	return full, short
}

func firstWeekDay(day Date, start time.Weekday) Date {
	start = normalizeWeekStart(start)
	back := (int(day.Weekday()) - int(start) + 7) % 7
	return day.AddDays(-back)
}

func normalizeWeekStart(start time.Weekday) time.Weekday {
	if start < time.Sunday || start > time.Saturday {
		return time.Sunday
	}
	return start
}

func addMonths(month Date, n int) Date {
	t := time.Date(month.Year, month.Month+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	return Date{Year: t.Year(), Month: t.Month(), Day: 1}
}

func monthTitle(year int, month time.Month, labels Labels) string {
	if month < time.January || month > time.December {
		return ""
	}
	return labels.MonthTitle(year, month)
}

func dateRangeTitle(from, to Date, labels Labels) string {
	if from.IsZero() || to.IsZero() {
		return ""
	}
	if from.Year == to.Year && from.Month == to.Month {
		return fmt.Sprintf("%s %d–%d, %d", labels.Months[int(from.Month)-1], from.Day, to.Day, from.Year)
	}
	if from.Year == to.Year {
		return fmt.Sprintf("%s %d–%s %d, %d", labels.Months[int(from.Month)-1], from.Day, labels.Months[int(to.Month)-1], to.Day, from.Year)
	}
	return fmt.Sprintf("%s %d, %d–%s %d, %d", labels.Months[int(from.Month)-1], from.Day, from.Year, labels.Months[int(to.Month)-1], to.Day, to.Year)
}
