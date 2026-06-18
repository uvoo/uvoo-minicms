package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"
	"uvoo-minicms/internal/auth"
	"uvoo-minicms/internal/db"
	"uvoo-minicms/internal/importer"
	"uvoo-minicms/internal/netguard"
)

type Service struct {
	Store          *db.Store
	UploadDir      string
	MaxUploadBytes int64
	SiteName       string
	ReadOnly       bool
}

func ok(v map[string]any) (*connect.Response[structpb.Struct], error) {
	s, err := structpb.NewStruct(v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(s), nil
}
func fields(req *connect.Request[structpb.Struct]) map[string]any { return req.Msg.AsMap() }
func str(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
func boolean(m map[string]any, k string) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return false
}
func number(m map[string]any, k string) int {
	switch v := m[k].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err == nil {
			return n
		}
		return 0
	default:
		return 0
	}
}

func (s *Service) Health(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	return ok(map[string]any{"ok": true, "time": time.Now().UTC().Format(time.RFC3339)})
}

func (s *Service) requireWritable() error {
	if s.ReadOnly {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("this Uvoo-MiniCMS replica is read-only; send admin changes to the writer replica"))
	}
	return nil
}

func (s *Service) Session(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	username := auth.UserFromContext(ctx)
	if username == "" {
		username, _, _ = basicAuth(req)
	}
	info := map[string]any{
		"username":    username,
		"auth_scheme": authScheme(req),
	}
	if token, source := bearerToken(req); token != "" {
		if claims, err := jwtClaims(token); err == nil {
			info["jwt_source"] = source
			info["jwt_claims"] = claims
		}
	}
	proxy := proxyIdentity(req)
	if len(proxy) > 0 {
		info["proxy_identity"] = proxy
	}
	return ok(map[string]any{"session": info})
}
func (s *Service) ListPages(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	pages, err := s.Store.ListPages(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	items := make([]any, 0, len(pages))
	for _, p := range pages {
		items = append(items, pageMap(p, false))
	}
	return ok(map[string]any{"pages": items})
}

func authScheme(req *connect.Request[structpb.Struct]) string {
	authHeader := strings.TrimSpace(req.Header().Get("Authorization"))
	if authHeader == "" {
		return "basic"
	}
	scheme, _, ok := strings.Cut(authHeader, " ")
	if !ok {
		return strings.ToLower(authHeader)
	}
	return strings.ToLower(scheme)
}

func basicAuth(req *connect.Request[structpb.Struct]) (string, string, bool) {
	authHeader := strings.TrimSpace(req.Header().Get("Authorization"))
	scheme, encoded, ok := strings.Cut(authHeader, " ")
	if !ok || !strings.EqualFold(scheme, "Basic") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", "", false
	}
	user, pass, ok := strings.Cut(string(decoded), ":")
	return user, pass, ok
}

func bearerToken(req *connect.Request[structpb.Struct]) (string, string) {
	authHeader := strings.TrimSpace(req.Header().Get("Authorization"))
	if scheme, token, ok := strings.Cut(authHeader, " "); ok && strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(token), "Authorization"
	}
	for _, header := range []string{"X-Auth-Request-Access-Token", "X-Forwarded-Access-Token", "X-Auth-Request-Id-Token", "X-Forwarded-Id-Token"} {
		if token := strings.TrimSpace(req.Header().Get(header)); token != "" {
			return token, header
		}
	}
	return "", ""
}

