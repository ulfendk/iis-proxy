package main

import (
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestDirector_RewritesUpstreamAndAuth(t *testing.T) {
	target := &url.URL{Scheme: "https", Host: "exchange.example.com"}
	req := httptest.NewRequest("POST", "http://localhost:8080/EWS/Exchange.asmx?foo=bar", nil)
	req.Header.Set("Authorization", "Basic bogus-client-value")

	director(req, target, "Basic real-server-value")

	if req.URL.Scheme != "https" || req.URL.Host != "exchange.example.com" {
		t.Fatalf("unexpected upstream URL: %s", req.URL)
	}
	if req.Host != "exchange.example.com" {
		t.Fatalf("Host header not rewritten: %s", req.Host)
	}
	if got := req.Header.Get("Authorization"); got != "Basic real-server-value" {
		t.Fatalf("Authorization not overridden, got %q", got)
	}
	if req.URL.Path != "/EWS/Exchange.asmx" || req.URL.RawQuery != "foo=bar" {
		t.Fatalf("path/query not preserved: %s", req.URL)
	}
	if req.Method != "POST" {
		t.Fatalf("method not preserved: %s", req.Method)
	}
}

func TestDirector_OverridesAuthEvenWhenClientSendsNone(t *testing.T) {
	target := &url.URL{Scheme: "https", Host: "exchange.example.com"}
	req := httptest.NewRequest("GET", "http://localhost:8080/autodiscover/autodiscover.xml", nil)

	director(req, target, "Basic real-server-value")

	if got := req.Header.Get("Authorization"); got != "Basic real-server-value" {
		t.Fatalf("Authorization not set, got %q", got)
	}
}

func TestDirector_StripsForwardedHeaders(t *testing.T) {
	target := &url.URL{Scheme: "https", Host: "exchange.example.com"}
	req := httptest.NewRequest("GET", "http://localhost:8080/owa/", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.50")
	req.Header.Set("X-Forwarded-Host", "localhost:8080")
	req.Header.Set("X-Forwarded-Proto", "http")

	director(req, target, "Basic real-server-value")

	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"} {
		if req.Header.Get(h) != "" {
			t.Fatalf("expected %s to be stripped, got %q", h, req.Header.Get(h))
		}
	}
}

func TestLoadConfig_RequiresCredentials(t *testing.T) {
	getenv := func(string) string { return "" }
	_, err := LoadConfig(getenv)
	if err == nil {
		t.Fatal("expected error for missing credentials, got nil")
	}
}

func TestLoadConfig_AppliesDefaults(t *testing.T) {
	env := map[string]string{
		"UPSTREAM_HOST":     "exchange.example.com",
		"EXCHANGE_USERNAME": "jdoe",
		"EXCHANGE_PASSWORD": "s3cret",
	}
	getenv := func(k string) string { return env[k] }

	cfg, err := LoadConfig(getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != "0.0.0.0:8080" {
		t.Errorf("ListenAddr default = %q", cfg.ListenAddr)
	}
	if cfg.UpstreamScheme != "https" {
		t.Errorf("UpstreamScheme default = %q", cfg.UpstreamScheme)
	}
	if cfg.UpstreamHost != "exchange.example.com" {
		t.Errorf("UpstreamHost = %q, want value from env", cfg.UpstreamHost)
	}
}

func TestLoadConfig_RequiresUpstreamHost(t *testing.T) {
	env := map[string]string{
		"EXCHANGE_USERNAME": "jdoe",
		"EXCHANGE_PASSWORD": "s3cret",
	}
	getenv := func(k string) string { return env[k] }

	_, err := LoadConfig(getenv)
	if err == nil {
		t.Fatal("expected error for missing UPSTREAM_HOST, got nil")
	}
}

func TestLoadConfig_ReadsFileVariant(t *testing.T) {
	dir := t.TempDir()
	pwFile := dir + "/password.txt"
	if err := os.WriteFile(pwFile, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	env := map[string]string{
		"UPSTREAM_HOST":          "exchange.example.com",
		"EXCHANGE_USERNAME":      "jdoe",
		"EXCHANGE_PASSWORD_FILE": pwFile,
	}
	getenv := func(k string) string { return env[k] }

	cfg, err := LoadConfig(getenv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Password != "s3cret" {
		t.Fatalf("Password = %q, want trimmed file contents", cfg.Password)
	}
}

func TestLoadConfig_UnreadableFileIsLoudError(t *testing.T) {
	env := map[string]string{
		"EXCHANGE_USERNAME":      "jdoe",
		"EXCHANGE_PASSWORD_FILE": "/nonexistent/path/does/not/exist",
	}
	getenv := func(k string) string { return env[k] }

	_, err := LoadConfig(getenv)
	if err == nil {
		t.Fatal("expected error for unreadable _FILE path, got nil")
	}
}

func TestBasicAuthHeader_EncodesDomainUserPass(t *testing.T) {
	cfg := Config{Domain: "CORP", Username: "jdoe", Password: "s3cret"}
	want := "Basic Q09SUFxqZG9lOnMzY3JldA=="
	if got := cfg.BasicAuthHeader(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBasicAuthHeader_OmitsDomainPrefixWhenEmpty(t *testing.T) {
	cfg := Config{Username: "jdoe", Password: "s3cret"}
	want := "Basic amRvZTpzM2NyZXQ="
	if got := cfg.BasicAuthHeader(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
