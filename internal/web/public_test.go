package web

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"uvoo-minicms/internal/db"
	"uvoo-minicms/internal/httpreq"
)

func TestYouTubeID(t *testing.T) {
	tests := map[string]string{
		"dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://www.youtube.com/shorts/dQw4w9WgXcQ":  "dQw4w9WgXcQ",
		"https://example.com/watch?v=dQw4w9WgXcQ":     "",
		"not-a-valid-video":                           "",
	}
	for input, want := range tests {
		if got := youtubeID(input); got != want {
			t.Fatalf("youtubeID(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestVimeoID(t *testing.T) {
	tests := map[string]string{
		"76979871":                                "76979871",
		"https://vimeo.com/76979871":              "76979871",
		"https://player.vimeo.com/video/76979871": "76979871",
		"https://example.com/video/76979871":      "",
		"https://vimeo.com/not-a-number":          "",
	}
	for input, want := range tests {
		if got := vimeoID(input); got != want {
			t.Fatalf("vimeoID(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestRenderMenuSupportsSectionParents(t *testing.T) {
	html := string(renderMenu([]db.NavItem{
		{ID: "resources", Type: "section", Label: "Resources", Enabled: true},
		{ID: "blog", Type: "link", ParentID: "resources", Label: "Blog", URL: "/blog", Enabled: true},
	}, ""))
	if !strings.Contains(html, `<button class="navSection"`) {
		t.Fatalf("expected section parent button, got %s", html)
	}
	if strings.Contains(html, `href=""`) {
		t.Fatalf("section parent should not render an empty link, got %s", html)
	}
	if !strings.Contains(html, `aria-expanded="false"`) || !strings.Contains(html, `▾`) {
		t.Fatalf("expected accessible chevron disclosure, got %s", html)
	}
}

func TestRenderMenuKeepsLinkParentClickable(t *testing.T) {
	html := string(renderMenu([]db.NavItem{
		{ID: "services", Type: "link", Label: "Services", URL: "/services", Enabled: true},
		{ID: "support", Type: "link", ParentID: "services", Label: "Support", URL: "/services/support", Enabled: true},
	}, ""))
	if !strings.Contains(html, `<a href="/services"`) {
		t.Fatalf("expected parent link to remain clickable, got %s", html)
	}
	if !strings.Contains(html, `class="navToggle"`) {
		t.Fatalf("expected separate submenu toggle, got %s", html)
	}
	if strings.Contains(html, `style="display:grid"`) {
		t.Fatalf("desktop toggle visibility should be controlled by scoped CSS, got %s", html)
	}
	if strings.Contains(html, `<style>`) {
		t.Fatalf("menu markup should not include its own style block, got %s", html)
	}
	if strings.Contains(html, `onclick=`) {
		t.Fatalf("menu markup should use delegated event handling, got %s", html)
	}
}

func TestNavMenuStyleOwnsNavigationBehavior(t *testing.T) {
	assets := navMenuStyle()
	if !strings.Contains(assets, `.nav .navToggle{display:grid!important}`) {
		t.Fatalf("expected top nav toggles to be visible on desktop and mobile, got %s", assets)
	}
	if !strings.Contains(assets, `.nav .navChevron{display:inline}`) {
		t.Fatalf("expected top nav section chevrons to be visible, got %s", assets)
	}
	if !strings.Contains(assets, `.nav .navGroup:hover>.subnav,.nav .navGroup:focus-within>.subnav{display:none}`) {
		t.Fatalf("expected top nav hover/focus expansion to be disabled, got %s", assets)
	}
	if !strings.Contains(assets, `.nav .navGroup.open>.subnav,.nav .navGroup.open:hover>.subnav,.nav .navGroup.open:focus-within>.subnav{display:flex;flex-direction:column}`) {
		t.Fatalf("expected top nav submenus to open only from explicit toggle state, got %s", assets)
	}
	if !strings.Contains(assets, `.drawerNav .navGroup:hover>.subnav,.drawerNav .navGroup:focus-within>.subnav{display:none}`) {
		t.Fatalf("expected drawer hover/focus override so collapsed submenus stay closed, got %s", assets)
	}
	if !strings.Contains(assets, `document.addEventListener('click'`) || !strings.Contains(assets, `.navToggle,.navSection`) {
		t.Fatalf("expected one delegated nav click handler, got %s", assets)
	}
	if !strings.Contains(assets, `g.parentElement.children`) || !strings.Contains(assets, `closeGroup(s)`) {
		t.Fatalf("expected opening one submenu to close sibling submenus, got %s", assets)
	}
	if !strings.Contains(assets, `closeAll()`) || !strings.Contains(assets, `e.key==='Escape'`) {
		t.Fatalf("expected outside-click and Escape cleanup for open submenus, got %s", assets)
	}
	if !strings.Contains(assets, `a[aria-current=page]`) {
		t.Fatalf("expected active page styling, got %s", assets)
	}
}

func TestRenderMenuMarksActivePage(t *testing.T) {
	html := string(renderMenu([]db.NavItem{
		{ID: "home", Type: "link", Label: "Home", URL: "/", Enabled: true},
		{ID: "support", Type: "link", Label: "Support", URL: "/support/", Enabled: true},
		{ID: "external", Type: "link", Label: "External", URL: "https://example.com/support", External: true, Enabled: true},
	}, "/support"))
	if !strings.Contains(html, `<a href="/support/" aria-current="page">Support</a>`) {
		t.Fatalf("expected current page marker on matching internal link, got %s", html)
	}
	if strings.Contains(html, `https://example.com/support" target="_blank" rel="noopener noreferrer" aria-current`) {
		t.Fatalf("external links should not be marked active, got %s", html)
	}
}

func TestRenderMenuCacheKeysByItemsAndCurrentPath(t *testing.T) {
	pub := NewPublic(nil, "Demo")
	first := string(pub.renderMenuCached([]db.NavItem{
		{ID: "home", Type: "link", Label: "Home", URL: "/", Enabled: true},
		{ID: "support", Type: "link", Label: "Support", URL: "/support", Enabled: true},
	}, "/support"))
	if !strings.Contains(first, `href="/support" aria-current="page">Support</a>`) {
		t.Fatalf("expected active support item, got %s", first)
	}

	second := string(pub.renderMenuCached([]db.NavItem{
		{ID: "home", Type: "link", Label: "Start", URL: "/", Enabled: true},
		{ID: "support", Type: "link", Label: "Support", URL: "/support", Enabled: true},
	}, "/"))
	if !strings.Contains(second, `href="/" aria-current="page">Start</a>`) {
		t.Fatalf("expected cache to reflect changed menu/current path, got %s", second)
	}
}

func TestPublicRenderInlineCode(t *testing.T) {
	body, err := NewPublic(nil, "Demo").render("Use `CMS_ADDR` for the bind address.\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `<code>CMS_ADDR</code>`) {
		t.Fatalf("expected inline code element, got %s", body)
	}
}

func TestPublicRenderFencedCodeHighlightsSyntax(t *testing.T) {
	body, err := NewPublic(nil, "Demo").render("```python\nclass Dog:\n    pass\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	if !strings.Contains(html, `<span`) || !strings.Contains(html, `color:`) {
		t.Fatalf("expected syntax-highlighted spans, got %s", html)
	}
	if !strings.Contains(html, `Dog`) {
		t.Fatalf("expected code content to be preserved, got %s", html)
	}
}

func TestPublicRenderMermaidCodeKeepsLanguageClass(t *testing.T) {
	body, err := NewPublic(nil, "Demo").render("```mermaid\ngraph TD\n  A-->B\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `language-mermaid`) {
		t.Fatalf("expected mermaid language class for client renderer hook, got %s", body)
	}
}

func TestPublicTemplateStylesInlineCode(t *testing.T) {
	var b bytes.Buffer
	err := publicTpl.Execute(&b, map[string]any{
		"SiteName": "Test",
		"Title":    "Home",
		"Body":     template.HTML(`<p>Use <code>CMS_ADDR</code>.</p>`),
	})
	if err != nil {
		t.Fatalf("execute public template: %v", err)
	}
	html := b.String()
	if !strings.Contains(html, `:not(pre)&gt;code`) && !strings.Contains(html, `:not(pre)>code`) {
		t.Fatalf("expected inline code CSS in template, got %s", html)
	}
	if !strings.Contains(html, `pre code`) {
		t.Fatalf("expected code block CSS reset in template, got %s", html)
	}
}

func TestPublicTemplateSideNavDoesNotRenderHiddenTopMenu(t *testing.T) {
	menu := renderMenu([]db.NavItem{
		{ID: "services", Type: "section", Label: "Services", Enabled: true},
		{ID: "support", Type: "link", ParentID: "services", Label: "Support", URL: "/support", Enabled: true},
	}, "/support")
	var b bytes.Buffer
	err := publicTpl.Execute(&b, map[string]any{
		"SiteName":     "Test",
		"Title":        "Home",
		"Body":         template.HTML("<p>Home</p>"),
		"MenuHTML":     menu,
		"NavMenuStyle": template.HTML(navMenuStyle()),
		"MenuEnabled":  true,
		"SideNav":      true,
	})
	if err != nil {
		t.Fatalf("execute public template: %v", err)
	}
	html := b.String()
	if !strings.Contains(html, `class="drawerNav"`) {
		t.Fatalf("expected drawer nav, got %s", html)
	}
	if strings.Contains(html, `id="nav"`) {
		t.Fatalf("side nav should not render hidden top nav, got %s", html)
	}
	if count := strings.Count(html, `id="nav-sub-services"`); count != 1 {
		t.Fatalf("expected one submenu id in side nav mode, got %d in %s", count, html)
	}
}

func TestPublicBlogRouteListsPublishedPosts(t *testing.T) {
	store, err := db.Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()
	settings, err := store.GetSettings(ctx, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	settings.BlogEnabled = true
	settings.BlogPath = "/blog"
	settings.BlogTitle = "News"
	settings.BlogMenuEnabled = true
	if _, err := store.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	for _, page := range []db.Page{
		{Slug: "new", Path: "/blog/new", Title: "New Post", ContentType: "post", Published: true, PublishedAt: "2026-02-01"},
		{Slug: "old", Path: "/blog/old", Title: "Old Post", ContentType: "post", Published: true, PublishedAt: "2026-01-01"},
		{Slug: "draft", Path: "/blog/draft", Title: "Draft Post", ContentType: "post", Published: false},
		{Slug: "page", Path: "/page", Title: "Regular Page", ContentType: "page", Published: true},
	} {
		if _, err := store.SavePage(ctx, page); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/blog", nil)
	rec := httptest.NewRecorder()
	NewPublic(store, "Demo").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	html := rec.Body.String()
	if !strings.Contains(html, "News") || !strings.Contains(html, "New Post") || !strings.Contains(html, "Old Post") {
		t.Fatalf("expected blog posts in response, got %s", html)
	}
	if !strings.Contains(html, `<link rel="alternate" type="application/rss+xml"`) || !strings.Contains(html, `href="/blog/feed.xml"`) {
		t.Fatalf("expected RSS discovery link, got %s", html)
	}
	if strings.Index(html, "New Post") > strings.Index(html, "Old Post") {
		t.Fatalf("expected newest post first, got %s", html)
	}
	if strings.Contains(html, "Draft Post") || strings.Contains(html, "Regular Page") {
		t.Fatalf("drafts and pages should not render on blog index, got %s", html)
	}
}

func TestPublicBlogFeedListsPublishedPosts(t *testing.T) {
	store, err := db.Open(t.TempDir() + "/cms.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	ctx := context.Background()
	settings, err := store.GetSettings(ctx, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	settings.BlogEnabled = true
	settings.BlogPath = "/news"
	settings.BlogTitle = "News"
	if _, err := store.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	for _, page := range []db.Page{
		{Slug: "new", Path: "/news/new", Title: "New & Good", MetaDescription: "Latest <update>", ContentType: "post", Published: true, PublishedAt: "2026-02-01"},
		{Slug: "draft", Path: "/news/draft", Title: "Draft Post", ContentType: "post", Published: false},
	} {
		if _, err := store.SavePage(ctx, page); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.test/news/feed.xml", nil)
	rec := httptest.NewRecorder()
	NewPublic(store, "Demo").ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/rss+xml") {
		t.Fatalf("expected RSS content type, got %q", got)
	}
	xml := rec.Body.String()
	for _, want := range []string{
		`<rss version="2.0">`,
		`<title>New &amp; Good</title>`,
		`<link>https://example.test/news/new</link>`,
		`<description>Latest &lt;update&gt;</description>`,
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("expected %q in feed, got %s", want, xml)
		}
	}
	if strings.Contains(xml, "Draft Post") {
		t.Fatalf("draft should not render in feed, got %s", xml)
	}
}

func TestPublicBaseURLOnlyTrustsForwardedHeadersWhenEnabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://internal.test/blog/feed.xml", nil)
	req.Host = "internal.test"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "public.example")

	if got := httpreq.BaseURL(req, false); got != "http://internal.test" {
		t.Fatalf("expected untrusted forwarded headers to be ignored, got %q", got)
	}
	if got := httpreq.BaseURL(req, true); got != "https://public.example" {
		t.Fatalf("expected trusted forwarded headers to be used, got %q", got)
	}
}