func jwtClaims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, errors.New("invalid jwt")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, key := range []string{"iss", "sub", "aud", "exp", "iat", "nbf", "email", "email_verified", "name", "preferred_username", "groups", "roles", "scope"} {
		if value, ok := claims[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func proxyIdentity(req *connect.Request[structpb.Struct]) map[string]any {
	headers := map[string]string{
		"user":               "X-Forwarded-User",
		"email":              "X-Forwarded-Email",
		"preferred_username": "X-Forwarded-Preferred-Username",
		"auth_request_user":  "X-Auth-Request-User",
		"auth_request_email": "X-Auth-Request-Email",
	}
	out := map[string]any{}
	for key, header := range headers {
		if value := strings.TrimSpace(req.Header().Get(header)); value != "" {
			out[key] = value
		}
	}
	return out
}
func (s *Service) GetPage(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	p, err := s.Store.GetPage(ctx, str(fields(req), "slug"))
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return ok(map[string]any{"page": pageMap(p, true)})
}
func (s *Service) SavePage(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	m := fields(req)
	slug := cleanSlug(str(m, "slug"))
	if slug == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid slug"))
	}
	routePath := cleanPath(str(m, "path"))
	if routePath == "" {
		routePath = "/" + slug
	}
	if slug == "home" {
		routePath = "/"
	} else if routePath == "/" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("only the home page can use /"))
	}
	settings, err := s.Store.GetSettings(ctx, s.SiteName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	p, err := s.Store.SavePageWithRevisions(ctx, db.Page{
		Slug:            slug,
		Path:            routePath,
		Title:           str(m, "title"),
		MetaDescription: str(m, "meta_description"),
		ContentType:     cleanContentType(str(m, "content_type")),
		Tags:            cleanTags(str(m, "tags")),
		Markdown:        str(m, "markdown"),
		Published:       boolean(m, "published"),
		PublishedAt:     cleanPublishedAt(str(m, "published_at")),
	}, settings.RevisionHistoryLimit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ok(map[string]any{"page": pageMap(p, true)})
}
func (s *Service) ListPageRevisions(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	revisions, err := s.Store.ListPageRevisions(ctx, cleanSlug(str(fields(req), "slug")))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	items := make([]any, 0, len(revisions))
	for _, revision := range revisions {
		items = append(items, pageRevisionMap(revision))
	}
	return ok(map[string]any{"revisions": items})
}
func (s *Service) DeletePage(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	if err := s.Store.DeletePage(ctx, str(fields(req), "slug")); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"ok": true})
}
func (s *Service) GetSettings(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	settings, err := s.Store.GetSettings(ctx, s.SiteName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"settings": settingsMap(settings)})
}
func (s *Service) SaveSettings(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	settings, err := settingsFromMap(fields(req), s.SiteName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	settings, err = s.Store.SaveSettings(ctx, settings)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ok(map[string]any{"settings": settingsMap(settings)})
}
func (s *Service) ListThemeHistory(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	items, err := s.Store.ListThemeHistory(ctx, 20)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	history := make([]any, 0, len(items))
	for _, item := range items {
		history = append(history, themeHistoryMap(item))
	}
	return ok(map[string]any{"themes": history})
}
func (s *Service) ListAssets(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	assets, err := s.Store.ListAssets(ctx, 120)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	items := make([]any, 0, len(assets))
	for _, asset := range assets {
		items = append(items, assetMap(asset))
	}
	return ok(map[string]any{"assets": items})
}
func (s *Service) DeleteAsset(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	id := int64(number(fields(req), "id"))
	if id <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("asset id is required"))
	}
	asset, err := s.Store.GetAsset(ctx, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	settings, err := s.Store.GetSettings(ctx, s.SiteName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if settings.LogoURL == asset.URL {
		settings.LogoURL = ""
		settings.LogoEnabled = false
	}
	if settings.FaviconURL == asset.URL {
		settings.FaviconURL = ""
		settings.FaviconEnabled = false
	}
	settings, err = s.Store.SaveSettings(ctx, settings)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.removeUploadFile(asset.Path); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.Store.DeleteAsset(ctx, id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"ok": true, "settings": settingsMap(settings)})
}
func (s *Service) GetACL(ctx context.Context, _ *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	settings, rules, err := s.Store.GetACL(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"acl": aclMap(settings, rules)})
}
func (s *Service) SaveACL(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	settings, rules, err := aclFromMap(fields(req))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	settings, rules, err = s.Store.SaveACL(ctx, settings, rules)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ok(map[string]any{"acl": aclMap(settings, rules)})
}
func (s *Service) ImportPreview(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	opts := importOptions(fields(req))
	opts.PreviewOnly = true
	result, err := importer.Importer{Client: netguard.NewHTTPClient(4 * time.Second)}.Preview(ctx, opts)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("import preview timed out after 15 seconds; the source site did not respond quickly enough"))
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ok(map[string]any{"import": importResultMap(result)})
}
func (s *Service) ImportSite(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	opts := importOptions(fields(req))
	if opts.DownloadImages {
		if err := ensureWritableDir(s.UploadDir); err != nil {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("upload directory %q is not writable: %w; run with -uploads set to a writable directory or fix ownership/permissions", s.UploadDir, err))
		}
	}
	result, err := importer.Importer{Client: netguard.NewHTTPClient(10 * time.Second)}.Import(ctx, s.Store, s.UploadDir, s.SiteName, opts)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("import timed out after 3 minutes; try a smaller max page count or check the source site"))
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := s.markImportExisting(ctx, &result); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"import": importResultMap(result)})
}
func (s *Service) UploadFile(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	m := fields(req)
	name := safeName(str(m, "name"))
	dataURL := str(m, "data")
	if name == "" || !strings.Contains(dataURL, ",") {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name and data URL required"))
	}
	if !allowedUpload(name) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("file type is not allowed"))
	}
	b64 := strings.SplitN(dataURL, ",", 2)[1]
	if int64(len(b64)) > s.MaxUploadBytes*2 {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("upload too large"))
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if int64(len(data)) > s.MaxUploadBytes {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("upload too large"))
	}
	if err := validateUploadContent(name, data); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	name = time.Now().UTC().Format("150405.000-") + name
	a, err := s.writeUploadAsset(ctx, name, data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"asset": assetMap(a)})
}
func (s *Service) SetSiteImage(ctx context.Context, req *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
	if err := s.requireWritable(); err != nil {
		return nil, err
	}
	m := fields(req)
	kind := strings.ToLower(str(m, "kind"))
	if kind != "logo" && kind != "favicon" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("kind must be logo or favicon"))
	}
	data, err := siteImageBytes(ctx, str(m, "data"), str(m, "url"), s.MaxUploadBytes)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image must be a PNG or JPEG"))
	}
	if format != "png" && format != "jpeg" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("image must be a PNG or JPEG"))
	}
	encoded, ext, err := optimizeSiteImage(kind, img, format)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if int64(len(encoded)) > s.MaxUploadBytes {
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("optimized image is too large"))
	}
	name := identityImageName(kind, firstNonEmpty(str(m, "name"), pathBaseFromURL(str(m, "url"))), ext)
	asset, err := s.writeUploadAsset(ctx, name, encoded)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	settings, err := s.Store.GetSettings(ctx, s.SiteName)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if kind == "logo" {
		settings.LogoURL = asset.URL
		settings.LogoEnabled = true
	} else {
		settings.FaviconURL = asset.URL
		settings.FaviconEnabled = true
	}
	settings, err = s.Store.SaveSettings(ctx, settings)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return ok(map[string]any{"asset": assetMap(asset), "settings": settingsMap(settings)})
}

