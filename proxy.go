package main

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// NewProxyHandler builds the http.Handler that reverse-proxies everything
// to the configured upstream, injecting a fixed Basic auth header on every
// request. /healthz is handled locally and never touches the upstream.
func NewProxyHandler(cfg Config, logger *slog.Logger) http.Handler {
	target := &url.URL{Scheme: cfg.UpstreamScheme, Host: cfg.UpstreamHost}
	authHeader := cfg.BasicAuthHeader() // static for the process lifetime

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			director(req, target, authHeader)
		},
		Transport: buildTransport(cfg),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Error("upstream proxy error", "path", r.URL.Path, "err", err)
			w.WriteHeader(http.StatusBadGateway)
		},
		// Flush immediately rather than buffering, so chunked/streaming
		// SOAP responses and attachments aren't held in memory.
		FlushInterval: -1,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.Handle("/", withLogging(logger, rp))
	return mux
}

// director mutates req in place per the httputil.ReverseProxy.Director
// contract. It is kept as a small, pure-ish function so it can be unit
// tested without spinning up a server. It only ever touches scheme, host,
// and a handful of headers — method, path, query, body, and all other
// headers pass through untouched.
func director(req *http.Request, target *url.URL, authHeader string) {
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.Host = target.Host // upstream Host header + TLS SNI

	// Always discard whatever auth the client sent and replace it with the
	// configured credentials — this is the whole point of the proxy.
	req.Header.Del("Authorization")
	req.Header.Set("Authorization", authHeader)

	// Don't leak anything about the local proxy upstream.
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Forwarded-Host")
	req.Header.Del("X-Forwarded-Proto")
}

func buildTransport(cfg Config) http.RoundTripper {
	return &http.Transport{
		Proxy:                 nil, // never honor an ambient outbound proxy for the upstream leg
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify},
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second, // EWS can be slow on large mailbox operations
		ExpectContinueTimeout: 1 * time.Second,
		// No request/response size limits: EWS attachments and large SOAP
		// payloads are expected, and ReverseProxy streams bodies via
		// io.Copy rather than buffering them, so this carries no
		// unbounded-memory risk.
	}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}
