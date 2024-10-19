package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
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
		applyIncomingProxyHeaders(req)

		var body []byte
		if needsInspect(req) {
			// Read body for inspection
			var err error

			body, err = io.ReadAll(req.Body)
			req.Body.Close()

			if err == nil {
				// Reassign body for proxy
				req.Body = io.NopCloser(bytes.NewReader(body))
			}
		}

		targetURL, err := url.Parse(cfg.Proxy.Url)
		if err != nil {
			panic(fmt.Errorf("invalid proxy URL: %v", err))
		}

		proxy := &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				rewriteOutgoingRequest(targetURL, r, req)
			},

			Transport: createTransport(targetURL, &cfg.Proxy),

			ModifyResponse: func(resp *http.Response) error {
				setProductInfo(&resp.Header)

				// Request does not need to inspect
				// Or there's something wrong with request body, passthrough.
				if body == nil {
					return nil
				}

				return inspectRequest(resp, body, &cfg.Limit)
			},
		}

		proxy.ServeHTTP(rw, req)
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
		ReadBufferSize:        cfg.ReadBufferSize,
		WriteBufferSize:       cfg.WriteBufferSize,
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

func rewriteOutgoingRequest(targetURL *url.URL, pr *httputil.ProxyRequest, req *http.Request) {
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

// applyIncomingProxyHeaders is a middleware that updates the request context
// with the original client IP and protocol based on X-Forwarded headers.
func applyIncomingProxyHeaders(r *http.Request) {
	// Extract the original client IP from X-Forwarded-For or X-Real-IP
	clientIP := getIncomingClientRealIP(r)

	// Extract the original protocol from X-Forwarded-Proto
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		r.URL.Scheme = forwardedProto
	}

	// Update r.RemoteAddr with the client IP
	if clientIP != "" {
		r.RemoteAddr = clientIP
	}
}

// getIncomingClientRealIP extracts the client IP address from X-Forwarded-For or X-Real-IP headers.
func getIncomingClientRealIP(r *http.Request) string {
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
	if header.Get("Server") == "" {
		header.Set("Server", SERVER_SIGNATURE)
	} else {
		header.Set("X-Powered-By", SERVER_SIGNATURE)
	}
}
