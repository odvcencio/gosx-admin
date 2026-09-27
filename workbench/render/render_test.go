package render

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"m31labs.dev/gosx"
	"m31labs.dev/gosx-admin/workbench"
	"m31labs.dev/gosx-admin/workbench/render/rendertest"
	"m31labs.dev/gosx/action"
	runtimehost "m31labs.dev/gosx/client/runtime/host"
	"m31labs.dev/gosx/session"
)

func testResource() workbench.Resource {
	return workbench.Resource{
		Slug: "items", Label: "Items", Singular: "Item", Route: "/admin/items",
		Columns: []workbench.Column{{Name: "title", Label: "Title"}, {Name: "email", Label: "Email"}, {Name: "secret", Label: "Secret"}},
		Fields:  []workbench.Field{{Name: "title", Label: "Title", Kind: workbench.FieldText, Required: true}, {Name: "email", Label: "Email", Kind: workbench.FieldText}, {Name: "secret", Label: "Secret", Kind: workbench.FieldText}},
		Actions: []workbench.Action{{Name: "save", Label: "Save"}},
	}
}

func testPolicy() Policy {
	return Policy{Rules: []Rule{{Role: "operator", Resource: "*", Read: true, Actions: []string{"*"}}}}
}

func testAccess() Access {
	return Access{Principal: Principal{ID: "op-1", Roles: []string{"operator"}}, Authorizer: testPolicy()}
}

func TestPolicyReadActField(t *testing.T) {
	policy := Policy{Rules: []Rule{
		{Role: "reader", Resource: "items", Read: true, Redact: []string{"email"}, Hide: []string{"secret"}},
		{Role: "editor", Resource: "items", Read: true, Actions: []string{"save"}},
	}}
	pr := Principal{Roles: []string{"reader", "editor"}}
	if !policy.CanRead(context.Background(), pr, "items") || !policy.CanAct(context.Background(), pr, "items", "save") {
		t.Fatal("matching reader/editor roles should read and save")
	}
	if got := policy.Field(context.Background(), pr, "items", "email"); got != FieldVisible {
		t.Fatalf("permissive matching field access = %v, want visible", got)
	}
	if got := policy.Field(context.Background(), Principal{Roles: []string{"reader"}}, "items", "secret"); got != FieldHidden {
		t.Fatalf("hidden field access = %v", got)
	}
	if policy.CanAct(context.Background(), Principal{Roles: []string{"reader"}}, "items", "save") {
		t.Fatal("reader may not act")
	}
}

func TestPolicyWildcards(t *testing.T) {
	p := Policy{Rules: []Rule{{Role: "admin", Resource: "*", Read: true, Actions: []string{"*"}}}}
	pr := Principal{Roles: []string{"admin"}}
	if !p.CanRead(context.Background(), pr, "tool") || !p.CanAct(context.Background(), pr, "resource", "delete") {
		t.Fatal("resource and action wildcards did not match")
	}
}

func TestAccessNilAuthorizerDenies(t *testing.T) {
	a := Access{}
	if a.CanRead(context.Background(), "items") || a.CanAct(context.Background(), "items", "save") || a.Field(context.Background(), "items", "title") != FieldHidden {
		t.Fatal("nil authorizer did not fail closed")
	}
}

func TestBuildNavigationOrder(t *testing.T) {
	ws := workbench.Workspace{
		Tools:     []workbench.Tool{{Slug: "tool-b", Label: "B", Route: "/b"}, {Slug: "tool-a", Label: "A", Route: "/a"}, {Slug: "tool-empty", Label: "Empty"}},
		Resources: []workbench.Resource{{Slug: "resource-b", Label: "RB", Route: "/rb"}, {Slug: "resource-a", Label: "RA", Route: "/ra"}},
	}
	nav := BuildNavigation(context.Background(), ws, NavOptions{Order: []string{"resource-a", "unknown", "tool-a"}, Access: testAccess()})
	got := []string{}
	for _, item := range nav.Items {
		got = append(got, item.Slug)
	}
	want := []string{"resource-a", "tool-a", "tool-b", "resource-b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("item order = %v, want %v", got, want)
	}
}

func TestBuildNavigationHidesUnreadable(t *testing.T) {
	ws := workbench.Workspace{Resources: []workbench.Resource{{Slug: "shown", Label: "Shown", Route: "/shown"}, {Slug: "hidden", Label: "Hidden", Route: "/hidden"}}}
	a := Access{Principal: Principal{Roles: []string{"operator"}}, Authorizer: Policy{Rules: []Rule{{Role: "operator", Resource: "shown", Read: true}}}}
	nav := BuildNavigation(context.Background(), ws, NavOptions{Access: a})
	if len(nav.Items) != 1 || nav.Items[0].Slug != "shown" {
		t.Fatalf("read filtering produced %#v", nav.Items)
	}
}

func TestBuildNavigationFourFitWithoutMore(t *testing.T) {
	ws := workbench.Workspace{}
	for _, slug := range []string{"a", "b", "c", "d"} {
		ws.Resources = append(ws.Resources, workbench.Resource{Slug: slug, Label: slug, Route: "/" + slug})
	}
	nav := BuildNavigation(context.Background(), ws, NavOptions{Access: testAccess()})
	if len(nav.Bar) != 4 || len(nav.More) != 0 {
		t.Fatalf("phone bar=%d More=%d", len(nav.Bar), len(nav.More))
	}
}

