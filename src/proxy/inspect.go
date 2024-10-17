package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

func NeedsInspect(req *http.Request) bool {
	// Will catch these patterns
	const (
		updatePattern = "/api/i/update"
		createPattern = "/api/notes/create"
		inboxPattern  = "/inbox"
		usersPattern  = `^/users/.*`
		maxBodySize   = 1048576
	)

	// Catch and check wether current request needs further process
	// log.Printf("[ACCESS] %s - %s - %s\n", req.RequestURI, req.RemoteAddr, req.UserAgent())

	if req.Method != http.MethodPost {
		// No request body in this case
		// Passthrough this response
		return false
	}

	if contentType := strings.ToLower(req.Header.Get("Content-Type")); !strings.HasPrefix(contentType, "application/activity+json") && !strings.HasPrefix(contentType, "application/ld+json") && !strings.HasPrefix(contentType, "application/json") {
		// Invalid request body content type
		// Passthrough this response
		log.Printf("[WARN] %s - %s, %s: %s\n", req.RequestURI, req.RemoteAddr, "invalid content type", contentType)
		return false
	}

	if req.ContentLength != -1 && req.ContentLength > maxBodySize {
		// Unknown or invalid response from upstream for current request
		// Passthrough this response
		log.Printf("[WARN] %s - %s, %s: %d of %d\n", req.RequestURI, req.RemoteAddr, "invalid or unknown content length", req.ContentLength, maxBodySize)
		return false
	}

	usersPatternRegex := regexp.MustCompile(usersPattern)

	uri := req.RequestURI
	return uri == updatePattern || uri == createPattern || uri == inboxPattern || usersPatternRegex.MatchString(uri)
}

func InspectRequest(resp *http.Response, body []byte) error {
	req := resp.Request

	var bodyJson map[string]interface{}
	err := json.Unmarshal(body, &bodyJson)
	if err != nil {
		// Passthrough
		return nil
	}

	cc := 0
	if ccSlice, ok := bodyJson["cc"].([]interface{}); ok {
		cc = len(ccSlice)
	}

	text := ""
	if t, ok := bodyJson["text"].(string); ok {
		text = t
	}

	atRegex := regexp.MustCompile(`@(\w+)(?:@([\w.-]+))?`)
	mentions := atRegex.FindAllString(text, -1)

	if len(mentions) > 5 {
		log.Printf("[WARN] %s - %s - %s, reaching mention limit: %d of %d\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), len(mentions), 5)
		log.Println(string(body))
		BadRequest(resp, req.RequestURI, "Your request has been filterd for too many Ats in filed text.")
		return nil
	}

	if cc > 5 {
		log.Printf("[WARN] %s - %s - %s, reaching cc limit: %d of %d\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), cc, 5)
		log.Println(string(body))
		BadRequest(resp, req.RequestURI, "Your request has been filterd for too many mensions in filed cc.")
	}

	return nil
}

// Note: Returning a 400 in ActivityPub may result in repeated retries from the remote
// or, in the worst case, delivery suspension. Therefore, return a 202 for 'inbox'.
func BadRequest(r *http.Response, requestURI string, message string) {
	// Set response status
	if strings.Contains(requestURI, "inbox") {
		r.StatusCode = http.StatusAccepted
		r.Status = http.StatusText(http.StatusAccepted)
	} else {
		r.StatusCode = http.StatusBadRequest
		r.Status = http.StatusText(http.StatusBadRequest)
	}

	// Reassign body
	buf := bytes.NewBufferString(fmt.Sprintf("{\"code\": %d,\"status\":\"%s\",\"message\":\"%s\"}", r.StatusCode, r.Status, message))
	r.Body.Close()
	r.Body = io.NopCloser(buf)

	// Clear upstream header and reassign an empty one
	r.Header = make(http.Header)

	// Rebuild headers
	SetProductInfo(&r.Header)
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	r.Header.Set("Content-Length", fmt.Sprint(buf.Len()))
	r.Header.Set("Cache-Control", "no-cache, no-store")
	r.Header.Set("Pragma", "no-cache")
}
