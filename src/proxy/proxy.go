package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/iedon/activitypub-fw/config"
)

const (
	SERVER_NAME      = "ActivityPub-FW"
	SERVER_VERSION   = "0.1"
	SERVER_SIGNATURE = SERVER_NAME + "/" + SERVER_VERSION
)

func ProxyHandler(cfg *config.Config) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		cfg.RLock()

		if isAllowedProxyServer(req, cfg) {
			applyInboundProxyHeaders(req)
		}

		if needsInspect(req, cfg) {
			// Read body for inspection
			var err error

			body, err := io.ReadAll(req.Body)
			req.Body.Close()

			if err == nil {
				// Reassign body for re-read by passthrough / not filtered requests
				req.Body = io.NopCloser(bytes.NewReader(body))
			}

			if reason := inspectRequest(req, body, cfg); reason != "" {
				filterRequest(rw, req, reason)
				return
			}
		}

		// Passthrough the request to upstream
		passthrough(rw, req, cfg)

		cfg.RUnlock()
	}
}

// Passthrough the request to upstream
func passthrough(rw http.ResponseWriter, req *http.Request, cfg *config.Config) {
	targetURL, err := url.Parse(cfg.Config.Proxy.Url)
	if err != nil {
		cfg.RUnlock()
		sendResponse(rw, http.StatusInternalServerError, "server error: invalid upstream url specified")
		return
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			rewriteOutboundRequest(targetURL, r, req)
		},

		Transport: createTransport(targetURL, cfg),

		ModifyResponse: func(resp *http.Response) error {
			setProductInfo(&resp.Header)
			return nil
		},
	}

	proxy.ServeHTTP(rw, req)
}

// createTransport configures the HTTP transport with custom dialing logic
func createTransport(targetURL *url.URL, cfg *config.Config) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   time.Duration(cfg.Config.Proxy.Timeout) * time.Second,   // Dial timeout
		KeepAlive: time.Duration(cfg.Config.Proxy.KeepAlive) * time.Second, // Keep-alive period for TCP connections
	}

	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var protocol, path string

			switch strings.ToLower(cfg.Config.Server.Protocol) {
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
			case "unix":
				protocol = "unix"
				path = cfg.Config.Proxy.UnixPath
			default:
				return nil, fmt.Errorf("unsupported protocol: %s", cfg.Config.Proxy.Protocol)
			}

			return dialer.DialContext(ctx, protocol, path)
		},
		MaxIdleConns:          cfg.Config.Proxy.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.Config.Proxy.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.Config.Proxy.MaxConnsPerHost,
		IdleConnTimeout:       time.Duration(cfg.Config.Proxy.IdleConnTimeout) * time.Second,
		TLSHandshakeTimeout:   time.Duration(cfg.Config.Proxy.TLSHandshakeTimeout) * time.Second,
		ExpectContinueTimeout: time.Duration(cfg.Config.Proxy.ExpectContinueTimeout) * time.Second,
		ForceAttemptHTTP2:     cfg.Config.Proxy.ForceAttemptHTTP2,
		ReadBufferSize:        cfg.Config.Proxy.ReadBufferSize,
		WriteBufferSize:       cfg.Config.Proxy.WriteBufferSize,
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
func setForwardHeader(headers *http.Header, key string, value string) {
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

func rewriteOutboundRequest(targetURL *url.URL, pr *httputil.ProxyRequest, req *http.Request) {
	pr.SetURL(targetURL)
	pr.Out.Host = req.Host

	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		setForwardHeader(&pr.Out.Header, "X-Forwarded-For", clientIP)
	} else if req.RemoteAddr != "" {
		setForwardHeader(&pr.Out.Header, "X-Forwarded-For", req.RemoteAddr)
	}

	setForwardHeader(&pr.Out.Header, "X-Forwarded-Host", req.Host)

	if scheme := req.URL.Scheme; scheme != "" {
		setForwardHeader(&pr.Out.Header, "X-Forwarded-Proto", scheme)
	} else {
		if req.TLS == nil {
			setForwardHeader(&pr.Out.Header, "X-Forwarded-Proto", "http")
		} else {
			setForwardHeader(&pr.Out.Header, "X-Forwarded-Proto", "https")
		}
	}
}

func isAllowedProxyServer(r *http.Request, cfg *config.Config) bool {
	if strings.ToLower(cfg.Config.Server.Protocol) == "unix" {
		return true
	}

	clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}

	ip, err := netip.ParseAddr(clientIP)
	if err != nil {
		return false
	}

	for _, netStr := range cfg.Config.Server.InboundProxyNetworks {
		if prefix, err := netip.ParsePrefix(netStr); err == nil {
			if prefix.Contains(ip) {
				return true
			}
		} else if addr, err := netip.ParseAddr(netStr); err == nil {
			if addr == ip {
				return true
			}
		}
	}

	return false
}

// applyInboundProxyHeaders is a middleware that updates the request context
// with the original client IP and protocol based on X-Forwarded headers.
func applyInboundProxyHeaders(r *http.Request) {
	// Extract the original client IP from X-Forwarded-For or X-Real-IP
	clientIP := getInboundClientRealIP(r)

	// Extract the original protocol from X-Forwarded-Proto
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		r.URL.Scheme = forwardedProto
	}

	// Update r.RemoteAddr with the client IP
	if clientIP != "" {
		r.RemoteAddr = clientIP
	}
}

// getInboundClientRealIP extracts the client IP address from X-Forwarded-For or X-Real-IP headers.
func getInboundClientRealIP(r *http.Request) string {
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

func setProductInfo(header *http.Header) {
	header.Set("Server", SERVER_SIGNATURE)
	header.Set("X-Powered-By", SERVER_SIGNATURE)
}

func sendResponse(rw http.ResponseWriter, statusCode int, message string) {
	header := rw.Header()

	setProductInfo(&header)
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", "no-cache, no-store")
	header.Set("Pragma", "no-cache")

	rw.WriteHeader(statusCode)
	rw.Write([]byte(fmt.Sprintf("{\"code\": %d,\"message\":\"%s\"}", statusCode, message)))
}

// Note: Returning a 400 in ActivityPub may result in repeated retries from the remote
// or, in the worst case, delivery suspension. Therefore, return a 202 for 'inbox'.
func filterRequest(rw http.ResponseWriter, req *http.Request, message string) {
	// Set response status
	var status int
	if strings.Contains(req.RequestURI, "inbox") {
		status = http.StatusAccepted
	} else {
		status = http.StatusBadRequest
	}

	sendResponse(rw, status, message)
}
