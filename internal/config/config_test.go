package config

import (
	"flag"
	"os"
	"testing"
)

func TestLoadReadsAdminRateLimitFromEnv(t *testing.T) {
	t.Setenv("CMS_ADMIN_RATE_LIMIT", "120")
	cfg := loadForTest(t)
	if cfg.AdminRateLimit != 120 {
		t.Fatalf("expected admin rate limit 120, got %d", cfg.AdminRateLimit)
	}
}

func TestLoadTreatsInvalidAdminRateLimitAsDisabled(t *testing.T) {
	t.Setenv("CMS_ADMIN_RATE_LIMIT", "-25")
	cfg := loadForTest(t)
	if cfg.AdminRateLimit != 0 {
		t.Fatalf("expected invalid negative rate limit to disable, got %d", cfg.AdminRateLimit)
	}
}

func TestLoadReadsAdminRateLimitFlag(t *testing.T) {
	cfg := loadForTest(t, "-admin-rate-limit", "45")
	if cfg.AdminRateLimit != 45 {
		t.Fatalf("expected admin rate limit 45, got %d", cfg.AdminRateLimit)
	}
}

func TestLoadNormalizesCSPMode(t *testing.T) {
	tests := map[string]string{
		"":            "enforce",
		"enforce":     "enforce",
		"report-only": "report-only",
		"off":         "off",
		"invalid":     "enforce",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			if raw != "" {
				t.Setenv("CMS_CSP_MODE", raw)
			}
			cfg := loadForTest(t)
			if cfg.CSPMode != want {
				t.Fatalf("expected CSP mode %q, got %q", want, cfg.CSPMode)
			}
		})
	}
}

func TestLoadReadsCSPModeFlag(t *testing.T) {
	cfg := loadForTest(t, "-csp-mode", "report-only")
	if cfg.CSPMode != "report-only" {
		t.Fatalf("expected report-only CSP mode, got %q", cfg.CSPMode)
	}
}

func TestLoadReadsHSTSConfigFromEnv(t *testing.T) {
	t.Setenv("CMS_HSTS_ENABLED", "true")
	t.Setenv("CMS_HSTS_MAX_AGE", "31536000")
	cfg := loadForTest(t)
	if !cfg.HSTSEnabled {
		t.Fatal("expected HSTS to be enabled")
	}
	if cfg.HSTSMaxAge != 31536000 {
		t.Fatalf("expected HSTS max age 31536000, got %d", cfg.HSTSMaxAge)
	}
}

func TestLoadNormalizesNegativeHSTSMaxAge(t *testing.T) {
	t.Setenv("CMS_HSTS_MAX_AGE", "-1")
	cfg := loadForTest(t)
	if cfg.HSTSMaxAge != 0 {
		t.Fatalf("expected negative HSTS max age to become 0, got %d", cfg.HSTSMaxAge)
	}
}

func TestLoadReadsHSTSFlags(t *testing.T) {
	cfg := loadForTest(t, "-hsts-enabled", "-hsts-max-age", "86400")
	if !cfg.HSTSEnabled {
		t.Fatal("expected HSTS flag to enable HSTS")
	}
	if cfg.HSTSMaxAge != 86400 {
		t.Fatalf("expected HSTS max age 86400, got %d", cfg.HSTSMaxAge)
	}
}

func TestLoadReadsReadOnlyFromEnv(t *testing.T) {
	t.Setenv("CMS_READ_ONLY", "true")
	cfg := loadForTest(t)
	if !cfg.ReadOnly {
		t.Fatal("expected CMS_READ_ONLY=true to enable read-only mode")
	}
}

func loadForTest(t *testing.T, args ...string) Config {
	t.Helper()
	oldCommandLine := flag.CommandLine
	oldArgs := os.Args
	t.Cleanup(func() {
		flag.CommandLine = oldCommandLine
		os.Args = oldArgs
	})
	flag.CommandLine = flag.NewFlagSet("config-test", flag.ContinueOnError)
	os.Args = append([]string{"uvoo-minicms"}, args...)
	return Load()
}
