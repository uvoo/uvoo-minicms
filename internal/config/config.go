package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr              string
	DataDir           string
	DBPath            string
	UploadDir         string
	WebRoot           string
	AdminUser         string
	AdminPass         string
	AdminRateLimit    int
	CSPMode           string
	HSTSEnabled       bool
	HSTSMaxAge        int
	SessionTTL        time.Duration
	AllowedCIDRs      []string
	DeniedCIDRs       []string
	TrustProxyHeaders bool
	MaxMindDBPath     string
	AllowedCountries  []string
	DeniedCountries   []string
	MaxUploadBytes    int64
	PublicSiteName    string
	TLSCertFile       string
	TLSKeyFile        string
	ReadOnly          bool
}

func Load() Config {
	data := env("CMS_DATA_DIR", "./data")
	cfg := Config{
		Addr:              env("CMS_ADDR", "127.0.0.1:8080"),
		DataDir:           data,
		DBPath:            env("CMS_DB", data+"/cms.db"),
		UploadDir:         env("CMS_UPLOAD_DIR", data+"/uploads"),
		WebRoot:           env("CMS_WEB_ROOT", "web/dist"),
		AdminUser:         env("CMS_ADMIN_USER", "admin"),
		AdminPass:         env("CMS_ADMIN_PASS", "change-me"),
		AdminRateLimit:    intEnv("CMS_ADMIN_RATE_LIMIT", 0),
		CSPMode:           env("CMS_CSP_MODE", "enforce"),
		HSTSEnabled:       boolEnv("CMS_HSTS_ENABLED", false),
		HSTSMaxAge:        intEnv("CMS_HSTS_MAX_AGE", 15552000),
		SessionTTL:        dur("CMS_SESSION_TTL", 12*time.Hour),
		AllowedCIDRs:      csv("CMS_ALLOW_CIDRS"),
		DeniedCIDRs:       csv("CMS_DENY_CIDRS"),
		TrustProxyHeaders: boolEnv("CMS_TRUST_PROXY_HEADERS", false),
		MaxMindDBPath:     os.Getenv("CMS_MAXMIND_DB"),
		AllowedCountries:  upperCSV("CMS_ALLOW_COUNTRIES"),
		DeniedCountries:   upperCSV("CMS_DENY_COUNTRIES"),
		MaxUploadBytes:    int64Env("CMS_MAX_UPLOAD_BYTES", 25<<20),
		PublicSiteName:    env("CMS_SITE_NAME", "Uvoo-MiniCMS"),
		TLSCertFile:       os.Getenv("CMS_TLS_CERT"),
		TLSKeyFile:        os.Getenv("CMS_TLS_KEY"),
		ReadOnly:          boolEnv("CMS_READ_ONLY", false),
	}
	allowCIDRs := strings.Join(cfg.AllowedCIDRs, ",")
	denyCIDRs := strings.Join(cfg.DeniedCIDRs, ",")
	allowCountries := strings.Join(cfg.AllowedCountries, ",")
	denyCountries := strings.Join(cfg.DeniedCountries, ",")
	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	flag.StringVar(&cfg.DBPath, "db", cfg.DBPath, "sqlite database path")
	flag.StringVar(&cfg.UploadDir, "uploads", cfg.UploadDir, "upload directory")
	flag.StringVar(&cfg.WebRoot, "web-root", cfg.WebRoot, "admin web asset directory")
	flag.StringVar(&cfg.AdminUser, "admin-user", cfg.AdminUser, "admin username")
	flag.StringVar(&cfg.AdminPass, "admin-pass", cfg.AdminPass, "admin password")
	flag.IntVar(&cfg.AdminRateLimit, "admin-rate-limit", cfg.AdminRateLimit, "admin/API requests per minute per client IP; 0 disables")
	flag.StringVar(&cfg.CSPMode, "csp-mode", cfg.CSPMode, "Content Security Policy mode: enforce, report-only, or off")
	flag.BoolVar(&cfg.HSTSEnabled, "hsts-enabled", cfg.HSTSEnabled, "emit Strict-Transport-Security on HTTPS requests")
	flag.IntVar(&cfg.HSTSMaxAge, "hsts-max-age", cfg.HSTSMaxAge, "Strict-Transport-Security max-age in seconds")
	flag.StringVar(&allowCIDRs, "allow-cidrs", allowCIDRs, "comma-separated IPv4/IPv6 CIDR allow list")
	flag.StringVar(&denyCIDRs, "deny-cidrs", denyCIDRs, "comma-separated IPv4/IPv6 CIDR deny list")
	flag.StringVar(&cfg.MaxMindDBPath, "maxmind-db", cfg.MaxMindDBPath, "MaxMind GeoIP2 country database path")
	flag.StringVar(&allowCountries, "allow-countries", allowCountries, "comma-separated ISO country allow list")
	flag.StringVar(&denyCountries, "deny-countries", denyCountries, "comma-separated ISO country deny list")
	flag.BoolVar(&cfg.TrustProxyHeaders, "trust-proxy-headers", cfg.TrustProxyHeaders, "trust reverse proxy client IP headers")
	flag.StringVar(&cfg.TLSCertFile, "tls-cert", cfg.TLSCertFile, "TLS certificate file")
	flag.StringVar(&cfg.TLSKeyFile, "tls-key", cfg.TLSKeyFile, "TLS private key file")
	flag.Parse()
	cfg.AllowedCIDRs = split(allowCIDRs, false)
	cfg.DeniedCIDRs = split(denyCIDRs, false)
	cfg.AllowedCountries = split(allowCountries, true)
	cfg.DeniedCountries = split(denyCountries, true)
	if cfg.AdminRateLimit < 0 {
		cfg.AdminRateLimit = 0
	}
	if cfg.HSTSMaxAge < 0 {
		cfg.HSTSMaxAge = 0
	}
	cfg.CSPMode = normalizeCSPMode(cfg.CSPMode)
	return cfg
}

func env(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func csv(k string) []string      { return split(os.Getenv(k), false) }
func upperCSV(k string) []string { return split(strings.ToUpper(os.Getenv(k)), true) }
func split(s string, upper bool) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			if upper {
				p = strings.ToUpper(p)
			}
			out = append(out, p)
		}
	}
	return out
}
func boolEnv(k string, d bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return d
	}
	return b
}
func int64Env(k string, d int64) int64 {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return d
	}
	return n
}
func intEnv(k string, d int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}
func normalizeCSPMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "off", "report-only":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "enforce"
	}
}
func dur(k string, d time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	x, err := time.ParseDuration(v)
	if err != nil {
		return d
	}
	return x
}