func (s *Service) writeUploadAsset(ctx context.Context, name string, data []byte) (db.Asset, error) {
	day := time.Now().UTC().Format("2006/01/02")
	dir := filepath.Join(s.UploadDir, day)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return db.Asset{}, err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0640); err != nil {
		return db.Asset{}, err
	}
	url := "/uploads/" + day + "/" + name
	return s.Store.InsertAsset(ctx, name, path, url, int64(len(data)))
}

func (s *Service) removeUploadFile(assetPath string) error {
	if strings.TrimSpace(assetPath) == "" {
		return nil
	}
	root, err := filepath.Abs(s.UploadDir)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(assetPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("asset path %q is outside upload directory", assetPath)
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func importOptions(m map[string]any) importer.Options {
	maxPages := number(m, "max_pages")
	if maxPages <= 0 {
		maxPages = 50
	}
	return importer.Options{
		URL:              str(m, "url"),
		MaxPages:         maxPages,
		IncludePosts:     boolean(m, "include_posts"),
		ImportMenu:       boolean(m, "import_menu"),
		Publish:          boolean(m, "publish"),
		UpdateExisting:   boolean(m, "update_existing"),
		DownloadImages:   boolean(m, "download_images"),
		AdvancedScraping: boolean(m, "advanced_scraping"),
	}
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func (s *Service) markImportExisting(ctx context.Context, result *importer.Result) error {
	pages, err := s.Store.ListPages(ctx)
	if err != nil {
		return err
	}
	bySlug := map[string]bool{}
	byPath := map[string]bool{}
	for _, page := range pages {
		bySlug[page.Slug] = true
		byPath[page.Path] = true
	}
	result.Existing = 0
	for i := range result.Pages {
		exists := bySlug[result.Pages[i].Slug] || byPath[result.Pages[i].Path]
		result.Pages[i].Exists = exists
		if exists {
			result.Existing++
		}
	}
	return nil
}

func importResultMap(result importer.Result) map[string]any {
	pages := make([]any, 0, len(result.Pages))
	for _, page := range result.Pages {
		pages = append(pages, map[string]any{
			"slug":             page.Slug,
			"path":             page.Path,
			"title":            page.Title,
			"meta_description": page.MetaDescription,
			"content_type":     page.ContentType,
			"tags":             page.Tags,
			"source_url":       page.SourceURL,
			"published":        page.Published,
			"exists":           page.Exists,
		})
	}
	menu := make([]any, 0, len(result.Menu))
	for _, item := range result.Menu {
		menu = append(menu, map[string]any{
			"id":        item.ID,
			"type":      item.Type,
			"parent_id": item.ParentID,
			"label":     item.Label,
			"url":       item.URL,
			"external":  item.External,
			"enabled":   item.Enabled,
		})
	}
	errors := make([]any, 0, len(result.Errors))
	for _, err := range result.Errors {
		errors = append(errors, err)
	}
	return map[string]any{
		"source":        result.Source,
		"base_url":      result.BaseURL,
		"pages":         pages,
		"menu":          menu,
		"imported":      result.Imported,
		"skipped":       result.Skipped,
		"errors":        errors,
		"existing":      result.Existing,
		"wordpress":     result.WordPress,
		"sitemap_url":   result.SitemapURL,
		"preview_limit": result.PreviewLimit,
	}
}

func pageMap(p db.Page, body bool) map[string]any {
	m := map[string]any{
		"id":               p.ID,
		"slug":             p.Slug,
		"path":             p.Path,
		"title":            p.Title,
		"meta_description": p.MetaDescription,
		"content_type":     p.ContentType,
		"tags":             p.Tags,
		"published":        p.Published,
		"published_at":     p.PublishedAt,
		"created_at":       p.CreatedAt,
		"updated_at":       p.UpdatedAt,
	}
	if body {
		m["markdown"] = p.Markdown
	}
	return m
}

func pageRevisionMap(r db.PageRevision) map[string]any {
	return map[string]any{
		"id":               r.ID,
		"page_id":          r.PageID,
		"slug":             r.Slug,
		"path":             r.Path,
		"title":            r.Title,
		"meta_description": r.MetaDescription,
		"content_type":     r.ContentType,
		"tags":             r.Tags,
		"markdown":         r.Markdown,
		"published":        r.Published,
		"published_at":     r.PublishedAt,
		"created_at":       r.CreatedAt,
	}
}

func assetMap(a db.Asset) map[string]any {
	return map[string]any{
		"id":         a.ID,
		"name":       a.Name,
		"url":        a.URL,
		"size":       a.Size,
		"created_at": a.CreatedAt,
	}
}

func aclMap(settings db.SecuritySettings, rules []db.ACLRule) map[string]any {
	items := make([]any, 0, len(rules))
	for _, rule := range rules {
		items = append(items, map[string]any{
			"id":         rule.ID,
			"scope":      rule.Scope,
			"action":     rule.Action,
			"cidr":       rule.CIDR,
			"note":       rule.Note,
			"enabled":    rule.Enabled,
			"created_at": rule.CreatedAt,
		})
	}
	return map[string]any{
		"admin_default":          settings.AdminDefault,
		"public_default":         settings.PublicDefault,
		"admin_allow_countries":  settings.AdminAllowCountries,
		"admin_deny_countries":   settings.AdminDenyCountries,
		"public_allow_countries": settings.PublicAllowCountries,
		"public_deny_countries":  settings.PublicDenyCountries,
		"rules":                  items,
	}
}

func aclFromMap(m map[string]any) (db.SecuritySettings, []db.ACLRule, error) {
	settings := db.SecuritySettings{
		AdminDefault:         cleanDefault(str(m, "admin_default")),
		PublicDefault:        cleanDefault(str(m, "public_default")),
		AdminAllowCountries:  str(m, "admin_allow_countries"),
		AdminDenyCountries:   str(m, "admin_deny_countries"),
		PublicAllowCountries: str(m, "public_allow_countries"),
		PublicDenyCountries:  str(m, "public_deny_countries"),
	}
	rawRules, _ := m["rules"].([]any)
	rules := make([]db.ACLRule, 0, len(rawRules))
	for _, raw := range rawRules {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		cidr := strings.TrimSpace(str(item, "cidr"))
		if cidr == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return db.SecuritySettings{}, nil, err
		}
		enabled := true
		if v, ok := item["enabled"].(bool); ok {
			enabled = v
		}
		rules = append(rules, db.ACLRule{
			Scope:   cleanACLScope(str(item, "scope")),
			Action:  cleanACLAction(str(item, "action")),
			CIDR:    cidr,
			Note:    str(item, "note"),
			Enabled: enabled,
		})
	}
	return settings, rules, nil
}

func cleanDefault(s string) string {
	if strings.EqualFold(s, "deny") {
		return "deny"
	}
	return "allow"
}
func cleanACLScope(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "admin", "public":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "all"
	}
}
func cleanACLAction(s string) string {
	if strings.EqualFold(s, "allow") {
		return "allow"
	}
	return "deny"
}

func settingsMap(settings db.Settings) map[string]any {
	menu := make([]any, 0, len(settings.Menu))
	for _, item := range settings.Menu {
		menu = append(menu, map[string]any{
			"id":        item.ID,
			"type":      item.Type,
			"parent_id": item.ParentID,
			"label":     item.Label,
			"url":       item.URL,
			"external":  item.External,
			"enabled":   item.Enabled,
		})
	}
	return map[string]any{
		"site_name":              settings.SiteName,
		"logo_url":               settings.LogoURL,
		"favicon_url":            settings.FaviconURL,
		"default_theme":          settings.DefaultTheme,
		"public_theme_style":     settings.PublicThemeStyle,
		"public_primary_color":   settings.PublicPrimaryColor,
		"public_secondary_color": settings.PublicSecondaryColor,
		"public_header_style":    settings.PublicHeaderStyle,
		"admin_theme":            settings.AdminTheme,
		"theme_style":            settings.ThemeStyle,
		"admin_primary_color":    settings.AdminPrimaryColor,
		"admin_secondary_color":  settings.AdminSecondaryColor,
		"admin_palette":          settings.AdminPalette,
		"footer_markdown":        settings.FooterMarkdown,
		"menu":                   menu,
		"logo_enabled":           settings.LogoEnabled,
		"favicon_enabled":        settings.FaviconEnabled,
		"menu_enabled":           settings.MenuEnabled,
		"footer_enabled":         settings.FooterEnabled,
		"theme_toggle_enabled":   settings.ThemeToggleEnabled,
		"icons_enabled":          settings.IconsEnabled,
		"search_enabled":         settings.SearchEnabled,
		"nav_layout":             settings.NavLayout,
		"blog_enabled":           settings.BlogEnabled,
		"blog_path":              settings.BlogPath,
		"blog_title":             settings.BlogTitle,
		"blog_menu_enabled":      settings.BlogMenuEnabled,
		"blog_posts_per_page":    settings.BlogPostsPerPage,
		"revision_history_limit": settings.RevisionHistoryLimit,
	}
}

func themeHistoryMap(item db.ThemeHistory) map[string]any {
	return map[string]any{
		"id":                     item.ID,
		"admin_theme":            item.AdminTheme,
		"theme_style":            item.ThemeStyle,
		"admin_primary_color":    item.AdminPrimaryColor,
		"admin_secondary_color":  item.AdminSecondaryColor,
		"admin_palette":          item.AdminPalette,
		"public_theme":           item.PublicTheme,
		"public_theme_style":     item.PublicThemeStyle,
		"public_primary_color":   item.PublicPrimaryColor,
		"public_secondary_color": item.PublicSecondaryColor,
		"public_header_style":    item.PublicHeaderStyle,
		"updated_at":             item.UpdatedAt,
	}
}

func settingsFromMap(m map[string]any, fallbackSiteName string) (db.Settings, error) {
	settings := db.Settings{
		SiteName:             firstNonEmpty(str(m, "site_name"), fallbackSiteName),
		LogoURL:              cleanAssetURL(str(m, "logo_url")),
		FaviconURL:           cleanAssetURL(str(m, "favicon_url")),
		DefaultTheme:         cleanTheme(str(m, "default_theme")),
		PublicThemeStyle:     cleanThemeStyle(str(m, "public_theme_style")),
		PublicPrimaryColor:   cleanHexColor(str(m, "public_primary_color")),
		PublicSecondaryColor: cleanHexColorWithDefault(str(m, "public_secondary_color"), "#64748b"),
		PublicHeaderStyle:    cleanHeaderStyle(str(m, "public_header_style")),
		AdminTheme:           cleanTheme(str(m, "admin_theme")),
		ThemeStyle:           cleanThemeStyle(str(m, "theme_style")),
		AdminPrimaryColor:    cleanHexColor(str(m, "admin_primary_color")),
		AdminSecondaryColor:  cleanHexColorWithDefault(str(m, "admin_secondary_color"), "#64748b"),
		AdminPalette:         cleanPalette(str(m, "admin_palette")),
		FooterMarkdown:       str(m, "footer_markdown"),
		Menu:                 navItems(m["menu"]),
		LogoEnabled:          boolean(m, "logo_enabled"),
		FaviconEnabled:       boolean(m, "favicon_enabled"),
		MenuEnabled:          boolean(m, "menu_enabled"),
		FooterEnabled:        boolean(m, "footer_enabled"),
		ThemeToggleEnabled:   boolean(m, "theme_toggle_enabled"),
		IconsEnabled:         boolean(m, "icons_enabled"),
		SearchEnabled:        boolean(m, "search_enabled"),
		NavLayout:            cleanNavLayout(str(m, "nav_layout")),
		BlogEnabled:          boolean(m, "blog_enabled"),
		BlogPath:             cleanBlogPath(str(m, "blog_path")),
		BlogTitle:            firstNonEmpty(str(m, "blog_title"), "Blog"),
		BlogMenuEnabled:      boolean(m, "blog_menu_enabled"),
		BlogPostsPerPage:     cleanBoundedInt(number(m, "blog_posts_per_page"), 20, 1, 100),
		RevisionHistoryLimit: cleanBoundedInt(number(m, "revision_history_limit"), 0, 0, 100),
	}
	if settings.SiteName == "" {
		return db.Settings{}, errors.New("site name required")
	}
	if err := validateNavItems(settings.Menu); err != nil {
		return db.Settings{}, err
	}
	return settings, nil
}

func navItems(v any) []db.NavItem {
	items, _ := v.([]any)
	out := make([]db.NavItem, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		label := str(m, "label")
		itemType := cleanNavItemType(str(m, "type"))
		url := cleanURL(str(m, "url"))
		if itemType == "section" {
			url = ""
		}
		if label == "" || (itemType != "section" && url == "") {
			continue
		}
		out = append(out, db.NavItem{
			ID:       cleanID(str(m, "id")),
			Type:     itemType,
			ParentID: cleanID(str(m, "parent_id")),
			Label:    label,
			URL:      url,
			External: itemType != "section" && (boolean(m, "external") || strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")),
			Enabled:  boolean(m, "enabled"),
		})
	}
	return out
}

func cleanNavItemType(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "section") {
		return "section"
	}
	return "link"
}

func validateNavItems(items []db.NavItem) error {
	ids := map[string]db.NavItem{}
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		if _, exists := ids[item.ID]; exists {
			return fmt.Errorf("menu item %q uses a duplicate id", item.Label)
		}
		ids[item.ID] = item
	}
	for _, item := range items {
		if item.ParentID == "" {
			continue
		}
		if item.ID != "" && item.ParentID == item.ID {
			return fmt.Errorf("menu item %q cannot be its own parent", item.Label)
		}
		if _, ok := ids[item.ParentID]; !ok {
			return fmt.Errorf("menu item %q references a missing parent", item.Label)
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(db.NavItem) error
	visit = func(item db.NavItem) error {
		if item.ID == "" || item.ParentID == "" {
			return nil
		}
		if visited[item.ID] {
			return nil
		}
		if visiting[item.ID] {
			return fmt.Errorf("menu item %q is part of a parent cycle", item.Label)
		}
		visiting[item.ID] = true
		if parent, ok := ids[item.ParentID]; ok {
			if err := visit(parent); err != nil {
				return err
			}
		}
		visiting[item.ID] = false
		visited[item.ID] = true
		return nil
	}
	for _, item := range items {
		if err := visit(item); err != nil {
			return err
		}
	}
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func cleanSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
func cleanPath(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if s == "/" {
		return "/"
	}
	parts := strings.Split(strings.Trim(s, "/"), "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = cleanSlug(part); part != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) == 0 {
		return "/"
	}
	return "/" + strings.Join(clean, "/")
}
func cleanContentType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "post":
		return "post"
	default:
		return "page"
	}
}
func cleanPublishedAt(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	return s
}
func cleanTheme(s string) string {
	if strings.EqualFold(s, "dark") {
		return "dark"
	}
	return "light"
}
func cleanThemeStyle(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "square", "material":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "soft"
	}
}
func cleanPalette(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "forest", "ember", "mono", "custom":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "slate"
	}
}
func cleanHeaderStyle(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "accent-line", "accent-bg":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "neutral"
	}
}
func cleanHexColor(s string) string {
	return cleanHexColorWithDefault(s, "#386bc0")
}
func cleanHexColorWithDefault(s, fallback string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	if regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(s) {
		return strings.ToLower(s)
	}
	return fallback
}
func cleanNavLayout(s string) string {
	if strings.EqualFold(s, "side") {
		return "side"
	}
	return "top"
}
func cleanBlogPath(s string) string {
	path := cleanPath(s)
	if path == "" || path == "/" {
		return "/blog"
	}
	return path
}
func cleanBoundedInt(value, fallback, min, max int) int {
	if value < min || value > max {
		return fallback
	}
	return value
}
func cleanTags(s string) string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if tag := cleanSlug(part); tag != "" {
			out = append(out, tag)
		}
	}
	return strings.Join(out, ", ")
}
func cleanID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return slugRe.ReplaceAllString(strings.ToLower(s), "-")
}
func cleanAssetURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "/uploads/") {
		return s
	}
	u, err := url.Parse(s)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return u.String()
	}
	return ""
}
func cleanURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "mailto:") || strings.HasPrefix(s, "tel:") {
		return s
	}
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return cleanPath(s)
}
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
func safeName(s string) string {
	s = filepath.Base(s)
	e := ext(s)
	name := strings.TrimSuffix(s, filepath.Ext(s))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if name == "" {
		name = "upload"
	}
	return name + e
}
func ext(s string) string {
	e := strings.ToLower(filepath.Ext(s))
	if len(e) > 12 {
		return ""
	}
	return e
}

