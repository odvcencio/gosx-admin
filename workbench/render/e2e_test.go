//go:build e2e

package render

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/theme"
	"m31labs.dev/gosx-admin/workbench"
	"m31labs.dev/gosx/action"
	runtimehost "m31labs.dev/gosx/client/runtime/host"
	"m31labs.dev/gosx/session"
)

func TestE2EReflow320At200Text(t *testing.T) {
	for _, page := range []string{"/list", "/detail", "/form"} {
		t.Run(strings.TrimPrefix(page, "/"), func(t *testing.T) {
			withE2EPage(t, page, 320, 800, func(ctx context.Context, _ string) {
				var ok bool
				err := chromedp.Run(ctx,
					chromedp.Evaluate(`document.documentElement.style.fontSize = "200%"`, nil),
					chromedp.Evaluate(`document.documentElement.scrollWidth <= document.documentElement.clientWidth && document.body.scrollWidth <= document.body.clientWidth`, &ok),
				)
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					t.Fatalf("horizontal overflow at 320 px with 200%% text")
				}
			})
		})
	}
}

func TestE2EFocusVisible(t *testing.T) {
	withE2EPage(t, "/form", 1440, 900, func(ctx context.Context, _ string) {
		var count int
		if err := chromedp.Run(ctx, chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href],button:not([disabled]),summary,input:not([type="hidden"]):not([disabled]),select:not([disabled]),textarea:not([disabled])')).filter(e => e.getClientRects().length).length`, &count)); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < count; i++ {
			var state struct {
				Tag   string `json:"tag"`
				Text  string `json:"text"`
				Width string `json:"width"`
				Style string `json:"style"`
			}
			if err := chromedp.Run(ctx, chromedp.KeyEvent(kb.Tab), chromedp.Evaluate(`(() => { const e=document.activeElement; const s=getComputedStyle(e); return {tag:e.tagName,text:(e.innerText||e.getAttribute('aria-label')||e.getAttribute('name')||'').trim(),width:s.outlineWidth,style:s.outlineStyle}; })()`, &state)); err != nil {
				t.Fatal(err)
			}
			width, _ := strconv.ParseFloat(strings.TrimSuffix(state.Width, "px"), 64)
			if state.Tag == "BODY" || width < 3 || state.Style == "none" {
				t.Fatalf("tab stop %d (%s %q) has no visible 3 px focus ring: width=%s style=%s", i, state.Tag, state.Text, state.Width, state.Style)
			}
		}
	})
}

func TestE2ETargetsAtLeast44px(t *testing.T) {
	for _, page := range []string{"/list", "/detail", "/form"} {
		t.Run(strings.TrimPrefix(page, "/"), func(t *testing.T) {
			withE2EPage(t, page, 390, 844, func(ctx context.Context, _ string) {
				var targets []struct {
					Tag    string  `json:"tag"`
					Text   string  `json:"text"`
					Height float64 `json:"height"`
				}
				if err := chromedp.Run(ctx, chromedp.Evaluate(`Array.from(document.querySelectorAll('button,summary,.gxa-nav a,.gxa-phonebar a,input:not([type="hidden"]),select,textarea')).filter(e => e.getClientRects().length).map(e => ({tag:e.tagName,text:(e.innerText||e.getAttribute('aria-label')||e.getAttribute('name')||'').trim(),height:e.getBoundingClientRect().height}))`, &targets)); err != nil {
					t.Fatal(err)
				}
				if len(targets) == 0 {
					t.Fatal("sample page had no target controls")
				}
				for _, target := range targets {
					if target.Height < 44 {
						t.Errorf("%s %q is %.1f px high", target.Tag, target.Text, target.Height)
					}
				}
			})
		})
	}
}

func TestE2EFocusNotObscuredByPhoneBar(t *testing.T) {
	withE2EPage(t, "/form", 320, 800, func(ctx context.Context, _ string) {
		var count int
		if err := chromedp.Run(ctx, chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href],button:not([disabled]),summary,input:not([type="hidden"]):not([disabled]),select:not([disabled]),textarea:not([disabled])')).filter(e => e.getClientRects().length).length`, &count)); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < count; i++ {
			var state struct {
				Tag      string  `json:"tag"`
				InBar    bool    `json:"inBar"`
				Bottom   float64 `json:"bottom"`
				BarTop   float64 `json:"barTop"`
				BarFound bool    `json:"barFound"`
			}
			if err := chromedp.Run(ctx, chromedp.KeyEvent(kb.Tab), chromedp.Evaluate(`(() => { const e=document.activeElement, b=document.querySelector('.gxa-phonebar'); return {tag:e.tagName,inBar:!!e.closest('.gxa-phonebar'),bottom:e.getBoundingClientRect().bottom,barTop:b?b.getBoundingClientRect().top:0,barFound:!!b}; })()`, &state)); err != nil {
				t.Fatal(err)
			}
			if !state.BarFound {
				t.Fatal("phone bar is missing")
			}
			if !state.InBar && state.Bottom > state.BarTop+0.5 {
				t.Fatalf("focused %s ends at %.1f px, under phone bar at %.1f px", state.Tag, state.Bottom, state.BarTop)
			}
		}
	})
}