func TestBuildNavigationFiveUseMore(t *testing.T) {
	ws := workbench.Workspace{}
	for _, slug := range []string{"a", "b", "c", "d", "e"} {
		ws.Resources = append(ws.Resources, workbench.Resource{Slug: slug, Label: strings.ToUpper(slug), Route: "/" + slug})
	}
	nav := BuildNavigation(context.Background(), ws, NavOptions{Phone: []string{"d", "b", "b"}, MoreHref: "/more", Access: testAccess()})
	if len(nav.Bar) != 4 || nav.Bar[0].Slug != "d" || nav.Bar[1].Slug != "b" || nav.Bar[2].Slug != "a" || nav.Bar[3].Slug != MoreSlug || len(nav.More) != 2 {
		t.Fatalf("unexpected bar/More: %#v %#v", nav.Bar, nav.More)
	}
}

func TestBuildNavigationCurrentLongestPrefix(t *testing.T) {
	ws := workbench.Workspace{Resources: []workbench.Resource{{Slug: "admin", Label: "Admin", Route: "/admin"}, {Slug: "items", Label: "Items", Route: "/admin/items"}}}
	nav := BuildNavigation(context.Background(), ws, NavOptions{CurrentPath: "/admin/items/7", Access: testAccess()})
	if nav.Items[0].Current != CurrentNone || nav.Items[1].Current != CurrentSection {
		t.Fatalf("current state = %#v", nav.Items)
	}
}

func TestBuildNavigationRootMatchesOnlyItself(t *testing.T) {
	ws := workbench.Workspace{Resources: []workbench.Resource{{Slug: "root", Label: "Root", Route: "/"}}}
	for _, path := range []string{"/", "/other"} {
		nav := BuildNavigation(context.Background(), ws, NavOptions{CurrentPath: path, Access: testAccess()})
		want := CurrentNone
		if path == "/" {
			want = CurrentPage
		}
		if nav.Items[0].Current != want {
			t.Errorf("path %q current=%v, want %v", path, nav.Items[0].Current, want)
		}
	}
}

func TestBuildNavigationMoreSection(t *testing.T) {
	ws := workbench.Workspace{}
	for _, slug := range []string{"today", "notes", "money", "reports", "calendar"} {
		ws.Resources = append(ws.Resources, workbench.Resource{Slug: slug, Label: slug, Route: "/" + slug})
	}
	nav := BuildNavigation(context.Background(), ws, NavOptions{CurrentPath: "/reports/weekly", Phone: []string{"today", "notes", "money"}, MoreHref: "/more", Access: testAccess()})
	if nav.Bar[3].Slug != MoreSlug || nav.Bar[3].Current != CurrentSection {
		t.Fatalf("More state = %#v", nav.Bar[3])
	}
}

func TestShellLandmarks(t *testing.T) {
	props := ShellProps{Brand: gosx.Text("Admin"), BrandHref: "/admin", Nav: Navigation{Items: []NavItem{{Slug: "items", Label: "Items", Href: "/items"}}}, Footer: gosx.Text("Help")}
	doc := gosx.RenderHTML(Shell(props, gosx.El("section", nil, gosx.El("h1", nil, gosx.Text("Items")))))
	rendertest.AssertAccessible(t, doc, rendertest.Options{Page: true})
	if !strings.HasPrefix(doc, `<div class="gxa-shell"><a`) || !strings.Contains(doc, `href="#main"`) {
		t.Fatalf("skip link is not the first link: %s", doc)
	}
}

func TestShellPartsMatchShell(t *testing.T) {
	props := ShellProps{Brand: gosx.Text("Admin"), BrandHref: "/", Account: gosx.Text("Operator"), Nav: Navigation{Items: []NavItem{{Slug: "x", Label: "X", Href: "/x"}}, Bar: []NavItem{{Slug: "x", Label: "X", Href: "/x"}}}, Status: StatusMessage{Text: "Saved."}, Footer: gosx.Text("Help")}
	parts := Parts(props)
	content := gosx.El("h1", nil, gosx.Text("Page"))
	manual := gosx.El("div", gosx.Attrs(gosx.Attr("class", "gxa-shell")), parts.SkipLink, parts.Header, parts.DesktopNav,
		gosx.El("main", gosx.Attrs(gosx.Attr("id", MainID), gosx.Attr("class", "gxa-main"), gosx.Attr("tabindex", "-1"), gosx.BoolAttr("data-gosx-main")), parts.Status, content), parts.Footer, parts.PhoneNav)
	if got, want := gosx.RenderHTML(Shell(props, content)), gosx.RenderHTML(manual); got != want {
		t.Fatalf("Parts composition differs\n got %s\nwant %s", got, want)
	}
}

func TestShellAriaCurrentValues(t *testing.T) {
	doc := gosx.RenderHTML(Shell(ShellProps{Brand: gosx.Text("Admin"), Nav: Navigation{Items: []NavItem{{Label: "Page", Href: "/page", Current: CurrentPage}, {Label: "Section", Href: "/section", Current: CurrentSection}}}}, gosx.El("h1", nil, gosx.Text("Page"))))
	if !strings.Contains(doc, `aria-current="page"`) || !strings.Contains(doc, `aria-current="true"`) {
		t.Fatalf("aria-current values missing: %s", doc)
	}
}