func allowedUpload(name string) bool {
	switch ext(name) {
	case ".avif", ".gif", ".ico", ".jpeg", ".jpg", ".png", ".webp", ".pdf", ".txt", ".md", ".csv":
		return true
	default:
		return false
	}
}

func validateUploadContent(name string, data []byte) error {
	if len(data) == 0 {
		return errors.New("file is empty")
	}
	switch ext(name) {
	case ".png":
		if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
			return nil
		}
	case ".jpg", ".jpeg":
		if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
			return nil
		}
	case ".gif":
		if len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))) {
			return nil
		}
	case ".webp":
		if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
			return nil
		}
	case ".avif":
		if len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")) {
			brand := string(data[8:12])
			if brand == "avif" || brand == "avis" || brand == "mif1" {
				return nil
			}
		}
	case ".ico":
		if len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 1 && data[3] == 0 {
			return nil
		}
	case ".pdf":
		if bytes.HasPrefix(data, []byte("%PDF-")) {
			return nil
		}
	case ".txt", ".md", ".csv":
		if bytes.IndexByte(data, 0) >= 0 {
			return errors.New("text uploads must not contain NUL bytes")
		}
		if utf8.Valid(data) {
			return nil
		}
		return errors.New("text uploads must be valid UTF-8")
	}
	return errors.New("file content does not match the allowed extension")
}

