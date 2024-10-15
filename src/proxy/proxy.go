package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/iedon/activitypub-fw/config"
)

func ProxyHandler(targetURL *url.URL, cfg *config.ProxyConfig) http.HandlerFunc {
	return func(respW http.ResponseWriter, req *http.Request) {
		ApplyIncomingProxyHeaders(req)
		proxy := &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				rewriteHandler(targetURL, r, req)
			},
			Transport:      createTransport(targetURL, cfg),
			ModifyResponse: modifyResponse,
		}
		proxy.ServeHTTP(respW, req)
	}
}

// createTransport configures the HTTP transport with custom dialing logic
func createTransport(targetURL *url.URL, cfg *config.ProxyConfig) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   time.Duration(cfg.Timeout) * time.Second,   // Dial timeout
		KeepAlive: time.Duration(cfg.KeepAlive) * time.Second, // Keep-alive period for TCP connections
	}

	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var protocol, path string

			switch cfg.Protocol {
			case "tcp":
				protocol = "tcp"
				port := targetURL.Port()
				if port == "" {
					var err error
					port, err = defaultPort(targetURL.Scheme)
					if err != nil {
						return nil, err
					}
				}
				path = net.JoinHostPort(targetURL.Hostname(), port)
			case "unix", "unixgram", "unixpacket":
				protocol = "unix"
				path = cfg.UnixPath
			default:
				return nil, fmt.Errorf("unsupported protocol: %s", cfg.Protocol)
			}

			return dialer.DialContext(ctx, protocol, path)
		},
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       time.Duration(cfg.IdleConnTimeout) * time.Second,
		TLSHandshakeTimeout:   time.Duration(cfg.TLSHandshakeTimeout) * time.Second,
		ExpectContinueTimeout: time.Duration(cfg.ExpectContinueTimeout) * time.Second,
		ForceAttemptHTTP2:     cfg.ForceAttemptHTTP2,
	}
}

// defaultPort returns the default port for a given scheme
func defaultPort(scheme string) (string, error) {
	switch scheme {
	case "http":
		return "80", nil
	case "https":
		return "443", nil
	default:
		return "", fmt.Errorf("unsupported scheme: %s", scheme)
	}
}

// Sets the X-Forward header in the HTTP request
func SetForwardHeader(headers *http.Header, key string, value string) {
	// If we aren't the first proxy retain prior
	// header information as a comma+space
	// separated list and fold multiple headers into one.
	prior, ok := (*headers)[key]
	omit := ok && prior == nil // Go net/http/httputil issue 38079: nil now means don't populate the header

	if len(prior) > 0 {
		value = strings.Join(prior, ", ") + ", " + value
	}

	if !omit {
		headers.Set(key, value)
	}
}

func rewriteHandler(targetURL *url.URL, pr *httputil.ProxyRequest, req *http.Request) {
	pr.SetURL(targetURL)
	pr.Out.Host = req.Host

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		SetForwardHeader(&pr.Out.Header, "X-Forwarded-For", clientIP)
	}

	SetForwardHeader(&pr.Out.Header, "X-Forwarded-Host", req.Host)

	var scheme string
	if req.TLS != nil {
		scheme = "https"
	} else {
		scheme = "http"
	}

	SetForwardHeader(&pr.Out.Header, "X-Forwarded-Proto", scheme)
}

// ApplyIncomingProxyHeaders is a middleware that updates the request context
// with the original client IP and protocol based on X-Forwarded headers.
func ApplyIncomingProxyHeaders(r *http.Request) {
	// Extract the original client IP from X-Forwarded-For or X-Real-IP
	clientIP := GetIncomingClientRealIP(r)

	// Extract the original protocol from X-Forwarded-Proto
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		r.URL.Scheme = forwardedProto
	}

	// Update r.RemoteAddr with the client IP
	if clientIP != "" {
		r.RemoteAddr = clientIP
	}
}

// GetIncomingClientRealIP extracts the client IP address from X-Forwarded-For or X-Real-IP headers.
func GetIncomingClientRealIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, we take the first one
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	return ""
}