func TestNavBadgeAccessibleText(t *testing.T) {
	doc := gosx.RenderHTML(Shell(ShellProps{Brand: gosx.Text("Admin"), Nav: Navigation{Items: []NavItem{{Label: "Items", Href: "/items", Badge: Badge{Count: 3, Label: "3 need review"}}}}}, gosx.El("h1", nil, gosx.Text("Items"))))
	if !strings.Contains(doc, `aria-hidden="true">3</span>`) || !strings.Contains(doc, `class="gxa-visually-hidden">, 3 need review</span>`) {
		t.Fatalf("badge accessible text missing: %s", doc)
	}
	zero := gosx.RenderHTML(Shell(ShellProps{Brand: gosx.Text("Admin"), Nav: Navigation{Items: []NavItem{{Label: "Items", Href: "/items", Badge: Badge{Count: 0}}}}}, gosx.El("h1", nil, gosx.Text("Items"))))
	if strings.Contains(zero, "gxa-badge") {
		t.Fatal("zero badge should be omitted")
	}
}

func TestStatusRegionRoles(t *testing.T) {
	for _, tc := range []struct {
		status StatusMessage
		role   string
	}{{StatusMessage{Text: "Saved."}, "status"}, {StatusMessage{Text: "Failed.", Error: true}, "alert"}} {
		doc := gosx.RenderHTML(Shell(ShellProps{Brand: gosx.Text("Admin"), Status: tc.status}, gosx.El("h1", nil, gosx.Text("Page"))))
		if !strings.Contains(doc, `data-gosx-toast-host`) || !strings.Contains(doc, `role="`+tc.role+`"`) {
			t.Errorf("status markup missing role %s: %s", tc.role, doc)
		}
	}
}

func TestStatusForReadsFlash(t *testing.T) {
	manager := testSessionManager(t)
	seedCookie, _ := seedSession(t, manager)
	seed := httptest.NewRecorder()
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.AddFlash(r, "__gosx_action_state", action.View{Name: "zeta", Status: http.StatusSeeOther, Result: action.Result{OK: true, Message: "zeta saved"}})
		session.AddFlash(r, "__gosx_action_state", action.View{Name: "alpha", Status: http.StatusSeeOther, Result: action.Result{OK: true, Message: "alpha saved"}})
	})).ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "http://example.test/seed-flashes", nil))
	seedCookie = seed.Result().Cookies()
	var got StatusMessage
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	for _, c := range seedCookie {
		req.AddCookie(c)
	}
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = StatusFor(r) })).ServeHTTP(httptest.NewRecorder(), req)
	if got.Text != "alpha saved" || got.Error {
		t.Fatalf("StatusFor() = %#v", got)
	}
}

func TestListTableSemantics(t *testing.T) {
	resource := testResource()
	doc := gosx.RenderHTML(List(context.Background(), ListProps{Resource: resource, Access: testAccess(), Rows: []Row{{ID: "1", Href: "/admin/items/1", Cells: map[string]string{"title": "One", "email": "one@example.test", "secret": "s"}}}}))
	for _, want := range []string{`<caption>Items</caption>`, `role="table"`, `role="rowgroup"`, `role="columnheader"`, `scope="col"`, `role="rowheader"`, `scope="row"`, `data-label="Email"`} {
		if !strings.Contains(doc, want) {
			t.Errorf("table lacks %s: %s", want, doc)
		}
	}
}

func TestListRedaction(t *testing.T) {
	a := Access{Principal: Principal{Roles: []string{"operator"}}, Authorizer: Policy{Rules: []Rule{{Role: "operator", Resource: "items", Read: true, Redact: []string{"email"}, Hide: []string{"secret"}}}}}
	doc := gosx.RenderHTML(List(context.Background(), ListProps{Resource: testResource(), Access: a, Rows: []Row{{Cells: map[string]string{"title": "Visible", "email": "private@example.test", "secret": "never-render"}}}}))
	if strings.Contains(doc, "never-render") || strings.Contains(doc, ">Secret<") || !strings.Contains(doc, ">Hidden<") || strings.Contains(doc, "private@example.test") {
		t.Fatalf("field redaction failed: %s", doc)
	}
}

func TestListStates(t *testing.T) {
	for _, tc := range []struct {
		state State
		role  string
	}{{StateEmpty, ""}, {StateError, `role="alert"`}, {StateLoading, `role="status"`}} {
		doc := gosx.RenderHTML(List(context.Background(), ListProps{Resource: testResource(), State: tc.state}))
		if tc.role != "" && !strings.Contains(doc, tc.role) {
			t.Errorf("state %v lacks %s", tc.state, tc.role)
		}
		if strings.Contains(doc, `<table`) {
			t.Errorf("state %v rendered a table", tc.state)
		}
	}
	zero := gosx.RenderHTML(List(context.Background(), ListProps{Resource: testResource(), Access: testAccess()}))
	if !strings.Contains(zero, "Nothing here yet.") {
		t.Fatalf("zero rows should render empty state: %s", zero)
	}
}

func TestErrorStateRetryAndHelp(t *testing.T) {
	doc := gosx.RenderHTML(StateBlock(StateError, StateCopy{Help: Link{Label: "Get help", Href: "/help"}}))
	if !strings.Contains(doc, `href=""`) || !strings.Contains(doc, `href="/help"`) || !strings.Contains(doc, "Try again") {
		t.Fatalf("error state links missing: %s", doc)
	}
	withoutHelp := gosx.RenderHTML(StateBlock(StateError, StateCopy{}))
	if strings.Contains(withoutHelp, "Get help") {
		t.Fatal("help link rendered without an href")
	}
}

