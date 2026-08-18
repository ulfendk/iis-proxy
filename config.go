package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// Config holds everything the proxy needs at runtime. It is loaded once at
// startup from environment variables (see LoadConfig) and never mutated.
type Config struct {
	ListenAddr     string // e.g. "0.0.0.0:8080"
	UpstreamScheme string // "https"
	UpstreamHost   string // "mail.example.com" (host[:port], no scheme)

	Domain   string
	Username string
	Password string // resolved plaintext credential; never logged

	InsecureSkipVerify bool
}

// LoadConfig reads configuration from environment variables via getenv
// (normally os.Getenv, injected here so it can be tested without touching
// the real environment). Any KEY_FILE variant is resolved first (the
// Docker/Podman secrets convention), falling back to KEY. Required fields
// missing after resolution produce a single descriptive error so the
// caller can fail fast and loud.
func LoadConfig(getenv func(string) string) (Config, error) {
	listenAddr, err := resolveEnv(getenv, "LISTEN_ADDR")
	if err != nil {
		return Config{}, err
	}
	upstreamScheme, err := resolveEnv(getenv, "UPSTREAM_SCHEME")
	if err != nil {
		return Config{}, err
	}
	upstreamHost, err := resolveEnv(getenv, "UPSTREAM_HOST")
	if err != nil {
		return Config{}, err
	}
	domain, err := resolveEnv(getenv, "EXCHANGE_DOMAIN")
	if err != nil {
		return Config{}, err
	}
	username, err := resolveEnv(getenv, "EXCHANGE_USERNAME")
	if err != nil {
		return Config{}, err
	}
	password, err := resolveEnv(getenv, "EXCHANGE_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	insecureSkipVerify, err := resolveEnv(getenv, "UPSTREAM_INSECURE_SKIP_VERIFY")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ListenAddr:         valueOrDefault(listenAddr, "0.0.0.0:8080"),
		UpstreamScheme:     valueOrDefault(upstreamScheme, "https"),
		UpstreamHost:       valueOrDefault(upstreamHost, "mail.example.com"),
		Domain:             domain,
		Username:           username,
		Password:           password,
		InsecureSkipVerify: insecureSkipVerify == "true",
	}

	var missing []string
	if cfg.Username == "" {
		missing = append(missing, "EXCHANGE_USERNAME")
	}
	if cfg.Password == "" {
		missing = append(missing, "EXCHANGE_PASSWORD")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// resolveEnv checks KEY_FILE first (Docker/Podman secrets convention),
// falling back to KEY. If KEY_FILE is set but unreadable, that is a fatal
// config error, not a silent fallback to "unset" — a misconfigured secrets
// mount should fail loudly at startup.
func resolveEnv(getenv func(string) string, key string) (string, error) {
	if path := getenv(key + "_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading %s_FILE: %w", key, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return getenv(key), nil
}

func valueOrDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// BasicAuthHeader builds the value for the Authorization header sent to the
// upstream: "Basic base64(DOMAIN\username:password)". The DOMAIN\ prefix is
// omitted if Domain is empty.
func (c Config) BasicAuthHeader() string {
	user := c.Username
	if c.Domain != "" {
		user = c.Domain + `\` + c.Username
	}
	raw := user + ":" + c.Password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}
