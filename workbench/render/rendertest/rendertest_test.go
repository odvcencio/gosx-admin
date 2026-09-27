package rendertest

import "testing"

func TestCheckPagePassesAccessibleShell(t *testing.T) {
	doc := `<div><a href="#main">Skip to content</a><header>Brand</header><nav aria-label="Main"><a href="/">Home</a></nav><main id="main"><h1>Home</h1></main><footer>Help</footer></div>`
	if problems := Check(doc, Options{Page: true}); len(problems) != 0 {
		t.Fatalf("Check() = %#v", problems)
	}
}

func TestRendertestFindsEachRule(t *testing.T) {
	cases := []struct {
		rule string
		doc  string
		page bool
	}{
		{"nav-name", `<nav><a href="/home">Home</a></nav>`, false},
		{"unique-id", `<p id="same"></p><p id="same"></p>`, false},
		{"label", `<input name="title">`, false},
		{"button-name", `<button></button>`, false},
		{"link-name", `<a href="/"></a>`, false},
		{"describedby", `<p aria-describedby="missing">Text</p>`, false},
		{"img-alt", `<img src="/image">`, false},
		{"heading-order", `<h2>Section</h2><h4>Deep section</h4>`, false},
		{"table-headers", `<table><tr><td>Value</td></tr></table>`, false},
		{"no-h1", `<h1>Fragment heading</h1>`, false},
		{"one-main", `<a href="#main">Skip</a><main id="main"><h1>One</h1></main><main><h2>Two</h2></main>`, true},
		{"one-h1", `<a href="#main">Skip</a><main id="main"><h2>Page</h2></main>`, true},
		{"skip-link", `<a href="/elsewhere">First link</a><main id="main"><h1>Page</h1></main>`, true},
		{"landmarks", `<a href="#main">Skip</a><header>One</header><header>Two</header><main id="main"><h1>Page</h1></main>`, true},
	}
	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			problems := Check(tc.doc, Options{Page: tc.page})
			for _, problem := range problems {
				if problem.Rule == tc.rule {
					return
				}
			}
			t.Fatalf("rule %q missing from %#v", tc.rule, problems)
		})
	}
}