func TestDetailRedaction(t *testing.T) {
	a := Access{Principal: Principal{Roles: []string{"operator"}}, Authorizer: Policy{Rules: []Rule{{Role: "operator", Resource: "items", Read: true, Redact: []string{"email"}, Hide: []string{"secret"}}}}}
	doc := gosx.RenderHTML(Detail(context.Background(), DetailProps{Resource: testResource(), Access: a, Record: Record{Values: map[string]string{"title": "Title", "email": "private@example.test", "secret": "never-render"}}}))
	if strings.Contains(doc, "never-render") || strings.Contains(doc, "private@example.test") || strings.Contains(doc, ">Secret<") || !strings.Contains(doc, ">Hidden<") {
		t.Fatalf("detail redaction failed: %s", doc)
	}
}

func TestFormControlsByKind(t *testing.T) {
	fields := []workbench.Field{
		{Name: "text", Label: "Text", Kind: workbench.FieldText}, {Name: "slug", Label: "Slug", Kind: workbench.FieldSlug},
		{Name: "textarea", Label: "Text area", Kind: workbench.FieldTextarea}, {Name: "money", Label: "Money", Kind: workbench.FieldMoney},
		{Name: "bool", Label: "Boolean", Kind: workbench.FieldBoolean}, {Name: "select", Label: "Select", Kind: workbench.FieldSelect, Options: []string{"one"}},
		{Name: "relation", Label: "Relation", Kind: workbench.FieldRelation, Options: []string{"one"}}, {Name: "date", Label: "Date", Kind: workbench.FieldDateTime}, {Name: "image", Label: "Image", Kind: workbench.FieldImage},
	}
	doc := gosx.RenderHTML(Form(context.Background(), FormProps{Resource: workbench.Resource{Slug: "items"}, Access: testAccess(), Fields: fields, Action: ActionForm{Resource: "items", Action: workbench.Action{Name: "save", Label: "Save"}, URL: "/save", Key: NewKey()}}))
	for _, want := range []string{`id="gxa-items-save-text"`, `id="gxa-items-save-slug"`, `<textarea`, `inputmode="decimal"`, `type="checkbox"`, `<select`, `type="datetime-local"`, `type="url"`} {
		if !strings.Contains(doc, want) {
			t.Errorf("form controls lack %s", want)
		}
	}
}

func TestFormDescribedBy(t *testing.T) {
	resource := workbench.Resource{Slug: "items", Fields: []workbench.Field{{Name: "title", Label: "Title", Kind: workbench.FieldText, Help: "One short sentence."}}}
	access := testAccess()
	feedback := Feedback{Errors: map[string]string{"title": "Enter a title."}}
	doc := gosx.RenderHTML(Form(context.Background(), FormProps{Resource: resource, Access: access, Action: ActionForm{Resource: "items", Action: workbench.Action{Name: "save"}, URL: "/save"}, Feedback: feedback}))
	if !strings.Contains(doc, `aria-describedby="gxa-items-save-title-help gxa-items-save-title-error"`) || !strings.Contains(doc, `id="gxa-items-save-title-help"`) || !strings.Contains(doc, `id="gxa-items-save-title-error"`) || !strings.Contains(doc, `aria-invalid="true"`) {
		t.Fatalf("help/error descriptions missing or unordered: %s", doc)
	}
}

func TestFormErrorSummaryLinksToFields(t *testing.T) {
	resource := workbench.Resource{Slug: "items", Fields: []workbench.Field{{Name: "title", Label: "Title", Kind: workbench.FieldText}, {Name: "email", Label: "Email", Kind: workbench.FieldText}}}
	feedback := Feedback{Errors: map[string]string{"title": "Enter a title.", "email": "Enter an email."}}
	doc := gosx.RenderHTML(Form(context.Background(), FormProps{Resource: resource, Access: testAccess(), Action: ActionForm{Resource: "items", Action: workbench.Action{Name: "save"}, URL: "/save"}, Feedback: feedback}))
	for _, want := range []string{`class="gxa-error-summary"`, `role="alert"`, `tabindex="-1"`, `autofocus`, `href="#gxa-items-save-title"`, `href="#gxa-items-save-email"`} {
		if !strings.Contains(doc, want) {
			t.Errorf("summary lacks %s", want)
		}
	}
}

func TestFormHiddenInputsOrder(t *testing.T) {
	doc := gosx.RenderHTML(Form(context.Background(), FormProps{Resource: workbench.Resource{Slug: "items", Fields: []workbench.Field{{Name: "title", Label: "Title"}}}, Access: testAccess(), Action: ActionForm{Resource: "items", Action: workbench.Action{Name: "save"}, URL: "/save", CSRF: "csrf", Key: "key", RecordID: "rec", Revision: 2, ReturnTo: "/back", Hidden: map[string]string{"z": "z", "a": "a"}}}))
	names := []string{CSRFField, KeyField, RecordField, RevisionField, action.ReturnTargetField, "a", "z"}
	last := -1
	for _, name := range names {
		index := strings.Index(doc, `name="`+name+`"`)
		if index <= last {
			t.Fatalf("hidden name %q at %d after %d: %s", name, index, last, doc)
		}
		last = index
	}
}

func TestFormRedactedFieldHasNoControl(t *testing.T) {
	a := Access{Principal: Principal{Roles: []string{"operator"}}, Authorizer: Policy{Rules: []Rule{{Role: "operator", Resource: "items", Read: true, Redact: []string{"email"}}}}}
	doc := gosx.RenderHTML(Form(context.Background(), FormProps{Resource: testResource(), Access: a, Action: ActionForm{Resource: "items", Action: workbench.Action{Name: "save"}, URL: "/save"}}))
	if strings.Contains(doc, `name="email"`) || strings.Contains(doc, `id="gxa-items-save-email"`) || !strings.Contains(doc, `class="gxa-field gxa-field--redacted"`) || !strings.Contains(doc, "Hidden") {
		t.Fatalf("redacted field rendered a control: %s", doc)
	}
}