func decodeDataURL(dataURL string, maxBytes int64) ([]byte, error) {
	if !strings.Contains(dataURL, ",") {
		return nil, errors.New("data URL required")
	}
	parts := strings.SplitN(dataURL, ",", 2)
	if !strings.Contains(strings.ToLower(parts[0]), ";base64") {
		return nil, errors.New("base64 data URL required")
	}
	if maxBytes > 0 && int64(len(parts[1])) > maxBytes*2 {
		return nil, errors.New("upload too large")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, errors.New("upload too large")
	}
	return data, nil
}

func siteImageBytes(ctx context.Context, dataURL, rawURL string, maxBytes int64) ([]byte, error) {
	if strings.TrimSpace(dataURL) != "" {
		return decodeDataURL(dataURL, maxBytes)
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("image data or URL required")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("valid http or https image URL required")
	}
	if err := netguard.ValidateURL(u); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Uvoo-MiniCMS Admin/1.0")
	req.Header.Set("Accept", "image/png,image/jpeg;q=0.9,*/*;q=0.1")
	client := netguard.NewHTTPClient(12 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %s", u.String(), resp.Status)
	}
	limit := maxBytes
	if limit <= 0 {
		limit = 8 << 20
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("downloaded image is too large")
	}
	return body, nil
}

func pathBaseFromURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return path.Base(u.Path)
}

func identityImageName(kind, original, outExt string) string {
	base := cleanSlug(strings.TrimSuffix(filepath.Base(original), filepath.Ext(original)))
	if base == "" {
		base = kind
	}
	return fmt.Sprintf("%s-%s-%s%s", kind, time.Now().UTC().Format("150405.000"), base, outExt)
}

func optimizeSiteImage(kind string, src image.Image, format string) ([]byte, string, error) {
	bounds := src.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil, "", errors.New("image has invalid dimensions")
	}
	var out image.Image
	ext := ".png"
	if kind == "favicon" {
		out = resizeCover(src, 64, 64)
	} else {
		out = resizeContain(src, 512, 160, false)
		if format == "jpeg" {
			ext = ".jpg"
		}
	}
	var b bytes.Buffer
	if ext == ".jpg" {
		if err := jpeg.Encode(&b, out, &jpeg.Options{Quality: 86}); err != nil {
			return nil, "", err
		}
	} else if err := png.Encode(&b, out); err != nil {
		return nil, "", err
	}
	return b.Bytes(), ext, nil
}

func resizeContain(src image.Image, maxW, maxH int, allowUpscale bool) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := math.Min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	if !allowUpscale && scale > 1 {
		scale = 1
	}
	dstW := maxInt(1, int(math.Round(float64(w)*scale)))
	dstH := maxInt(1, int(math.Round(float64(h)*scale)))
	return resizeRect(src, b, dstW, dstH)
}

