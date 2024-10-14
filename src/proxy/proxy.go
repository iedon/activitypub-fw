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
)

func ProxyHandler(protocol string, targetURL *url.URL, unixPath string) http.HandlerFunc {
	return func(respW http.ResponseWriter, req *http.Request) {
		proxy := &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				rewriteHandler(targetURL, r, req)
			},
			Transport: createTransport(protocol, targetURL, unixPath),
		}
		proxy.ServeHTTP(respW, req)
	}
}

// createTransport configures the HTTP transport with custom dialing logic
func createTransport(protocol string, targetURL *url.URL, unixPath string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var _protocol, _path string

			switch protocol {
			case "tcp":
				_protocol = "tcp"
				port := targetURL.Port()
				if port == "" {
					port = defaultPort(targetURL.Scheme)
				}
				_path = net.JoinHostPort(targetURL.Hostname(), port)
			case "unix", "unixgram", "unixpacket":
				_protocol = "unix"
				_path = unixPath
			default:
				return nil, fmt.Errorf("unsupported protocol: %s", protocol)
			}

			return net.Dial(_protocol, _path)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// defaultPort returns the default port for a given scheme
func defaultPort(scheme string) string {
	switch scheme {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		panic(fmt.Sprintf("Unsupported scheme: %s", scheme))
	}
}

// Updates the X-Forwarded-* header in the HTTP request
func UpdateHeader(headers *http.Header, key string, value string) {
	// If we aren't the first proxy retain prior
	// X-Forwarded-* information as a comma+space
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
		UpdateHeader(&pr.Out.Header, "X-Forwarded-For", clientIP)
	}

	UpdateHeader(&pr.Out.Header, "X-Forwarded-Host", req.Host)

	var scheme string
	if req.TLS != nil {
		scheme = "https"
	} else {
		scheme = "http"
	}
	UpdateHeader(&pr.Out.Header, "X-Forwarded-Proto", scheme)

}