func TestFormUsesFeedback(t *testing.T) {
	resource := workbench.Resource{Slug: "items", Fields: []workbench.Field{{Name: "title", Label: "Title", Kind: workbench.FieldText}}}
	feedback := Feedback{Values: map[string]string{"title": "typed"}, Conflict: &Conflict{Revision: 9, Current: map[string]string{"title": "saved"}, Entered: map[string]string{"title": "typed"}}}
	doc := gosx.RenderHTML(Form(context.Background(), FormProps{Resource: resource, Access: testAccess(), Values: map[string]string{"title": "original"}, Feedback: feedback, Action: ActionForm{Resource: "items", Action: workbench.Action{Name: "save"}, URL: "/save", Revision: 2}}))
	if !strings.Contains(doc, `value="typed"`) || !strings.Contains(doc, `name="gxa_revision" value="9"`) || !strings.Contains(doc, "This record changed") {
		t.Fatalf("feedback did not replace saved values or revision: %s", doc)
	}
}

func TestParseFormRules(t *testing.T) {
	tests := []struct {
		name  string
		field workbench.Field
		value string
		want  string
		bad   bool
	}{
		{"trim", workbench.Field{Name: "name", Label: "Name", Kind: workbench.FieldText}, "  Sam  ", "Sam", false},
		{"required", workbench.Field{Name: "title", Label: "Title", Kind: workbench.FieldText, Required: true}, " ", "", true},
		{"unicode max length", workbench.Field{Name: "title", Label: "Title", Kind: workbench.FieldText, MaxLength: 2}, "🙂🙂", "🙂🙂", false},
		{"unicode too long", workbench.Field{Name: "title", Label: "Title", Kind: workbench.FieldText, MaxLength: 1}, "🙂🙂", "🙂🙂", true},
		{"select", workbench.Field{Name: "choice", Label: "Choice", Kind: workbench.FieldSelect, Options: []string{"one"}}, "one", "one", false},
		{"bad select", workbench.Field{Name: "choice", Label: "Choice", Kind: workbench.FieldSelect, Options: []string{"one"}}, "two", "two", true},
		{"boolean checked", workbench.Field{Name: "enabled", Label: "Enabled", Kind: workbench.FieldBoolean}, "true", "true", false},
		{"boolean unchecked", workbench.Field{Name: "enabled", Label: "Enabled", Kind: workbench.FieldBoolean}, "", "false", false},
		{"datetime", workbench.Field{Name: "at", Label: "At", Kind: workbench.FieldDateTime}, "2026-09-26T09:30", "2026-09-26T09:30", false},
		{"bad datetime", workbench.Field{Name: "at", Label: "At", Kind: workbench.FieldDateTime}, "tomorrow", "tomorrow", true},
		{"money", workbench.Field{Name: "amount", Label: "Amount", Kind: workbench.FieldMoney}, "12.50", "12.50", false},
		{"bad money", workbench.Field{Name: "amount", Label: "Amount", Kind: workbench.FieldMoney}, "-1.234", "-1.234", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values, errs := ParseForm(context.Background(), testAccess(), "items", []workbench.Field{tc.field}, map[string]string{tc.field.Name: tc.value})
			if values[tc.field.Name] != tc.want {
				t.Fatalf("value = %q, want %q", values[tc.field.Name], tc.want)
			}
			if gotBad := errs[tc.field.Name] != ""; gotBad != tc.bad {
				t.Fatalf("errors = %#v, bad=%v", errs, tc.bad)
			}
		})
	}
}

func TestParseFormDropsUnwritable(t *testing.T) {
	a := Access{Principal: Principal{Roles: []string{"operator"}}, Authorizer: Policy{Rules: []Rule{{Role: "operator", Resource: "items", Read: true, Hide: []string{"hidden"}, Redact: []string{"redacted"}}}}}
	fields := []workbench.Field{{Name: "readonly", Label: "Readonly", ReadOnly: true}, {Name: "hidden", Label: "Hidden"}, {Name: "redacted", Label: "Redacted"}, {Name: "writable", Label: "Writable"}}
	values, errs := ParseForm(context.Background(), a, "items", fields, map[string]string{"readonly": "r", "hidden": "h", "redacted": "x", "writable": "w", "unknown": "u"})
	if len(errs) != 0 || len(values) != 1 || values["writable"] != "w" {
		t.Fatalf("unwritable fields survived: %#v %#v", values, errs)
	}
}

func TestKeys(t *testing.T) {
	key := NewKey()
	if !ValidKey(key) || len(key) != 32 {
		t.Fatalf("NewKey() = %q", key)
	}
	for _, invalid := range []string{"ABCDEF0123456789ABCDEF0123456789", "0123456789abcdef0123456789abcde", "0123456789abcdef0123456789abcdeg"} {
		if ValidKey(invalid) {
			t.Errorf("ValidKey(%q) = true", invalid)
		}
	}
}

func TestActionOmittedWithoutPermission(t *testing.T) {
	node := Action(context.Background(), Access{}, ActionForm{Resource: "items", Action: workbench.Action{Name: "delete", Label: "Delete"}, URL: "/delete"})
	if got := gosx.RenderHTML(node); got != "" {
		t.Fatalf("unauthorized action rendered %q", got)
	}
}