func resizeCover(src image.Image, dstW, dstH int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	srcRatio := float64(sw) / float64(sh)
	dstRatio := float64(dstW) / float64(dstH)
	crop := b
	if srcRatio > dstRatio {
		cropW := maxInt(1, int(math.Round(float64(sh)*dstRatio)))
		x0 := b.Min.X + (sw-cropW)/2
		crop = image.Rect(x0, b.Min.Y, x0+cropW, b.Max.Y)
	} else if srcRatio < dstRatio {
		cropH := maxInt(1, int(math.Round(float64(sw)/dstRatio)))
		y0 := b.Min.Y + (sh-cropH)/2
		crop = image.Rect(b.Min.X, y0, b.Max.X, y0+cropH)
	}
	return resizeRect(src, crop, dstW, dstH)
}

func resizeRect(src image.Image, rect image.Rectangle, dstW, dstH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		sy := float64(rect.Min.Y) + (float64(y)+0.5)*float64(rect.Dy())/float64(dstH) - 0.5
		for x := 0; x < dstW; x++ {
			sx := float64(rect.Min.X) + (float64(x)+0.5)*float64(rect.Dx())/float64(dstW) - 0.5
			dst.Set(x, y, bilinearAt(src, rect, sx, sy))
		}
	}
	return dst
}

