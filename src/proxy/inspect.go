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

	"github.com/iedon/activitypub-fw/config"
)

func needsInspect(req *http.Request, limit *config.LimitConfig) bool {
	// Catch and check wether current request needs further process
	// log.Printf("[ACCESS] %s - %s - %s\n", req.RequestURI, req.RemoteAddr, req.UserAgent())

	if req.Method != http.MethodPost {
		// No request body in this case
		// Passthrough this response
		return false
	}

	// Will catch these patterns
	const (
		updatePattern = "/api/i/update"
		createPattern = "/api/notes/create"
		inboxPattern  = "/inbox"
		usersPattern  = `^/users/.*`
	)

	usersPatternRegex, err := regexp.Compile(usersPattern)
	if err != nil {
		return false
	}

	uri := req.RequestURI
	if uri != updatePattern && uri != createPattern && uri != inboxPattern && !usersPatternRegex.MatchString(uri) {
		// Not target uri, passthrough
		return false
	}

	if contentType := strings.ToLower(req.Header.Get("Content-Type")); !strings.HasPrefix(contentType, "application/activity+json") && !strings.HasPrefix(contentType, "application/ld+json") && !strings.HasPrefix(contentType, "application/json") {
		// Invalid request body content type
		// Passthrough this response
		log.Printf("[WARN] %s - %s, %s: %s\n", req.RequestURI, req.RemoteAddr, "invalid content type", contentType)
		return false
	}

	if req.ContentLength != -1 && req.ContentLength > limit.MaxBodySize {
		// Unknown or invalid response from upstream for current request
		// Passthrough this response
		log.Printf("[WARN] %s - %s, body too long or content length is unknwon: %d/%d\n", req.RequestURI, req.RemoteAddr, req.ContentLength, limit.MaxBodySize)
		return false
	}

	return true
}

func inspectRequest(resp *http.Response, body []byte, limit *config.LimitConfig) error {
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

	if cc > limit.Cc {
		log.Printf("[WARN] %s - %s - %s, reaching cc limit: %d/%d\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), cc, limit.Cc)
		log.Println(string(body))
		badRequest(resp, req.RequestURI, "Your request has been filtered for too many items in filed cc.")
		return nil
	}

	mentions := countMentions(&bodyJson)
	if mentions > limit.Mentions {
		log.Printf("[WARN] %s - %s - %s, reaching mention limit: %d/%d\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), mentions, limit.Mentions)
		log.Println(string(body))
		badRequest(resp, req.RequestURI, "Your request has been filtered for too many mentions.")
		return nil
	}

	if hit, keyword := hasBadWords(&bodyJson, limit); hit {
		log.Printf("[WARN] %s - %s - %s, hitting keyword: %s\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), keyword)
		log.Println(string(body))
		badRequest(resp, req.RequestURI, "Your request has been filtered for having keywords that denied by our server.")
		return nil
	}

	return nil
}

func countMentions(body *map[string]interface{}) int {
	// Check if "object" exists and is a map
	object, ok := (*body)["object"].(map[string]interface{})
	if !ok {
		return 0 // Return 0 if object is not a map
	}

	// Check if "tag" exists and is a slice
	tags, ok := object["tag"].([]interface{})
	if !ok {
		return 0 // Return 0 if tag is not a slice
	}

	count := 0
	for _, tag := range tags {
		if tagMap, ok := tag.(map[string]interface{}); ok {
			if tagType, ok := tagMap["type"].(string); ok && strings.ToLower(tagType) == "mention" {
				count++
			}
		}
	}

	return count
}

func hasBadWords(body *map[string]interface{}, limit *config.LimitConfig) (bool, string) {
	content := ""

	// Check body.content
	if t, ok := (*body)["content"].(string); ok {
		content = t
	} else {
		// content is not in body, check body.object.content
		// Check if "object" exists and is a map
		object, ok := (*body)["object"].(map[string]interface{})
		if !ok {
			return false, "" // Return false if object is not a map
		}

		if t, ok := object["content"].(string); ok {
			content = t
		}
	}

	if content == "" {
		return false, ""
	}

	for _, keyword := range limit.Keywords {
		if strings.Contains(content, keyword) {
			return true, keyword
		}
	}

	return false, ""
}

// Note: Returning a 400 in ActivityPub may result in repeated retries from the remote
// or, in the worst case, delivery suspension. Therefore, return a 202 for 'inbox'.
func badRequest(r *http.Response, requestURI string, message string) {
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
	setProductInfo(&r.Header)
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	r.Header.Set("Content-Length", fmt.Sprint(buf.Len()))
	r.Header.Set("Cache-Control", "no-cache, no-store")
	r.Header.Set("Pragma", "no-cache")
}
