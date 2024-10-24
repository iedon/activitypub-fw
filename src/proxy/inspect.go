package proxy

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/iedon/activitypub-fw/config"
)

func needsInspect(req *http.Request, cfg *config.Config) bool {
	// Catch and check wether current request needs further process
	if cfg.Config.Server.Debug {
		log.Printf("[ACCESS] %s - %s - %s\n", req.RequestURI, req.RemoteAddr, req.UserAgent())
	}

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

	if req.ContentLength != -1 && req.ContentLength > cfg.Config.Limit.MaxBodySize {
		// Unknown or invalid response from upstream for current request
		// Passthrough this response
		log.Printf("[WARN] %s - %s, body too long or content length is unknwon: %d/%d\n", req.RequestURI, req.RemoteAddr, req.ContentLength, cfg.Config.Limit.MaxBodySize)
		return false
	}

	return true
}

// Empty string if the request passed/should skip the inspection, anything else for the reason this request should be filtered.
func inspectRequest(req *http.Request, body []byte, cfg *config.Config) string {
	var bodyJson map[string]interface{}
	err := json.Unmarshal(body, &bodyJson)
	if err != nil {
		// Passthrough
		return ""
	}

	if cfg.Config.Server.Debug {
		log.Println(string(body))
	}

	cc := 0
	if ccSlice, ok := bodyJson["cc"].([]interface{}); ok {
		cc = len(ccSlice)
	}

	if cfg.Config.Server.Debug {
		log.Printf("[INFO] %s - %s - %s, cc limit: %d/%d\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), cc, cfg.Config.Limit.Cc)
	}

	if cc > cfg.Config.Limit.Cc {
		log.Printf("[WARN] %s - %s - %s, reaching cc limit: %d/%d\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), cc, cfg.Config.Limit.Cc)
		log.Println(string(body))
		return "Your request has been filtered for too many items in filed cc."
	}

	mentions := countMentions(&bodyJson)

	if cfg.Config.Server.Debug {
		log.Printf("[INFO] %s - %s - %s, mention limit: %d/%d\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), mentions, cfg.Config.Limit.Mentions)
	}

	if mentions > cfg.Config.Limit.Mentions {
		log.Printf("[WARN] %s - %s - %s, reaching mention limit: %d/%d\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), mentions, cfg.Config.Limit.Mentions)
		log.Println(string(body))
		return "Your request has been filtered for too many mentions."
	}

	if hit, keyword := hasBadWords(&bodyJson, cfg); hit {
		log.Printf("[WARN] %s - %s - %s, hitting keyword: %s\n", req.RequestURI, req.RemoteAddr, req.UserAgent(), keyword)
		log.Println(string(body))
		return "Your request has been filtered for having keywords that denied by our server."
	}

	return ""
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

func hasBadWords(body *map[string]interface{}, cfg *config.Config) (bool, string) {
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

	for _, keyword := range cfg.Config.Limit.Keywords {
		if strings.Contains(content, keyword) {
			return true, keyword
		}
	}

	return false, ""
}