func TestE2EPhoneBarAtMostFour(t *testing.T) {
	withE2EPage(t, "/list", 320, 800, func(ctx context.Context, _ string) {
		var result struct {
			Count        int    `json:"count"`
			NavDisplay   string `json:"navDisplay"`
			PhoneDisplay string `json:"phoneDisplay"`
		}
		if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => ({count:document.querySelectorAll('.gxa-phonebar a').length,navDisplay:getComputedStyle(document.querySelector('.gxa-nav')).display,phoneDisplay:getComputedStyle(document.querySelector('.gxa-phonebar')).display}))()`, &result)); err != nil {
			t.Fatal(err)
		}
		if result.Count > 4 || result.NavDisplay != "none" || result.PhoneDisplay == "none" {
			t.Fatalf("phone navigation state = %#v", result)
		}
	})
}

func TestE2EManagedConflictShowsView(t *testing.T) {
	withE2EPage(t, "/form", 390, 844, func(ctx context.Context, _ string) {
		if err := chromedp.Run(ctx, chromedp.Clear(`#gxa-items-save-1-title`), chromedp.SendKeys(`#gxa-items-save-1-title`, "Typed title")); err != nil {
			t.Fatalf("edit form: %v", err)
		}
		if err := chromedp.Run(ctx, chromedp.Click(`.gxa-form button[type="submit"]`)); err != nil {
			t.Fatalf("submit form: %v", err)
		}
		if err := chromedp.Run(ctx, chromedp.WaitVisible(`.gxa-conflict`)); err != nil {
			t.Fatalf("wait for conflict view: %v", err)
		}
		var result struct {
			Conflict  string `json:"conflict"`
			Value     string `json:"value"`
			Current   string `json:"current"`
			URL       string `json:"url"`
			FormState string `json:"formState"`
		}
		if err := chromedp.Run(ctx, chromedp.Evaluate(`({conflict:document.querySelector('.gxa-conflict h2')?.textContent||'',value:document.querySelector('#gxa-items-save-1-title')?.value||'',current:document.querySelector('.gxa-conflict')?.textContent||'',url:location.href,formState:document.querySelector('form')?.getAttribute('data-gosx-form-state')||''})`, &result)); err != nil {
			t.Fatal(err)
		}
		if result.Conflict != "This record changed" || result.Value != "Typed title" || !strings.Contains(result.Current, "Saved title") {
			t.Fatalf("managed conflict view = %#v", result)
		}
	})
}

func withE2EPage(t *testing.T, path string, width, height int, run func(context.Context, string)) {
	t.Helper()
	serverURL := e2eWorkspaceServer(t)
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), chromedp.ExecPath("/usr/bin/google-chrome"), chromedp.NoSandbox,
		chromedp.Flag("headless", true), chromedp.Flag("disable-gpu", true), chromedp.Flag("disable-dev-shm-usage", true))
	defer cancelAllocator()
	ctx, cancelBrowser := chromedp.NewContext(allocator)
	defer cancelBrowser()
	ctx, cancelTimeout := context.WithTimeout(ctx, 40*time.Second)
	defer cancelTimeout()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(int64(width), int64(height)), chromedp.Navigate(serverURL+path), chromedp.WaitReady("body")); err != nil {
		t.Fatal(err)
	}
	run(ctx, serverURL)
}