func TestConfirmActionMarkup(t *testing.T) {
	doc := gosx.RenderHTML(Action(context.Background(), testAccess(), ActionForm{Resource: "items", Action: workbench.Action{Name: "delete", Label: "Delete", Confirm: true, Description: "This removes the item."}, URL: "/delete", Confirm: &Confirmation{Summary: "Item 17", Check: "I reviewed this item."}}))
	for _, want := range []string{`<details class="gxa-confirm">`, `<summary`, "Item 17", "This removes the item.", `name="gxa_confirm"`, `value="yes"`, `required`, `for="gxa-items-delete-confirm"`, "I reviewed this item."} {
		if !strings.Contains(doc, want) {
			t.Errorf("confirmation markup lacks %q", want)
		}
	}
}

func TestConflictViewOnlyChangedVisibleFields(t *testing.T) {
	a := Access{Principal: Principal{Roles: []string{"operator"}}, Authorizer: Policy{Rules: []Rule{{Role: "operator", Resource: "items", Read: true, Redact: []string{"email"}, Hide: []string{"secret"}}}}}
	resource := testResource()
	doc := gosx.RenderHTML(ConflictView(context.Background(), a, resource, Conflict{Current: map[string]string{"title": "same", "email": "private", "secret": "hidden"}, Entered: map[string]string{"title": "same", "email": "entered-private", "secret": "entered-hidden"}}, "conflict", 2))
	if strings.Contains(doc, "same") || strings.Contains(doc, "private") || strings.Contains(doc, "hidden") || strings.Contains(doc, "Secret") || strings.Contains(doc, "Email") {
		t.Fatalf("unchanged or inaccessible fields rendered: %s", doc)
	}
	resource.Fields = resource.Fields[:1]
	doc = gosx.RenderHTML(ConflictView(context.Background(), testAccess(), resource, Conflict{Current: map[string]string{"title": "saved"}, Entered: map[string]string{"title": "typed"}}, "conflict", 2))
	if !strings.Contains(doc, "saved") || !strings.Contains(doc, "typed") {
		t.Fatalf("changed visible values absent: %s", doc)
	}
}

func TestGuardUnauthenticatedRedirects(t *testing.T) {
	opts := guardOptions()
	opts.Principal = func(*http.Request) (Principal, error) { return Principal{}, ErrUnauthenticated }
	response := guardPost(t, testSession(t), Guard(opts, nil), map[string]string{}, true)
	var result action.Result
	if json.Unmarshal(response.Body.Bytes(), &result) != nil || response.Code != http.StatusSeeOther || result.Redirect != "/sign-in" {
		t.Fatalf("unauthenticated response = %d %s", response.Code, response.Body.String())
	}
}

func TestGuardCSRF(t *testing.T) {
	var events []AuditEvent
	opts := guardOptions()
	opts.Auditor = AuditFunc(func(_ context.Context, e AuditEvent) error { events = append(events, e); return nil })
	guard := Guard(opts, func(context.Context, ActionInput) (ActionResult, error) { return ActionResult{}, nil })
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/gosx/action/save", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	action.ServeHandler(response, req, guard)
	if response.Code != http.StatusForbidden {
		t.Fatalf("no-session status = %d", response.Code)
	}
	cs := testSession(t)
	response = guardPost(t, cs, guard, map[string]string{CSRFField: "wrong"}, true)
	if response.Code != http.StatusForbidden || len(events) != 2 || events[0].Reason != "csrf" || events[1].Reason != "csrf" || events[0].Outcome != AuditDenied {
		t.Fatalf("CSRF status/events = %d %#v", response.Code, events)
	}
}

func TestGuardForbiddenIs404(t *testing.T) {
	var event AuditEvent
	opts := guardOptions()
	opts.Authorizer = Policy{}
	opts.Auditor = AuditFunc(func(_ context.Context, e AuditEvent) error { event = e; return nil })
	response := guardPost(t, testSession(t), Guard(opts, nil), nil, true)
	if response.Code != http.StatusNotFound || event.Outcome != AuditDenied || event.Reason != "forbidden" {
		t.Fatalf("forbidden status/event = %d %#v", response.Code, event)
	}
}

func TestGuardBadKeyOrRevision(t *testing.T) {
	for _, tc := range []struct{ key, rev, reason string }{{"BAD", "1", "key"}, {"0123456789abcdef0123456789abcdef", "-1", "revision"}, {"0123456789abcdef0123456789abcdef", "no", "revision"}} {
		t.Run(tc.reason+tc.rev+tc.key, func(t *testing.T) {
			var reason string
			opts := guardOptions()
			opts.Auditor = AuditFunc(func(_ context.Context, e AuditEvent) error { reason = e.Reason; return nil })
			response := guardPost(t, testSession(t), Guard(opts, nil), map[string]string{KeyField: tc.key, RevisionField: tc.rev}, true)
			if response.Code != http.StatusUnprocessableEntity || reason != tc.reason {
				t.Fatalf("status/reason = %d/%s", response.Code, reason)
			}
		})
	}
}

func TestGuardRequiresConfirmation(t *testing.T) {
	opts := guardOptions()
	opts.Action.Confirm = true
	response := guardPost(t, testSession(t), Guard(opts, nil), map[string]string{"title": "Valid"}, true)
	result := readResult(t, response)
	if response.Code != http.StatusUnprocessableEntity || result.FieldErrors[ConfirmField] != "Check this box to confirm." {
		t.Fatalf("confirmation response = %d %#v", response.Code, result)
	}
}

