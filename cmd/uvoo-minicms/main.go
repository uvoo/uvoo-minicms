package main

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"uvoo-minicms/cmsv1connect"
	"uvoo-minicms/internal/acl"
	"uvoo-minicms/internal/auth"
	"uvoo-minicms/internal/config"
	"uvoo-minicms/internal/db"
	"uvoo-minicms/internal/geo"
	"uvoo-minicms/internal/httpreq"
	"uvoo-minicms/internal/service"
	"uvoo-minicms/internal/web"
)

func main() {
	cfg := config.Load()
	must(validateRuntimeSecurity(cfg))
	must(os.MkdirAll(cfg.UploadDir, 0750))
	var store *db.Store
	var err error
	if cfg.ReadOnly {
		store, err = db.OpenReadOnly(cfg.DBPath)
	} else {
		store, err = db.Open(cfg.DBPath)
	}
	must(err)
	ipf, err := auth.NewIPFilter(cfg.AllowedCIDRs, cfg.DeniedCIDRs, cfg.TrustProxyHeaders)
	must(err)
	geof, err := geo.New(cfg.MaxMindDBPath, cfg.AllowedCountries, cfg.DeniedCountries, cfg.TrustProxyHeaders)
	must(err)
	defer geof.Close()

	svc := &service.Service{Store: store, UploadDir: cfg.UploadDir, MaxUploadBytes: cfg.MaxUploadBytes, SiteName: cfg.PublicSiteName, ReadOnly: cfg.ReadOnly}
	_, api := cmsv1connect.NewCMSServiceHandler(svc)
	admin := http.FileServer(http.Dir(cfg.WebRoot))
	uploads := http.StripPrefix("/uploads/", http.FileServer(http.Dir(cfg.UploadDir)))
	pub := web.NewPublic(store, cfg.PublicSiteName)
	pub.TrustProxy = cfg.TrustProxyHeaders
	adminACL := acl.Filter{Store: store, Scope: "admin", TrustProxy: cfg.TrustProxyHeaders}
	publicACL := acl.Filter{Store: store, Scope: "public", TrustProxy: cfg.TrustProxyHeaders}
	adminGeo := geof.WithStore(store, "admin")
	publicGeo := geof.WithStore(store, "public")
	adminRateLimit := auth.NewRateLimiter(cfg.AdminRateLimit, time.Minute, cfg.TrustProxyHeaders)

	mux := http.NewServeMux()
	mux.Handle("/cms.v1.CMSService/", chain(api, noStore, ipf.Middleware, adminACL.Middleware, adminGeo.Middleware, sameOrigin(cfg.TrustProxyHeaders), adminRateLimit.Middleware, auth.Basic{User: cfg.AdminUser, Pass: cfg.AdminPass}.Middleware))
	mux.Handle("/uploads/", chain(uploads, ipf.Middleware, publicACL.Middleware, publicGeo.Middleware, cacheUploads))
	mux.Handle("/admin/", chain(http.StripPrefix("/admin/", admin), noStore, ipf.Middleware, adminACL.Middleware, adminGeo.Middleware, adminRateLimit.Middleware, auth.Basic{User: cfg.AdminUser, Pass: cfg.AdminPass}.Middleware))
	mux.Handle("/", chain(pub, ipf.Middleware, publicACL.Middleware, publicGeo.Middleware))

	tlsEnabled := cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		log.Fatal("both TLS cert and key must be provided")
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: secureHeaders(mux, cfg.CSPMode, cfg.HSTSEnabled, cfg.HSTSMaxAge, cfg.TrustProxyHeaders), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 4 * time.Minute, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Printf("uvoo-minicms listening on %s db=%s uploads=%s web-root=%s tls=%t read-only=%t", cfg.Addr, cfg.DBPath, filepath.Clean(cfg.UploadDir), filepath.Clean(cfg.WebRoot), tlsEnabled, cfg.ReadOnly)
	if tlsEnabled {
		log.Fatal(srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile))
	}
	log.Fatal(srv.ListenAndServe())
}

type mw func(http.Handler) http.Handler

func chain(h http.Handler, mws ...mw) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
func secureHeaders(next http.Handler, cspMode string, hstsEnabled bool, hstsMaxAge int, trustProxy bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		setCSPHeader(w, cspMode)
		setHSTSHeader(w, r, hstsEnabled, hstsMaxAge, trustProxy)
		next.ServeHTTP(w, r)
	})
}

func setCSPHeader(w http.ResponseWriter, mode string) {
	switch mode {
	case "off":
		return
	case "report-only":
		w.Header().Set("Content-Security-Policy-Report-Only", contentSecurityPolicy())
	default:
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy())
	}
}

func contentSecurityPolicy() string {
	return strings.Join([]string{
		"default-src 'self'",
		"base-uri 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"form-action 'self'",
		"connect-src 'self'",
		"img-src 'self' data: blob: http: https:",
		"media-src 'self' data: blob: http: https:",
		"font-src 'self' data: https://cdnjs.cloudflare.com",
		"style-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com",
		"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net",
		"frame-src https://www.youtube-nocookie.com https://player.vimeo.com",
	}, "; ")
}

func setHSTSHeader(w http.ResponseWriter, r *http.Request, enabled bool, maxAge int, trustProxy bool) {
	if !enabled || maxAge <= 0 || !httpreq.IsHTTPS(r, trustProxy) {
		return
	}
	w.Header().Set("Strict-Transport-Security", "max-age="+strconv.Itoa(maxAge))
}

func cacheUploads(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(trustProxy bool) mw {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			if !sameOriginRequest(r, trustProxy) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func sameOriginRequest(r *http.Request, trustProxy bool) bool {
	expected := httpreq.BaseURL(r, trustProxy)
	for _, raw := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return false
		}
		host := httpreq.CleanHost(u.Host)
		if host == "" {
			return false
		}
		if strings.EqualFold(u.Scheme+"://"+host, expected) {
			return true
		}
		return false
	}
	return true
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func validateRuntimeSecurity(cfg config.Config) error {
	if insecureAdminPass(cfg.AdminPass) && exposedBind(cfg.Addr) {
		return errors.New("refusing to start with default or empty CMS_ADMIN_PASS on a non-loopback bind address")
	}
	return nil
}

func insecureAdminPass(pass string) bool {
	switch strings.TrimSpace(pass) {
	case "", "change-me", "change-me-now":
		return true
	default:
		return false
	}
}

func exposedBind(addr string) bool {
	host := strings.TrimSpace(addr)
	if host == "" {
		return true
	}
	if strings.HasPrefix(host, ":") {
		return true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch strings.ToLower(host) {
	case "", "*", "0.0.0.0", "::":
		return true
	case "localhost":
		return false
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}