func e2eWorkspaceServer(t *testing.T) string {
	t.Helper()
	manager, err := session.New("render-e2e-session-secret-0123456789", session.Options{CookieName: "gxa_e2e", Path: "/", AllowInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	resource := workbench.Resource{Slug: "items", Label: "Items", Singular: "Item", Route: "/list", Columns: []workbench.Column{{Name: "title", Label: "Title"}}, Fields: []workbench.Field{{Name: "title", Label: "Title", Kind: workbench.FieldText, Required: true, Help: "Use a short title."}}}
	ws := workbench.Workspace{Resources: []workbench.Resource{
		{Slug: "today", Label: "Today", Route: "/list"}, {Slug: "notes", Label: "Notes", Route: "/detail"},
		{Slug: "money", Label: "Money", Route: "/form"}, {Slug: "calendar", Label: "Calendar", Route: "/calendar"},
		{Slug: "reports", Label: "Reports", Route: "/reports"},
	}}
	access := Access{Principal: Principal{ID: "e2e", Roles: []string{"operator"}}, Authorizer: testPolicy()}
	actionSpec := workbench.Action{Name: "save", Label: "Save", Description: "Save this sample record."}
	guard := Guard(GuardOptions{Resource: resource, Action: actionSpec, Fields: resource.Fields, Authorizer: testPolicy(),
		Principal: func(*http.Request) (Principal, error) { return Principal{ID: "e2e", Roles: []string{"operator"}}, nil },
		SignInURL: "/sign-in", Fallback: "/form"}, func(_ context.Context, in ActionInput) (ActionResult, error) {
		return ActionResult{}, &ConflictError{Revision: 2, Current: map[string]string{"title": "Saved title"}}
	})
	mux := http.NewServeMux()
	mux.Handle(theme.Path, theme.Handler())
	mux.HandleFunc("POST /gosx/action/save", func(w http.ResponseWriter, r *http.Request) { action.ServeHandler(w, r, guard) })
	for _, path := range []string{"/list", "/detail", "/form"} {
		pagePath := path
		mux.HandleFunc("GET "+pagePath, func(w http.ResponseWriter, r *http.Request) {
			nav := BuildNavigation(r.Context(), ws, NavOptions{CurrentPath: r.URL.Path, Order: []string{"today", "notes", "money", "calendar", "reports"}, Phone: []string{"today", "notes", "money"}, MoreHref: "/more", Access: access})
			var content gosx.Node
			switch pagePath {
			case "/list":
				rowAction := ActionForm{Resource: resource.Slug, Action: workbench.Action{Name: "archive", Label: "Archive", Confirm: true}, URL: "/archive", CSRF: CSRFToken(r), Key: NewKey(), RecordID: "1", Revision: 1, Confirm: &Confirmation{Summary: "Sample item", Consequence: "The item will be archived."}}
				content = gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-page")), gosx.El("h1", nil, gosx.Text("Items")), List(r.Context(), ListProps{Resource: resource, Access: access, Rows: []Row{{ID: "1", Href: "/detail", Cells: map[string]string{"title": "Sample title"}, Actions: []ActionForm{rowAction}}}}))
			case "/detail":
				detailAction := ActionForm{Resource: resource.Slug, Action: workbench.Action{Name: "archive", Label: "Archive", Description: "The item will be archived.", Confirm: true}, URL: "/archive", CSRF: CSRFToken(r), Key: NewKey(), RecordID: "1", Revision: 1, Confirm: &Confirmation{Summary: "Sample item", Consequence: "The item will be archived.", Check: "I reviewed this action."}}
				content = gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-page")), gosx.El("h1", nil, gosx.Text("Item")), Detail(r.Context(), DetailProps{Resource: resource, Access: access, Record: Record{ID: "1", Revision: 1, Values: map[string]string{"title": "Saved title"}}, Actions: []ActionForm{detailAction}}))
			case "/form":
				feedback, _ := ReadFeedback(r, "save")
				values := map[string]string{"title": "Original title"}
				formAction := ActionForm{Resource: resource.Slug, Action: actionSpec, URL: "/gosx/action/save", CSRF: CSRFToken(r), Key: NewKey(), RecordID: "1", Revision: 1, ReturnTo: "/form"}
				content = gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-page")), gosx.El("h1", nil, gosx.Text("Edit item")), Form(r.Context(), FormProps{Resource: resource, Access: access, Action: formAction, Values: values, Feedback: feedback}))
			}
			frame := Shell(ShellProps{Brand: gosx.Text("Sample admin"), BrandHref: "/list", Nav: nav, Footer: gosx.El("a", gosx.Attrs(gosx.Attr("href", "/help")), gosx.Text("Help")), Status: StatusFor(r)}, content)
			doc := `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><link rel="stylesheet" href="` + theme.Href() + `"></head><body>` + gosx.RenderHTML(frame) + `<script data-gosx-navigation="true">` + runtimehost.NavigationRuntime + `</script></body></html>`
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(doc))
		})
	}
	server := httptest.NewServer(manager.Middleware(mux))
	t.Cleanup(server.Close)
	return server.URL
}