func TestGuardValidation(t *testing.T) {
	opts := guardOptions()
	response := guardPost(t, testSession(t), Guard(opts, nil), map[string]string{}, true)
	result := readResult(t, response)
	if response.Code != http.StatusUnprocessableEntity || result.Message != "Check the highlighted fields." || result.FieldErrors["title"] == "" {
		t.Fatalf("validation response = %d %#v", response.Code, result)
	}
}

func TestGuardConflictNative(t *testing.T) {
	manager := testSessionManager(t)
	cookies, _ := seedSession(t, manager)
	opts := guardOptions()
	opts.Authorizer = Policy{Rules: []Rule{{Role: "operator", Resource: "items", Read: true, Actions: []string{"save"}, Redact: []string{"email"}, Hide: []string{"secret"}}}}
	guard := Guard(opts, func(context.Context, ActionInput) (ActionResult, error) {
		return ActionResult{}, &ConflictError{Revision: 8, Current: map[string]string{"title": "saved", "email": "private-email", "secret": "private-secret"}}
	})
	cookies, token := seedSession(t, manager)
	response := guardPostWithManager(t, manager, cookies, guard, map[string]string{"title": "typed"}, false, token)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("native conflict did not redirect: %d %s", response.Code, response.Body.String())
	}
	newCookies := response.Result().Cookies()
	var feedback Feedback
	request := httptest.NewRequest(http.MethodGet, "/admin/items/1", nil)
	for _, cookie := range newCookies {
		request.AddCookie(cookie)
	}
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		feedback, _ = ReadFeedback(r, "save")
	})).ServeHTTP(httptest.NewRecorder(), request)
	if feedback.Status != http.StatusConflict || feedback.Values["title"] != "typed" || feedback.Conflict == nil || feedback.Conflict.Revision != 8 || len(feedback.Conflict.Current) != 1 || feedback.Conflict.Current["title"] != "saved" {
		t.Fatalf("native conflict feedback = %#v", feedback)
	}
}

func TestGuardConflictManaged(t *testing.T) {
	manager := testSessionManager(t)
	cookies, token := seedSession(t, manager)
	opts := guardOptions()
	opts.Authorizer = Policy{Rules: []Rule{{Role: "operator", Resource: "items", Read: true, Actions: []string{"save"}, Redact: []string{"email"}, Hide: []string{"secret"}}}}
	guard := Guard(opts, func(context.Context, ActionInput) (ActionResult, error) {
		return ActionResult{}, &ConflictError{Revision: 7, Current: map[string]string{"title": "saved", "email": "private-email", "secret": "private-secret"}}
	})
	response := guardPostWithManager(t, manager, cookies, guard, map[string]string{"title": "typed"}, true, token)
	result := readResult(t, response)
	if response.Code != http.StatusConflict || result.Redirect != "/admin/items/1" || result.Values["title"] != "typed" {
		t.Fatalf("managed conflict response = %d %#v", response.Code, result)
	}
	var responseData struct {
		Current map[string]string `json:"current"`
	}
	if json.Unmarshal(result.Data, &responseData) != nil || len(responseData.Current) != 1 || responseData.Current["title"] != "saved" || strings.Contains(response.Body.String(), "private-email") || strings.Contains(response.Body.String(), "private-secret") {
		t.Fatalf("managed conflict response exposed a restricted field: %s", response.Body.String())
	}
	var feedback Feedback
	request := httptest.NewRequest(http.MethodGet, "/admin/items/1", nil)
	for _, cookie := range response.Result().Cookies() {
		request.AddCookie(cookie)
	}
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { feedback, _ = ReadFeedback(r, "save") })).ServeHTTP(httptest.NewRecorder(), request)
	if feedback.Conflict == nil || feedback.Conflict.Revision != 7 || feedback.Values["title"] != "typed" || len(feedback.Conflict.Current) != 1 || feedback.Conflict.Current["title"] != "saved" {
		t.Fatalf("managed conflict flash = %#v", feedback)
	}
}

func TestGuardConflictSizeRule(t *testing.T) {
	large := strings.Repeat("x", 1800)
	guard := Guard(guardOptions(), func(context.Context, ActionInput) (ActionResult, error) {
		return ActionResult{}, &ConflictError{Revision: 4, Current: map[string]string{"title": "now"}}
	})
	manager := testSessionManager(t)
	cookies, token := seedSession(t, manager)
	native := guardPostWithManager(t, manager, cookies, guard, map[string]string{"title": large}, false, token)
	for _, cookie := range native.Result().Cookies() {
		cookies = []*http.Cookie{cookie}
	}
	var feedback Feedback
	get := httptest.NewRequest(http.MethodGet, "/admin/items/1", nil)
	for _, cookie := range cookies {
		get.AddCookie(cookie)
	}
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { feedback, _ = ReadFeedback(r, "save") })).ServeHTTP(httptest.NewRecorder(), get)
	if feedback.Values != nil || feedback.Message != "This record changed. Your entries were too long to keep, so check the saved values and enter your change again." {
		t.Fatalf("oversize native feedback = %#v", feedback)
	}
	managed := guardPostWithManager(t, manager, cookies, guard, map[string]string{"title": large}, true, token)
	result := readResult(t, managed)
	if result.Redirect != "" || result.Values["title"] != large {
		t.Fatalf("oversize managed result redirect=%q values=%d", result.Redirect, len(result.Values["title"]))
	}
}