func bilinearAt(src image.Image, rect image.Rectangle, sx, sy float64) color.NRGBA {
	x0 := clampInt(int(math.Floor(sx)), rect.Min.X, rect.Max.X-1)
	y0 := clampInt(int(math.Floor(sy)), rect.Min.Y, rect.Max.Y-1)
	x1 := clampInt(x0+1, rect.Min.X, rect.Max.X-1)
	y1 := clampInt(y0+1, rect.Min.Y, rect.Max.Y-1)
	tx := clampFloat(sx-float64(x0), 0, 1)
	ty := clampFloat(sy-float64(y0), 0, 1)
	c00 := color.NRGBAModel.Convert(src.At(x0, y0)).(color.NRGBA)
	c10 := color.NRGBAModel.Convert(src.At(x1, y0)).(color.NRGBA)
	c01 := color.NRGBAModel.Convert(src.At(x0, y1)).(color.NRGBA)
	c11 := color.NRGBAModel.Convert(src.At(x1, y1)).(color.NRGBA)
	return color.NRGBA{
		R: blendByte(c00.R, c10.R, c01.R, c11.R, tx, ty),
		G: blendByte(c00.G, c10.G, c01.G, c11.G, tx, ty),
		B: blendByte(c00.B, c10.B, c01.B, c11.B, tx, ty),
		A: blendByte(c00.A, c10.A, c01.A, c11.A, tx, ty),
	}
}

func blendByte(c00, c10, c01, c11 uint8, tx, ty float64) uint8 {
	top := float64(c00)*(1-tx) + float64(c10)*tx
	bottom := float64(c01)*(1-tx) + float64(c11)*tx
	return uint8(math.Round(top*(1-ty) + bottom*ty))
}

func clampInt(v, minValue, maxValue int) int {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}

func clampFloat(v, minValue, maxValue float64) float64 {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func JSONError(w http.ResponseWriter, code int, msg string) { http.Error(w, msg, code) }
