//go:build e2e

package schedule_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/schedule"
	"m31labs.dev/gosx-admin/theme"
)

func TestCalendarResponsiveE2E(t *testing.T) {
	monthDate := schedule.Date{Year: 2027, Month: time.February, Day: 1}
	today := schedule.Date{Year: 2027, Month: time.February, Day: 15}
	event := fixtureEvent("long-title")
	event.AllDay = true
	event.StartDate, event.EndDate = today, today.AddDays(2)
	event.Title = "A long event title that must wrap inside a narrow calendar cell"
	events := []schedule.Event{event}
	viewOptions := schedule.ViewOptions{Today: today, WeekStart: time.Monday, Zone: time.UTC}
	pages := map[string]string{
		"/month":  gosx.RenderHTML(schedule.RenderMonth(schedule.Month(events, monthDate, viewOptions), schedule.RenderOptions{PageHref: func(d schedule.Date) string { return "/month?date=" + d.String() }})),
		"/week":   gosx.RenderHTML(schedule.RenderWeek(schedule.Week(events, today, viewOptions), schedule.RenderOptions{PageHref: func(d schedule.Date) string { return "/week?date=" + d.String() }})),
		"/agenda": gosx.RenderHTML(schedule.RenderAgenda(schedule.Agenda(events, today, today.AddDays(7), viewOptions), schedule.RenderOptions{})),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		markup, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width, initial-scale=1\"><style>html{font-size:200%%}body{margin:0}</style><style>%s</style></head><body>%s</body></html>", theme.Stylesheet(), markup)
	}))
	defer server.Close()

	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), chromedp.Headless, chromedp.NoSandbox, chromedp.DisableGPU, chromedp.ExecPath("/usr/bin/google-chrome"), chromedp.Flag("disable-dev-shm-usage", true))
	defer cancelAllocator()
	ctx, cancelBrowser := chromedp.NewContext(allocator)
	defer cancelBrowser()
	ctx, cancelTimeout := context.WithTimeout(ctx, 45*time.Second)
	defer cancelTimeout()
	for _, path := range []string{"/month", "/week", "/agenda"} {
		t.Run(path, func(t *testing.T) {
			var result struct {
				Viewport  int    `json:"viewport"`
				Document  int    `json:"document"`
				RootFont  string `json:"rootFont"`
				FullLabel string `json:"fullLabel"`
				Outside   string `json:"outside"`
				Offenders string `json:"offenders"`
				Narrow    bool   `json:"narrow"`
				Table     string `json:"table"`
			}
			err := chromedp.Run(ctx,
				chromedp.EmulateViewport(320, 900),
				chromedp.Navigate(server.URL+path),
				chromedp.Evaluate(`(() => { const full = document.querySelector('.gxa-cal__full-label'); const outside = document.querySelector('.gxa-cal__day--outside'); const table = document.querySelector('.gxa-cal__grid'); const offenders = [...document.querySelectorAll('*')].filter(e => e.getBoundingClientRect().right > innerWidth + 1).map(e => e.tagName.toLowerCase() + '.' + e.className + ':' + Math.round(e.getBoundingClientRect().right)); return {viewport: window.innerWidth, document: document.documentElement.scrollWidth, rootFont: getComputedStyle(document.documentElement).fontSize, fullLabel: full ? getComputedStyle(full).position : '', outside: outside ? getComputedStyle(outside).display : '', offenders: offenders.join(','), narrow: matchMedia('(max-width: 40em)').matches, table: table ? getComputedStyle(table).display : ''}; })()`, &result),
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Viewport != 320 || result.Document > result.Viewport {
				t.Fatalf("horizontal overflow at 320px: viewport=%d document=%d offenders=%s narrow=%t table=%s", result.Viewport, result.Document, result.Offenders, result.Narrow, result.Table)
			}
			if result.RootFont != "32px" {
				t.Fatalf("root font size = %q, want 32px (200%%)", result.RootFont)
			}
			if path == "/month" && result.FullLabel != "static" {
				t.Fatalf("month full date label position = %q, narrow=%t, table=%s", result.FullLabel, result.Narrow, result.Table)
			}
			if path == "/month" && result.Outside != "none" {
				t.Fatalf("outside month cell display = %q", result.Outside)
			}
		})
	}
}