func TestGuardUnavailable(t *testing.T) {
	called := false
	var event AuditEvent
	opts := guardOptions()
	opts.OnError = func(err error) { called = err != nil }
	opts.Auditor = AuditFunc(func(_ context.Context, e AuditEvent) error { event = e; return nil })
	response := guardPost(t, testSession(t), Guard(opts, func(context.Context, ActionInput) (ActionResult, error) {
		return ActionResult{}, errors.New("private db detail")
	}), map[string]string{"title": "Typed"}, true)
	result := readResult(t, response)
	if response.Code != http.StatusServiceUnavailable || result.Message != "We could not save this yet. Please try again." || strings.Contains(response.Body.String(), "private db detail") || !called || event.Outcome != AuditFailed {
		t.Fatalf("unavailable result/audit = %d %#v %#v called=%v", response.Code, result, event, called)
	}
}

func TestGuardSuccessAndReplay(t *testing.T) {
	calls, audits := 0, 0
	opts := guardOptions()
	opts.Auditor = AuditFunc(func(context.Context, AuditEvent) error { audits++; return nil })
	guard := Guard(opts, func(_ context.Context, in ActionInput) (ActionResult, error) {
		calls++
		if in.Key == "0123456789abcdef0123456789abcdef" && calls == 2 {
			return ActionResult{Replayed: true, Message: "Completed."}, nil
		}
		return ActionResult{Message: "Completed."}, nil
	})
	for i := 0; i < 2; i++ {
		response := guardPost(t, testSession(t), guard, map[string]string{"title": "A"}, true)
		result := readResult(t, response)
		if response.Code != http.StatusSeeOther || result.Message != "Completed." || result.Redirect != "/admin/items/1" {
			t.Fatalf("success/replay = %d %#v", response.Code, result)
		}
	}
	if calls != 2 || audits != 0 {
		t.Fatalf("calls=%d audits=%d", calls, audits)
	}
}

func TestGuardEnteredValuesExcludeSecrets(t *testing.T) {
	response := guardPost(t, testSession(t), Guard(guardOptions(), func(context.Context, ActionInput) (ActionResult, error) {
		return ActionResult{}, &ValidationError{Fields: map[string]string{"title": "Check this field."}}
	}), map[string]string{"title": "Typed"}, true)
	result := readResult(t, response)
	for _, key := range []string{CSRFField, KeyField, action.ReturnTargetField} {
		if _, ok := result.Values[key]; ok {
			t.Errorf("secret field %q echoed", key)
		}
	}
	if result.Values["title"] != "Typed" {
		t.Fatalf("entered value omitted: %#v", result.Values)
	}
}

func TestRuntimeHooksPresent(t *testing.T) {
	for _, hook := range []string{"form-error", "form-status", "data-gosx-field-error", "data-gosx-toast-host", "data-gosx-main"} {
		if !strings.Contains(runtimehost.NavigationRuntime, hook) {
			t.Errorf("navigation runtime lacks %q", hook)
		}
	}
}

func guardOptions() GuardOptions {
	return GuardOptions{
		Resource: testResource(), Action: workbench.Action{Name: "save", Label: "Save"},
		Fields:     []workbench.Field{{Name: "title", Label: "Title", Kind: workbench.FieldText, Required: true}},
		Authorizer: testPolicy(), Principal: func(*http.Request) (Principal, error) {
			return Principal{ID: "operator-1", Roles: []string{"operator"}}, nil
		},
		SignInURL: "/sign-in", Fallback: "/admin/items/1", Now: func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) },
	}
}

type sessionFixture struct {
	manager *session.Manager
	cookies []*http.Cookie
	token   string
}

func testSessionManager(t *testing.T) *session.Manager {
	t.Helper()
	manager, err := session.New("test-session-secret-0123456789", session.Options{CookieName: "render-test", Path: "/", AllowInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func seedSession(t *testing.T, manager *session.Manager) ([]*http.Cookie, string) {
	t.Helper()
	var token string
	request := httptest.NewRequest(http.MethodGet, "http://example.test/seed", nil)
	recorder := httptest.NewRecorder()
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { token = session.Token(r) })).ServeHTTP(recorder, request)
	return recorder.Result().Cookies(), token
}

func testSession(t *testing.T) sessionFixture {
	t.Helper()
	manager := testSessionManager(t)
	cookies, token := seedSession(t, manager)
	return sessionFixture{manager: manager, cookies: cookies, token: token}
}

func guardPost(t *testing.T, env sessionFixture, handler action.Handler, extra map[string]string, managed bool) *httptest.ResponseRecorder {
	t.Helper()
	return guardPostWithManager(t, env.manager, env.cookies, handler, extra, managed, env.token)
}

func guardPostWithManager(t *testing.T, manager *session.Manager, cookies []*http.Cookie, handler action.Handler, extra map[string]string, managed bool, tokens ...string) *httptest.ResponseRecorder {
	t.Helper()
	values := url.Values{CSRFField: {""}, KeyField: {"0123456789abcdef0123456789abcdef"}, RecordField: {"record-1"}, RevisionField: {"1"}, action.ReturnTargetField: {"/admin/items/1"}}
	if len(tokens) > 0 {
		values.Set(CSRFField, tokens[0])
	}
	for key, value := range extra {
		values.Set(key, value)
	}
	request := httptest.NewRequest(http.MethodPost, "http://example.test/gosx/action/save", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Referer", "http://example.test/admin/items/1")
	if managed {
		request.Header.Set("Accept", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	manager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { action.ServeHandler(w, r, handler) })).ServeHTTP(recorder, request)
	return recorder
}

func readResult(t *testing.T, response *httptest.ResponseRecorder) action.Result {
	t.Helper()
	var result action.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode action result: %v: %s", err, response.Body.String())
	}
	return result
}
