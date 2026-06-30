package web

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"uvoo-minicms/internal/db"
	"uvoo-minicms/internal/httpreq"
)

type Public struct {
	Store       *db.Store
	SiteName    string
	TrustProxy  bool
	md          goldmark.Markdown
	renderMu    sync.RWMutex
	renderCache map[string]template.HTML
}

const maxRenderCacheEntries = 256

func NewPublic(st *db.Store, siteName string) *Public {
	return &Public{Store: st, SiteName: siteName, md: goldmark.New(goldmark.WithExtensions(
		extension.GFM,
		highlighting.NewHighlighting(
			highlighting.WithStyle("monokai"),
			highlighting.WithFormatOptions(chromahtml.WithLineNumbers(false)),
		),
	))}
}

func (p *Public) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/admin" {
		http.Redirect(w, r, "/admin/", http.StatusFound)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/admin/") || strings.HasPrefix(r.URL.Path, "/uploads/") {
		http.NotFound(w, r)
		return
	}
	settings, err := p.Store.GetSettings(r.Context(), p.SiteName)
	if err != nil {
		http.Error(w, "settings error", 500)
		return
	}
	if r.URL.Path == "/search" {
		p.serveSearch(w, r, settings)
		return
	}
	routePath := "/" + strings.Trim(r.URL.Path, "/")
	if routePath == "/" {
		routePath = "/"
	}
	menu := menuWithBlog(settings)
	if settings.BlogEnabled && cleanNavPath(routePath) == blogFeedPath(settings) {
		p.serveBlogFeed(w, r, settings)
		return
	}
	if settings.BlogEnabled && cleanNavPath(routePath) == cleanNavPath(settings.BlogPath) {
		posts, err := p.Store.ListPublishedPosts(r.Context(), settings.BlogPostsPerPage)
		if err != nil {
			http.Error(w, "blog error", 500)
			return
		}
		footer, err := p.renderFooter(settings)
		if err != nil {
			http.Error(w, "render error", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = publicTpl.Execute(w, map[string]any{
			"SiteName":             settings.SiteName,
			"Title":                settings.BlogTitle,
			"MetaDescription":      "",
			"Body":                 renderBlogIndex(settings, posts),
			"Footer":               footer,
			"MenuHTML":             p.renderMenuCached(menu, routePath),
			"NavMenuStyle":         template.HTML(navMenuStyle()),
			"SearchHTML":           "",
			"LogoURL":              settings.LogoURL,
			"FaviconURL":           settings.FaviconURL,
			"DefaultTheme":         settings.DefaultTheme,
			"PublicThemeStyle":     settings.PublicThemeStyle,
			"PublicPrimaryColor":   settings.PublicPrimaryColor,
			"PublicSecondaryColor": settings.PublicSecondaryColor,
			"PublicHeaderStyle":    settings.PublicHeaderStyle,
			"HeaderClass":          publicHeaderClass(settings.PublicHeaderStyle),
			"LogoEnabled":          settings.LogoEnabled,
			"FaviconEnabled":       settings.FaviconEnabled,
			"MenuEnabled":          settings.MenuEnabled,
			"FooterEnabled":        settings.FooterEnabled,
			"ThemeToggleEnabled":   settings.ThemeToggleEnabled,
			"IconsEnabled":         settings.IconsEnabled,
			"SearchEnabled":        settings.SearchEnabled,
			"NavLayout":            settings.NavLayout,
			"SideNav":              settings.NavLayout == "side",
			"HasMermaid":           false,
			"RSSURL":               rssURL(settings),
		})
		return
	}
	page, err := p.Store.GetPublishedByPath(r.Context(), routePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body, err := p.renderCached(renderCacheKey("page", page.UpdatedAt, page.Markdown), page.Markdown)
	if err != nil {
		http.Error(w, "render error", 500)
		return
	}
	var footer template.HTML
	if settings.FooterEnabled {
		footer, err = p.renderFooter(settings)
		if err != nil {
			http.Error(w, "render error", 500)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = publicTpl.Execute(w, map[string]any{
		"SiteName":             settings.SiteName,
		"Title":                page.Title,
		"MetaDescription":      page.MetaDescription,
		"Body":                 body,
		"Footer":               footer,
		"MenuHTML":             p.renderMenuCached(menu, routePath),
		"NavMenuStyle":         template.HTML(navMenuStyle()),
		"SearchHTML":           "",
		"LogoURL":              settings.LogoURL,
		"FaviconURL":           settings.FaviconURL,
		"DefaultTheme":         settings.DefaultTheme,
		"PublicThemeStyle":     settings.PublicThemeStyle,
		"PublicPrimaryColor":   settings.PublicPrimaryColor,
		"PublicSecondaryColor": settings.PublicSecondaryColor,
		"PublicHeaderStyle":    settings.PublicHeaderStyle,
		"HeaderClass":          publicHeaderClass(settings.PublicHeaderStyle),
		"LogoEnabled":          settings.LogoEnabled,
		"FaviconEnabled":       settings.FaviconEnabled,
		"MenuEnabled":          settings.MenuEnabled,
		"FooterEnabled":        settings.FooterEnabled,
		"ThemeToggleEnabled":   settings.ThemeToggleEnabled,
		"IconsEnabled":         settings.IconsEnabled,
		"SearchEnabled":        settings.SearchEnabled,
		"NavLayout":            settings.NavLayout,
		"SideNav":              settings.NavLayout == "side",
		"HasMermaid":           strings.Contains(page.Markdown, "```mermaid"),
		"RSSURL":               rssURL(settings),
	})
}

func (p *Public) serveSearch(w http.ResponseWriter, r *http.Request, settings db.Settings) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	pages, err := p.Store.SearchPages(r.Context(), query)
	if err != nil {
		http.Error(w, "search error", 500)
		return
	}
	var b strings.Builder
	b.WriteString(`<h1>Search</h1><form class="searchPage" action="/search" method="get"><input name="q" value="`)
	b.WriteString(template.HTMLEscapeString(query))
	b.WriteString(`" placeholder="Search pages and posts"><button type="submit">Search</button></form>`)
	if query != "" {
		fmt.Fprintf(&b, `<p class="muted">%d result(s) for <strong>%s</strong></p>`, len(pages), template.HTMLEscapeString(query))
		b.WriteString(`<div class="resultList">`)
		for _, page := range pages {
			fmt.Fprintf(&b, `<a class="result" href="%s"><span>%s</span><strong>%s</strong><small>%s</small></a>`, template.HTMLEscapeString(page.Path), template.HTMLEscapeString(page.ContentType), template.HTMLEscapeString(page.Title), template.HTMLEscapeString(firstNonEmpty(page.MetaDescription, page.Tags, page.Path)))
		}
		b.WriteString(`</div>`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = publicTpl.Execute(w, map[string]any{
		"SiteName":             settings.SiteName,
		"Title":                "Search",
		"Body":                 template.HTML(b.String()),
		"Footer":               template.HTML(""),
		"MenuHTML":             p.renderMenuCached(menuWithBlog(settings), "/search"),
		"NavMenuStyle":         template.HTML(navMenuStyle()),
		"LogoURL":              settings.LogoURL,
		"FaviconURL":           settings.FaviconURL,
		"DefaultTheme":         settings.DefaultTheme,
		"PublicThemeStyle":     settings.PublicThemeStyle,
		"PublicPrimaryColor":   settings.PublicPrimaryColor,
		"PublicSecondaryColor": settings.PublicSecondaryColor,
		"PublicHeaderStyle":    settings.PublicHeaderStyle,
		"HeaderClass":          publicHeaderClass(settings.PublicHeaderStyle),
		"LogoEnabled":          settings.LogoEnabled,
		"FaviconEnabled":       settings.FaviconEnabled,
		"MenuEnabled":          settings.MenuEnabled,
		"FooterEnabled":        false,
		"ThemeToggleEnabled":   settings.ThemeToggleEnabled,
		"IconsEnabled":         settings.IconsEnabled,
		"SearchEnabled":        settings.SearchEnabled,
		"NavLayout":            settings.NavLayout,
		"SideNav":              settings.NavLayout == "side",
		"RSSURL":               rssURL(settings),
	})
}

func (p *Public) serveBlogFeed(w http.ResponseWriter, r *http.Request, settings db.Settings) {
	posts, err := p.Store.ListPublishedPosts(r.Context(), settings.BlogPostsPerPage)
	if err != nil {
		http.Error(w, "feed error", 500)
		return
	}
	base := httpreq.BaseURL(r, p.TrustProxy)
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0"><channel>`)
	writeXMLElement(&b, "title", firstNonEmpty(settings.BlogTitle, "Blog"))
	writeXMLElement(&b, "link", absoluteURL(base, cleanNavPath(settings.BlogPath)))
	writeXMLElement(&b, "description", firstNonEmpty(settings.BlogTitle, "Blog"))
	if len(posts) > 0 {
		writeXMLElement(&b, "lastBuildDate", rssDate(posts[0]))
	}
	for _, post := range posts {
		b.WriteString(`<item>`)
		writeXMLElement(&b, "title", post.Title)
		writeXMLElement(&b, "link", absoluteURL(base, post.Path))
		fmt.Fprintf(&b, `<guid isPermaLink="true">%s</guid>`, escapeXML(absoluteURL(base, post.Path)))
		writeXMLElement(&b, "pubDate", rssDate(post))
		writeXMLElement(&b, "description", firstNonEmpty(post.MetaDescription, post.Tags))
		b.WriteString(`</item>`)
	}
	b.WriteString(`</channel></rss>`)
	_, _ = w.Write([]byte(b.String()))
}

func (p *Public) renderFooter(settings db.Settings) (template.HTML, error) {
	if !settings.FooterEnabled {
		return "", nil
	}
	return p.renderCached(renderCacheKey("footer", "", settings.FooterMarkdown), settings.FooterMarkdown)
}

func rssURL(settings db.Settings) string {
	if !settings.BlogEnabled {
		return ""
	}
	return blogFeedPath(settings)
}

func blogFeedPath(settings db.Settings) string {
	return cleanNavPath(firstNonEmpty(settings.BlogPath, "/blog")) + "/feed.xml"
}

func absoluteURL(base, path string) string {
	u, err := url.Parse(path)
	if err == nil && u.IsAbs() {
		return u.String()
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return path
	}
	ref := &url.URL{Path: cleanNavPath(path)}
	return baseURL.ResolveReference(ref).String()
}

func rssDate(post db.Page) string {
	value := firstNonEmpty(post.PublishedAt, post.CreatedAt)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC().Format(time.RFC1123Z)
		}
	}
	return time.Now().UTC().Format(time.RFC1123Z)
}

func writeXMLElement(b *strings.Builder, name, value string) {
	fmt.Fprintf(b, `<%s>%s</%s>`, name, escapeXML(value), name)
}

func escapeXML(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func renderBlogIndex(settings db.Settings, posts []db.Page) template.HTML {
	var b strings.Builder
	title := firstNonEmpty(settings.BlogTitle, "Blog")
	fmt.Fprintf(&b, `<h1>%s</h1>`, template.HTMLEscapeString(title))
	if len(posts) == 0 {
		b.WriteString(`<p class="muted">No published posts yet.</p>`)
		return template.HTML(b.String())
	}
	b.WriteString(`<div class="resultList">`)
	for _, post := range posts {
		fmt.Fprintf(&b, `<a class="result" href="%s"><span>%s</span><strong>%s</strong><small>%s</small></a>`,
			template.HTMLEscapeString(post.Path),
			template.HTMLEscapeString(blogDate(post)),
			template.HTMLEscapeString(post.Title),
			template.HTMLEscapeString(firstNonEmpty(post.MetaDescription, post.Tags, post.Path)),
		)
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

func blogDate(post db.Page) string {
	value := firstNonEmpty(post.PublishedAt, post.CreatedAt)
	if len(value) >= len("2006-01-02") {
		return value[:len("2006-01-02")]
	}
	return "Post"
}

func menuWithBlog(settings db.Settings) []db.NavItem {
	if !settings.BlogEnabled || !settings.BlogMenuEnabled {
		return settings.Menu
	}
	blogPath := cleanNavPath(firstNonEmpty(settings.BlogPath, "/blog"))
	for _, item := range settings.Menu {
		if !item.Enabled || item.Type == "section" || item.External {
			continue
		}
		u, err := url.Parse(item.URL)
		if err == nil && !u.IsAbs() && cleanNavPath(u.Path) == blogPath {
			return settings.Menu
		}
	}
	menu := append([]db.NavItem{}, settings.Menu...)
	menu = append(menu, db.NavItem{
		ID:      "blog",
		Type:    "link",
		Label:   firstNonEmpty(settings.BlogTitle, "Blog"),
		URL:     blogPath,
		Enabled: true,
	})
	return menu
}

func (p *Public) render(markdown string) (template.HTML, error) {
	markdown, replacements := p.expandRichMarkdown(markdown)
	var b bytes.Buffer
	if err := p.md.Convert([]byte(markdown), &b); err != nil {
		return "", err
	}
	html := b.String()
	for token, replacement := range replacements {
		html = strings.ReplaceAll(html, "<p>"+token+"</p>", replacement)
		html = strings.ReplaceAll(html, token, replacement)
	}
	return template.HTML(html), nil
}

func (p *Public) renderCached(key, markdown string) (template.HTML, error) {
	return p.cachedHTML(key, func() (template.HTML, error) {
		return p.render(markdown)
	})
}

func (p *Public) renderMenuCached(items []db.NavItem, currentPath string) template.HTML {
	html, _ := p.cachedHTML(menuCacheKey(items, currentPath), func() (template.HTML, error) {
		return renderMenu(items, currentPath), nil
	})
	return html
}

func (p *Public) cachedHTML(key string, render func() (template.HTML, error)) (template.HTML, error) {
	p.renderMu.RLock()
	if p.renderCache != nil {
		if html, ok := p.renderCache[key]; ok {
			p.renderMu.RUnlock()
			return html, nil
		}
	}
	p.renderMu.RUnlock()

	html, err := render()
	if err != nil {
		return "", err
	}
	p.renderMu.Lock()
	if p.renderCache == nil || len(p.renderCache) >= maxRenderCacheEntries {
		p.renderCache = map[string]template.HTML{}
	}
	p.renderCache[key] = html
	p.renderMu.Unlock()
	return html, nil
}

func renderCacheKey(scope, version, markdown string) string {
	sum := sha1.Sum([]byte(markdown))
	return fmt.Sprintf("%s:%s:%x", scope, version, sum)
}

func menuCacheKey(items []db.NavItem, currentPath string) string {
	raw, err := json.Marshal(struct {
		Items       []db.NavItem `json:"items"`
		CurrentPath string       `json:"current_path"`
	}{Items: items, CurrentPath: cleanNavPath(currentPath)})
	if err != nil {
		raw = []byte(currentPath)
	}
	sum := sha1.Sum(raw)
	return fmt.Sprintf("menu:%x", sum)
}

func (p *Public) expandRichMarkdown(markdown string) (string, map[string]string) {
	replacements := map[string]string{}
	markdown = p.expandCards(markdown, replacements)
	markdown = expandMediaEmbeds(markdown, replacements)
	markdown = expandIcons(markdown, replacements)
	return markdown, replacements
}

var cardStartRe = regexp.MustCompile(`^:::card(?:\s+(.*))?$`)
var attrRe = regexp.MustCompile(`([a-zA-Z_]+)="([^"]*)"`)
var iconRe = regexp.MustCompile(`\{\{icon:([a-zA-Z0-9 -]+)\}\}`)
var mediaEmbedRe = regexp.MustCompile(`\{\{(youtube|vimeo):([^}]+)\}\}`)
var youtubeIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
var vimeoIDRe = regexp.MustCompile(`^[0-9]{6,12}$`)

func (p *Public) expandCards(markdown string, replacements map[string]string) string {
	lines := strings.Split(markdown, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		matches := cardStartRe.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if matches == nil {
			out = append(out, lines[i])
			continue
		}
		attrs := parseAttrs(matches[1])
		var bodyLines []string
		for i++; i < len(lines) && strings.TrimSpace(lines[i]) != ":::"; i++ {
			bodyLines = append(bodyLines, lines[i])
		}
		var body bytes.Buffer
		if err := p.md.Convert([]byte(strings.Join(bodyLines, "\n")), &body); err != nil {
			body.WriteString(template.HTMLEscapeString(strings.Join(bodyLines, "\n")))
		}
		token := fmt.Sprintf("UVOO_MINICMS_CARD_%d", len(replacements))
		replacements[token] = renderCard(attrs, body.String())
		out = append(out, token)
	}
	return strings.Join(out, "\n")
}

func expandIcons(markdown string, replacements map[string]string) string {
	return iconRe.ReplaceAllStringFunc(markdown, func(match string) string {
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{icon:"), "}}"))
		token := fmt.Sprintf("UVOO_MINICMS_ICON_%d", len(replacements))
		replacements[token] = renderIcon(name)
		return token
	})
}

func expandMediaEmbeds(markdown string, replacements map[string]string) string {
	return mediaEmbedRe.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := mediaEmbedRe.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		html := renderMediaEmbed(strings.ToLower(parts[1]), strings.TrimSpace(parts[2]))
		if html == "" {
			return ""
		}
		token := fmt.Sprintf("UVOO_MINICMS_MEDIA_%d", len(replacements))
		replacements[token] = html
		return token
	})
}

func parseAttrs(raw string) map[string]string {
	attrs := map[string]string{}
	for _, match := range attrRe.FindAllStringSubmatch(raw, -1) {
		attrs[strings.ToLower(match[1])] = strings.TrimSpace(match[2])
	}
	return attrs
}

func renderCard(attrs map[string]string, body string) string {
	title := template.HTMLEscapeString(attrs["title"])
	icon := renderIcon(attrs["icon"])
	var b strings.Builder
	b.WriteString(`<section class="cms-card">`)
	if title != "" || attrs["icon"] != "" {
		b.WriteString(`<header class="cms-card-head">`)
		if attrs["icon"] != "" {
			b.WriteString(icon)
		}
		if title != "" {
			b.WriteString(`<h3>`)
			b.WriteString(title)
			b.WriteString(`</h3>`)
		}
		b.WriteString(`</header>`)
	}
	b.WriteString(`<div class="cms-card-body">`)
	b.WriteString(body)
	b.WriteString(`</div></section>`)
	return b.String()
}

func renderIcon(name string) string {
	className := sanitizeIconClass(name)
	if className == "" {
		return ""
	}
	return `<span class="cms-icon"><i class="` + template.HTMLEscapeString(className) + `" aria-hidden="true"></i></span>`
}

func sanitizeIconClass(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`[^a-z0-9 -]`).ReplaceAllString(name, "")
	if name == "" {
		return ""
	}
	if strings.Contains(name, "fa-") {
		return strings.Join(strings.Fields(name), " ")
	}
	return "fa-solid fa-" + strings.ReplaceAll(name, " ", "-")
}

func renderMediaEmbed(provider, raw string) string {
	var src, title string
	switch provider {
	case "youtube":
		id := youtubeID(raw)
		if id == "" {
			return ""
		}
		src = "https://www.youtube-nocookie.com/embed/" + id
		title = "YouTube video"
	case "vimeo":
		id := vimeoID(raw)
		if id == "" {
			return ""
		}
		src = "https://player.vimeo.com/video/" + id
		title = "Vimeo video"
	default:
		return ""
	}
	return `<div class="cms-embed" style="position:relative;aspect-ratio:16/9;margin:24px 0;background:#020617;border-radius:var(--radius-sm);overflow:hidden;box-shadow:0 14px 34px var(--shadow)"><iframe src="` + template.HTMLEscapeString(src) + `" title="` + title + `" loading="lazy" style="position:absolute;inset:0;width:100%;height:100%;border:0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen referrerpolicy="strict-origin-when-cross-origin"></iframe></div>`
}

func youtubeID(raw string) string {
	raw = strings.TrimSpace(raw)
	if youtubeIDRe.MatchString(raw) {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	switch host {
	case "youtube.com", "m.youtube.com", "music.youtube.com":
		if id := u.Query().Get("v"); youtubeIDRe.MatchString(id) {
			return id
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 && (parts[0] == "embed" || parts[0] == "shorts") && youtubeIDRe.MatchString(parts[1]) {
			return parts[1]
		}
	case "youtu.be":
		id := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
		if youtubeIDRe.MatchString(id) {
			return id
		}
	}
	return ""
}

func vimeoID(raw string) string {
	raw = strings.TrimSpace(raw)
	if vimeoIDRe.MatchString(raw) {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	if host != "vimeo.com" && host != "player.vimeo.com" {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if vimeoIDRe.MatchString(parts[i]) {
			return parts[i]
		}
	}
	return ""
}

func renderMenu(items []db.NavItem, currentPath string) template.HTML {
	children := map[string][]db.NavItem{}
	for _, item := range items {
		if item.Enabled {
			children[item.ParentID] = append(children[item.ParentID], item)
		}
	}
	var b strings.Builder
	writeMenu(&b, children, "", currentPath)
	return template.HTML(b.String())
}

func writeMenu(b *strings.Builder, children map[string][]db.NavItem, parentID, currentPath string) {
	for _, item := range children[parentID] {
		kids := children[item.ID]
		label := template.HTMLEscapeString(item.Label)
		subnavID := template.HTMLEscapeString("nav-sub-" + item.ID)
		if len(kids) > 0 {
			if item.Type == "section" {
				fmt.Fprintf(b, `<div class="navGroup"><button class="navSection" type="button" aria-expanded="false" aria-controls="%s" style="display:flex;align-items:center;gap:6px;border:0;background:transparent;color:var(--ink);padding:8px 11px;border-radius:var(--radius-pill);cursor:pointer;font:inherit;font-weight:600"><span>%s</span><span class="navChevron" aria-hidden="true">▾</span></button><div class="subnav" id="%s">`, subnavID, label, subnavID)
			} else {
				fmt.Fprintf(b, `<div class="navGroup"><div class="navParent"><a href="%s"%s%s>%s</a><button class="navToggle" type="button" aria-label="Toggle %s submenu" aria-expanded="false" aria-controls="%s">▾</button></div><div class="subnav" id="%s">`, template.HTMLEscapeString(item.URL), externalAttrs(item.External), activeAttrs(item, currentPath), label, label, subnavID, subnavID)
			}
			writeMenu(b, children, item.ID, currentPath)
			b.WriteString(`</div></div>`)
			continue
		}
		if item.Type == "section" {
			fmt.Fprintf(b, `<span class="navSectionLabel" style="color:var(--muted);padding:8px 11px;font-weight:700">%s</span>`, label)
			continue
		}
		fmt.Fprintf(b, `<a href="%s"%s%s>%s</a>`, template.HTMLEscapeString(item.URL), externalAttrs(item.External), activeAttrs(item, currentPath), template.HTMLEscapeString(item.Label))
	}
}

func navMenuStyle() string {
	return `<style>.nav .navToggle{display:grid!important}.nav .navChevron{display:inline}.nav a[aria-current=page],.drawerNav a[aria-current=page]{background:color-mix(in srgb,var(--accent) 14%,var(--soft));color:var(--accent);font-weight:800}.nav .navSection:hover,.nav .navSection:focus{background:var(--soft);outline:0}.nav .navGroup:hover>.subnav,.nav .navGroup:focus-within>.subnav{display:none}.nav .navGroup.open>.subnav,.nav .navGroup.open:hover>.subnav,.nav .navGroup.open:focus-within>.subnav{display:flex;flex-direction:column}.drawerNav .navToggle{display:grid!important}.drawerNav .navChevron{display:inline}.drawerNav .navSection{width:100%;justify-content:space-between;border-radius:var(--radius-sm)!important;padding:10px 12px!important}.drawerNav .navSection:hover,.drawerNav .navSection:focus{background:var(--soft);outline:0}.drawerNav .navGroup:hover>.subnav,.drawerNav .navGroup:focus-within>.subnav{display:none}.drawerNav .navGroup.open>.subnav,.drawerNav .navGroup.open:hover>.subnav,.drawerNav .navGroup.open:focus-within>.subnav{display:flex;flex-direction:column}.siteSide .drawerBtn{order:-2;display:inline-grid!important}.siteSide .brand{margin-right:auto}@media(max-width:720px){.nav .navSection{width:100%;justify-content:space-between;padding:11px 12px!important;border-radius:var(--radius-sm)!important}.nav .navSectionLabel{padding:11px 12px!important}}</style><script>(function(){function resetToggle(g){var b=g.querySelector('.navSection,.navToggle');if(b){b.setAttribute('aria-expanded','false');var c=b.querySelector('.navChevron');if(c){c.textContent='▾'}else{b.textContent='▾'}}}function closeGroup(g){g.classList.remove('open');resetToggle(g)}function closeAll(){Array.prototype.forEach.call(document.querySelectorAll('.navGroup.open'),closeGroup)}document.addEventListener('click',function(e){var t=e.target.closest&&e.target.closest('.navToggle,.navSection');if(!t){if(e.target.closest&&!e.target.closest('.navGroup')){closeAll()}return}var g=t.closest('.navGroup');if(!g){return}var open=!g.classList.contains('open');if(open&&g.parentElement){Array.prototype.forEach.call(g.parentElement.children,function(s){if(s!==g&&s.classList&&s.classList.contains('navGroup')){closeGroup(s)}})}g.classList.toggle('open',open);t.setAttribute('aria-expanded',open?'true':'false');var c=t.querySelector('.navChevron');if(c){c.textContent=open?'▴':'▾'}else{t.textContent=open?'▴':'▾'}});document.addEventListener('keydown',function(e){if(e.key==='Escape'){closeAll()}})})()</script>`
}

func activeAttrs(item db.NavItem, currentPath string) string {
	if item.Type == "section" || item.External || currentPath == "" {
		return ""
	}
	u, err := url.Parse(item.URL)
	if err != nil || u.IsAbs() {
		return ""
	}
	if cleanNavPath(u.Path) == cleanNavPath(currentPath) {
		return ` aria-current="page"`
	}
	return ""
}

func cleanNavPath(path string) string {
	path = "/" + strings.Trim(path, "/")
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func externalAttrs(external bool) string {
	if external {
		return ` target="_blank" rel="noopener noreferrer"`
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func publicHeaderClass(style string) string {
	switch style {
	case "accent-line":
		return "header-accent-line"
	case "accent-bg":
		return "header-accent-bg"
	default:
		return "header-neutral"
	}
}

var publicTpl = template.Must(template.New("page").Parse(`<!doctype html><html lang="en" data-theme="{{.DefaultTheme}}" data-ui-style="{{.PublicThemeStyle}}" data-header="{{.PublicHeaderStyle}}" style="--accent:{{.PublicPrimaryColor}};--accent-2:{{.PublicSecondaryColor}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}} · {{.SiteName}}</title>{{if .MetaDescription}}<meta name="description" content="{{.MetaDescription}}">{{end}}{{if .RSSURL}}<link rel="alternate" type="application/rss+xml" title="{{.SiteName}} RSS" href="{{.RSSURL}}">{{end}}{{if and .FaviconEnabled .FaviconURL}}<link rel="icon" href="{{.FaviconURL}}">{{end}}{{if .IconsEnabled}}<link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.2/css/all.min.css">{{end}}<style>
:root{--bg:#f4f7fb;--paper:#ffffff;--ink:#172033;--muted:#657085;--accent:#2563eb;--accent-2:#64748b;--line:#d8dee9;--soft:#edf2f7;--shadow:#0f172a14;--radius:18px;--radius-sm:12px;--radius-pill:999px}[data-theme=dark]{--bg:#0f172a;--paper:#172033;--ink:#e5edf8;--muted:#9fb0c7;--accent:#7ab7ff;--accent-2:#94a3b8;--line:#2c3b52;--soft:#111827;--shadow:#00000040}[data-ui-style=square]{--radius:4px;--radius-sm:3px;--radius-pill:3px}[data-ui-style=material]{--radius:8px;--radius-sm:4px;--radius-pill:4px;--shadow:#0f172a2b}*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at top left,color-mix(in srgb,var(--accent-2) 12%,var(--soft)),var(--bg) 36rem);color:var(--ink);font:16px/1.68 ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,sans-serif}a{color:var(--accent)}img{max-width:100%;height:auto;border-radius:var(--radius-sm)}pre{overflow:auto;background:#0b1220;color:#f5f7fb;padding:16px;border-radius:var(--radius-sm)}pre code{color:inherit;background:transparent;border:0;padding:0}code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}:not(pre)>code{color:var(--accent);background:color-mix(in srgb,var(--accent) 12%,var(--soft));border:1px solid color-mix(in srgb,var(--accent) 22%,var(--line));border-radius:5px;padding:2px 5px;font-size:.92em}.site{min-height:100vh;display:flex;flex-direction:column}.top{position:sticky;top:0;z-index:5;background:color-mix(in srgb,var(--paper) 88%,transparent);backdrop-filter:blur(16px);border-bottom:1px solid var(--line)}[data-header=accent-line] .top{border-bottom:3px solid var(--accent);box-shadow:0 10px 30px var(--shadow)}[data-header=accent-line] .nav a:hover{color:var(--accent);background:color-mix(in srgb,var(--accent) 11%,var(--soft))}[data-header=accent-bg] .top{background:linear-gradient(135deg,var(--accent),color-mix(in srgb,var(--accent) 68%,var(--accent-2)))!important;border-bottom:0;box-shadow:0 16px 42px var(--shadow)}[data-header=accent-bg] .top .brand,[data-header=accent-bg] .top .nav a,[data-header=accent-bg] .top .theme,[data-header=accent-bg] .top .menuBtn,[data-header=accent-bg] .top .drawerBtn,[data-header=accent-bg] .top .searchMenu summary{color:#fff}[data-header=accent-bg] .top .theme,[data-header=accent-bg] .top .menuBtn,[data-header=accent-bg] .top .drawerBtn,[data-header=accent-bg] .top .searchMenu summary{background:rgba(255,255,255,.14);border-color:rgba(255,255,255,.36)}[data-header=accent-bg] .top .nav a:hover{background:rgba(255,255,255,.16)}[data-header=accent-bg] .top .subnav{background:var(--paper);border-color:var(--line)}[data-header=accent-bg] .top .subnav a{color:var(--ink)}.bar{max-width:1060px;margin:0 auto;padding:14px 22px;display:flex;align-items:center;gap:18px}.brand{display:flex;align-items:center;gap:10px;margin-right:auto;color:var(--ink);font-weight:800;text-decoration:none;letter-spacing:-.02em}.brand img{max-width:180px;max-height:34px;width:auto;height:auto;object-fit:contain;border-radius:var(--radius-sm)}.nav{display:flex;align-items:center;gap:4px}.nav a{color:var(--ink);text-decoration:none;padding:8px 11px;border-radius:var(--radius-pill)}.nav a:hover{background:var(--soft)}.navParent{display:flex;align-items:center}.navGroup{position:relative}.navToggle{display:none;border:1px solid var(--line);background:var(--paper);color:var(--ink);border-radius:var(--radius-pill);width:34px;height:34px;place-items:center;cursor:pointer;font-weight:800;line-height:1}.subnav{display:none;position:absolute;right:0;top:100%;min-width:190px;background:var(--paper);border:1px solid var(--line);border-radius:var(--radius-sm);padding:8px;box-shadow:0 18px 50px var(--shadow)}.navGroup:hover>.subnav,.navGroup:focus-within>.subnav{display:flex;flex-direction:column}.subnav .subnav{position:static;box-shadow:none;border:0;border-left:1px solid var(--line);border-radius:0;margin-left:12px}.theme .dark{display:none}[data-theme=dark] .theme .light{display:none}[data-theme=dark] .theme .dark{display:inline}.theme,.menuBtn,.drawerBtn{border:1px solid var(--line);background:var(--paper);color:var(--ink);border-radius:var(--radius-pill);width:38px;height:38px;display:inline-grid;place-items:center;cursor:pointer;font-size:18px;line-height:1}.menuBtn{display:none}.drawerBtn{display:none}.searchMenu{position:relative}.searchMenu summary{list-style:none;cursor:pointer;border:1px solid var(--line);border-radius:var(--radius-pill);width:38px;height:38px;display:grid;place-items:center;font-size:18px}.searchMenu summary::-webkit-details-marker{display:none}.searchMenu form{position:absolute;right:0;top:calc(100% + 8px);display:flex;gap:6px;background:var(--paper);border:1px solid var(--line);border-radius:var(--radius-sm);padding:10px;box-shadow:0 18px 50px var(--shadow)}.searchMenu input,.searchPage input,.drawerSearch input{border:1px solid var(--line);border-radius:var(--radius-pill);background:var(--paper);color:var(--ink);padding:9px 12px}.searchMenu button,.searchPage button,.drawerSearch button{border:0;border-radius:var(--radius-pill);background:var(--accent);color:white;padding:9px 13px}.searchPage{display:flex;gap:8px;margin:18px 0}.searchPage input{flex:1}.muted{color:var(--muted)}.resultList{display:grid;gap:12px}.result{display:grid;gap:4px;text-decoration:none;color:var(--ink);border:1px solid var(--line);border-radius:var(--radius-sm);padding:16px;background:var(--soft)}.result span{font-size:12px;text-transform:uppercase;color:var(--accent);font-weight:800}.result small{color:var(--muted)}.drawerShade{position:fixed;inset:0;background:#02061766;z-index:9;opacity:0;pointer-events:none;transition:opacity .22s ease}.drawer{position:fixed;inset:0 auto 0 0;width:min(320px,86vw);background:var(--paper);border-right:1px solid var(--line);z-index:10;transform:translateX(-105%);transition:transform .26s cubic-bezier(.2,.8,.2,1);padding:18px;box-shadow:20px 0 60px var(--shadow);overflow:auto}.drawer.open{transform:translateX(0)}.drawerShade.open{opacity:1;pointer-events:auto}.drawerBrand{margin-bottom:18px}.drawerSearch{display:flex;gap:8px;margin-bottom:16px}.drawerSearch input{min-width:0;flex:1}.drawerNav{display:flex;flex-direction:column;gap:4px}.drawerNav a{color:var(--ink);text-decoration:none;padding:10px 12px;border-radius:var(--radius-sm)}.drawerNav a:hover{background:var(--soft)}.drawerNav .navParent{display:grid;grid-template-columns:minmax(0,1fr) 34px;gap:4px;align-items:center}.drawerNav .navToggle{display:grid}.drawerNav .subnav{position:static;display:none;background:transparent;box-shadow:none;border:0;border-left:1px solid var(--line);border-radius:0;margin-left:14px;padding:2px 0 2px 10px}.drawerNav .navGroup.open>.subnav{display:flex;flex-direction:column}.drawerNav .navGroup{display:flex;flex-direction:column}.wrap{width:min(920px,calc(100% - 32px));margin:42px auto;flex:1}.card{background:linear-gradient(180deg,var(--paper),color-mix(in srgb,var(--paper) 84%,var(--accent-2)));border:1px solid var(--line);border-radius:var(--radius);box-shadow:0 20px 60px var(--shadow);padding:clamp(24px,5vw,52px)}article h1:first-child{margin-top:0}h1,h2,h3{line-height:1.15;letter-spacing:-.03em}blockquote{border-left:4px solid var(--accent);margin-left:0;padding-left:16px;color:var(--muted)}.cms-icon{display:inline-grid;place-items:center;color:var(--accent);margin-inline:.08em}.cms-card{border:1px solid var(--line);border-radius:var(--radius);background:linear-gradient(180deg,var(--paper),color-mix(in srgb,var(--soft) 82%,var(--accent-2)));padding:20px;margin:22px 0;box-shadow:0 14px 34px var(--shadow)}.cms-card-head{display:flex;align-items:center;gap:12px;margin-bottom:8px}.cms-card-head .cms-icon{width:38px;height:38px;border-radius:var(--radius-sm);background:color-mix(in srgb,var(--accent) 13%,transparent);font-size:18px}.cms-card h3{margin:0}.cms-card-body>*:first-child{margin-top:0}.cms-card-body>*:last-child{margin-bottom:0}.mermaid{background:var(--soft);color:var(--ink);padding:16px;border-radius:var(--radius-sm);overflow:auto}.foot{border-top:1px solid var(--line);color:var(--muted);padding:28px 22px;text-align:center}.foot>*{max-width:920px;margin-left:auto;margin-right:auto}.foot p{margin:.25rem auto}@media(min-width:721px){.siteSide .bar{max-width:none}.siteSide .menuBtn,.siteSide .nav{display:none}.siteSide .drawerBtn{display:inline-block}}@media(max-width:720px){.bar{flex-wrap:wrap}.menuBtn{display:inline-block}.nav{display:none;width:100%;flex-direction:column;align-items:stretch;padding-top:10px}.nav.open{display:flex}.nav a{padding:11px 12px}.navParent{display:grid;grid-template-columns:minmax(0,1fr) 34px;gap:4px;align-items:center}.navToggle{display:grid}.navGroup{display:flex;flex-direction:column}.navGroup:hover>.subnav,.navGroup:focus-within>.subnav{display:none}.navGroup.open>.subnav{display:flex;flex-direction:column}.subnav{position:static;display:none;background:transparent;box-shadow:none;border:0;border-left:1px solid var(--line);border-radius:0;margin-left:14px;padding:2px 0 2px 10px}.wrap{margin:24px auto}}</style>{{.NavMenuStyle}}</head><body>{{if .SideNav}}<div class="drawerShade"></div><aside class="drawer"><a class="brand drawerBrand" href="/">{{if and .LogoEnabled .LogoURL}}<img src="{{.LogoURL}}" alt="">{{end}}<span>{{.SiteName}}</span></a>{{if .SearchEnabled}}<form class="drawerSearch" action="/search" method="get"><input name="q" placeholder="Search"><button type="submit">⌕</button></form>{{end}}<nav class="drawerNav">{{.MenuHTML}}</nav></aside>{{end}}<div class="site {{if .SideNav}}siteSide{{end}}"><header class="top"><div class="bar">{{if .SideNav}}<button class="drawerBtn" type="button" aria-label="Open side menu" aria-expanded="false">☰</button>{{end}}<a class="brand" href="/">{{if and .LogoEnabled .LogoURL}}<img src="{{.LogoURL}}" alt="">{{end}}<span>{{.SiteName}}</span></a>{{if and .MenuEnabled (not .SideNav)}}<button class="menuBtn" type="button" aria-label="Open menu" aria-expanded="false" aria-controls="nav">☰</button><nav class="nav" id="nav">{{.MenuHTML}}</nav>{{end}}{{if .SearchEnabled}}<details class="searchMenu"><summary aria-label="Search">⌕</summary><form action="/search" method="get"><input name="q" placeholder="Search"><button type="submit">Go</button></form></details>{{end}}{{if .ThemeToggleEnabled}}<button class="theme" type="button" aria-label="Toggle dark mode"><span class="themeIcon light">☾</span><span class="themeIcon dark">☀</span></button>{{end}}</div></header><main class="wrap"><section class="card"><article>{{.Body}}</article></section></main>{{if .FooterEnabled}}<footer class="foot">{{.Footer}}</footer>{{end}}</div>{{if .ThemeToggleEnabled}}<script>(function(){var root=document.documentElement;var saved=localStorage.getItem('uvoo-minicms-theme');if(saved){root.dataset.theme=saved}var theme=document.querySelector('.theme');if(theme){theme.onclick=function(){var next=root.dataset.theme==='dark'?'light':'dark';root.dataset.theme=next;localStorage.setItem('uvoo-minicms-theme',next)}};var btn=document.querySelector('.menuBtn');var nav=document.querySelector('#nav');if(btn&&nav){btn.onclick=function(){var open=nav.classList.toggle('open');btn.setAttribute('aria-expanded',open?'true':'false')}}var dbtn=document.querySelector('.drawerBtn');var drawer=document.querySelector('.drawer');var shade=document.querySelector('.drawerShade');function setDrawer(open){if(drawer&&shade){drawer.classList.toggle('open',open);shade.classList.toggle('open',open);if(dbtn){dbtn.setAttribute('aria-expanded',open?'true':'false')}}}if(dbtn){dbtn.onclick=function(){setDrawer(!drawer.classList.contains('open'))}}if(shade){shade.onclick=function(){setDrawer(false)}}})()</script>{{else}}<script>(function(){var btn=document.querySelector('.menuBtn');var nav=document.querySelector('#nav');if(btn&&nav){btn.onclick=function(){var open=nav.classList.toggle('open');btn.setAttribute('aria-expanded',open?'true':'false')}}var dbtn=document.querySelector('.drawerBtn');var drawer=document.querySelector('.drawer');var shade=document.querySelector('.drawerShade');function setDrawer(open){if(drawer&&shade){drawer.classList.toggle('open',open);shade.classList.toggle('open',open);if(dbtn){dbtn.setAttribute('aria-expanded',open?'true':'false')}}}if(dbtn){dbtn.onclick=function(){setDrawer(!drawer.classList.contains('open'))}}if(shade){shade.onclick=function(){setDrawer(false)}}})()</script>{{end}}{{if .HasMermaid}}<script type="module">import mermaid from 'https://cdn.jsdelivr.net/npm/mermaid@11.16.0/dist/mermaid.esm.min.mjs';document.querySelectorAll('pre code.language-mermaid').forEach(function(code){var div=document.createElement('div');div.className='mermaid';div.textContent=code.textContent;code.parentElement.replaceWith(div)});mermaid.initialize({startOnLoad:true,theme:document.documentElement.dataset.theme==='dark'?'dark':'default'});</script>{{end}}</body></html>`))
