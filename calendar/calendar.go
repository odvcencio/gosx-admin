package calendar

import (
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusScheduled Status = "scheduled"
	StatusOpen      Status = "open"
	StatusFull      Status = "full"
	StatusCancelled Status = "cancelled"
)

type Event struct {
	ID           string
	Title        string
	Description  string
	Start        time.Time
	End          time.Time
	AllDay       bool
	Timezone     string
	Location     string
	ResourceKind string
	ResourceID   string
	Status       Status
	Color        string
	Capacity     int
	Registered   int
	Href         string
}

type Filter struct {
	From         time.Time
	To           time.Time
	ResourceKind string
	ResourceID   string
	Statuses     []Status
}

type Store interface {
	ListEvents(Filter) []Event
}

type Options struct {
	Month     time.Time
	Today     time.Time
	WeekStart time.Weekday
	Limit     int
}

type MonthView struct {
	Label     string
	Year      int
	Month     time.Month
	Start     time.Time
	End       time.Time
	Weeks     []WeekView
	HasEvents bool
}

type WeekView struct {
	Days []DayView
}

type DayView struct {
	Date          time.Time
	Label         string
	InMonth       bool
	Today         bool
	Events        []Event
	EventCount    int
	OverflowCount int
	HasEvents     bool
}

func NormalizeEvent(event Event) (Event, bool) {
	event.ID = strings.TrimSpace(event.ID)
	event.Title = strings.TrimSpace(event.Title)
	event.Description = strings.TrimSpace(event.Description)
	event.Timezone = strings.TrimSpace(event.Timezone)
	event.Location = strings.TrimSpace(event.Location)
	event.ResourceKind = strings.TrimSpace(event.ResourceKind)
	event.ResourceID = strings.TrimSpace(event.ResourceID)
	event.Color = strings.TrimSpace(event.Color)
	event.Href = strings.TrimSpace(event.Href)
	if event.Title == "" || event.Start.IsZero() {
		return Event{}, false
	}
	if event.End.IsZero() || event.End.Before(event.Start) {
		event.End = event.Start
	}
	if event.Status == "" {
		event.Status = StatusScheduled
	}
	if event.Capacity < 0 {
		event.Capacity = 0
	}
	if event.Registered < 0 {
		event.Registered = 0
	}
	return event, true
}

func NormalizeEvents(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, event := range events {
		if normalized, ok := NormalizeEvent(event); ok {
			out = append(out, normalized)
		}
	}
	sortEvents(out)
	return out
}

func FilterEvents(events []Event, filter Filter) []Event {
	statuses := map[Status]bool{}
	for _, status := range filter.Statuses {
		if status != "" {
			statuses[status] = true
		}
	}
	out := make([]Event, 0, len(events))
	for _, event := range NormalizeEvents(events) {
		if !filter.From.IsZero() && event.End.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && !event.Start.Before(filter.To) {
			continue
		}
		if strings.TrimSpace(filter.ResourceKind) != "" && event.ResourceKind != strings.TrimSpace(filter.ResourceKind) {
			continue
		}
		if strings.TrimSpace(filter.ResourceID) != "" && event.ResourceID != strings.TrimSpace(filter.ResourceID) {
			continue
		}
		if len(statuses) > 0 && !statuses[event.Status] {
			continue
		}
		out = append(out, event)
	}
	return out
}

func Month(events []Event, options Options) MonthView {
	month := firstOfMonth(options.Month)
	if month.IsZero() {
		month = firstOfMonth(time.Now())
	}
	today := dateOnly(options.Today)
	weekStart := options.WeekStart
	gridStart := startOfWeek(month, weekStart)
	gridEnd := gridStart.AddDate(0, 0, 42)
	filtered := FilterEvents(events, Filter{From: gridStart, To: gridEnd})
	eventsByDay := map[string][]Event{}
	for _, event := range filtered {
		key := dayKey(event.Start)
		eventsByDay[key] = append(eventsByDay[key], event)
	}
	weeks := make([]WeekView, 0, 6)
	hasEvents := false
	for week := 0; week < 6; week++ {
		days := make([]DayView, 0, 7)
		for day := 0; day < 7; day++ {
			date := gridStart.AddDate(0, 0, week*7+day)
			dayEvents := append([]Event(nil), eventsByDay[dayKey(date)]...)
			sortEvents(dayEvents)
			eventCount := len(dayEvents)
			if eventCount > 0 {
				hasEvents = true
			}
			overflow := 0
			if options.Limit > 0 && eventCount > options.Limit {
				overflow = eventCount - options.Limit
				dayEvents = dayEvents[:options.Limit]
			}
			days = append(days, DayView{
				Date:          date,
				Label:         date.Format("2"),
				InMonth:       date.Month() == month.Month() && date.Year() == month.Year(),
				Today:         !today.IsZero() && sameDay(date, today),
				Events:        dayEvents,
				EventCount:    eventCount,
				OverflowCount: overflow,
				HasEvents:     eventCount > 0,
			})
		}
		weeks = append(weeks, WeekView{Days: days})
	}
	return MonthView{
		Label:     month.Format("January 2006"),
		Year:      month.Year(),
		Month:     month.Month(),
		Start:     gridStart,
		End:       gridEnd,
		Weeks:     weeks,
		HasEvents: hasEvents,
	}
}

func sortEvents(events []Event) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Start.Equal(events[j].Start) {
			return events[i].Title < events[j].Title
		}
		return events[i].Start.Before(events[j].Start)
	})
}

func firstOfMonth(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, value.Location())
}

func startOfWeek(value time.Time, weekStart time.Weekday) time.Time {
	value = dateOnly(value)
	for value.Weekday() != weekStart {
		value = value.AddDate(0, 0, -1)
	}
	return value
}

func dateOnly(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func sameDay(a, b time.Time) bool {
	a = dateOnly(a)
	b = dateOnly(b)
	return a.Equal(b)
}

func dayKey(value time.Time) string {
	return dateOnly(value).Format("2006-01-02")
}
