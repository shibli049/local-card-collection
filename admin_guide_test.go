package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestAdminGuideRequiresLoginAndDoesNotStayCachedAfterLogout(t *testing.T) {
	c := newTestCatalog(t)
	response := publicCall(c, "GET", "/admin-guide.html", "", nil)
	if response.Code != 401 || !strings.Contains(response.Body.String(), "Please sign in") {
		t.Fatal("guide accessible without login", response.Code)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("guide denial may be cached")
	}
	login := publicCall(c, "POST", "/api/login", `{"username":"admin","password":"Inventory@2026"}`, nil)
	cookie := login.Result().Cookies()[0]
	response = publicCall(c, "GET", "/admin-guide.html", "", cookie)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Import an existing folder") {
		t.Fatal("guide unavailable after login", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Header().Get("Content-Type"), "text/html") {
		t.Fatal("incorrect guide response headers")
	}
	if strings.Contains(response.Body.String(), adminPassword) {
		t.Fatal("guide exposes the administrator password")
	}
	publicCall(c, "POST", "/api/logout", `{}`, cookie)
	if response := publicCall(c, "GET", "/admin-guide.html", "", cookie); response.Code != 401 {
		t.Fatal("guide accessible after logout")
	}
}

func TestAdminGuideRendersMarkdownAndBuildsWorkingTopicLinks(t *testing.T) {
	source, err := assets.ReadFile("static/admin-guide.md")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderAdminGuide(source)
	if err != nil {
		t.Fatal(err)
	}
	page := string(rendered)
	for _, expected := range []string{`<h1 id="import-an-existing-folder">`, `<h2 id="connect-the-folder">`, `<ol>`, `<li>`, `<code>catalog.json</code>`, `<blockquote>`} {
		if !strings.Contains(page, expected) {
			t.Fatal("Markdown was not rendered", expected)
		}
	}
	if strings.Contains(page, "{{.Content}}") || strings.Contains(page, "## Get started") {
		t.Fatal("unrendered source shown")
	}
	links := regexp.MustCompile(`href="#([^"]+)"`).FindAllStringSubmatch(page, -1)
	if len(links) != 5 {
		t.Fatal("missing guide topics", len(links))
	}
	for _, link := range links {
		if !strings.Contains(page, `id="`+link[1]+`"`) {
			t.Fatal("topic link has no matching heading", link[1])
		}
	}
}

func TestAdminGuideMarkdownDoesNotRenderScriptsOrUnsafeLinks(t *testing.T) {
	page, err := renderAdminGuide([]byte("# Guide\n\n<script>alert('unsafe')</script>\n\n[Bad link](javascript:alert(1))\n\n## Help\n\nUse **Save details** and `catalog.json`."))
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	if strings.Contains(html, "<script>") || strings.Contains(html, `href="javascript:`) {
		t.Fatal("unsafe Markdown content rendered")
	}
	if !strings.Contains(html, "<strong>Save details</strong>") || !strings.Contains(html, "<code>catalog.json</code>") {
		t.Fatal("normal formatting lost")
	}
}

func TestRawGuideMarkdownIsNotServedPublicly(t *testing.T) {
	c := newTestCatalog(t)
	if response := publicCall(c, "GET", "/admin-guide.md", "", nil); response.Code != 404 {
		t.Fatal("raw guide source is publicly served", response.Code)
	}
}
