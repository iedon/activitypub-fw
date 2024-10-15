package proxy

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

const (
	SERVER_NAME      = "ActivityPub-FW"
	SERVER_VERSION   = "0.1"
	SERVER_SIGNATURE = SERVER_NAME + "/" + SERVER_VERSION
)

func modifyResponse(r *http.Response) error {
	r.Header.Set("Server", SERVER_SIGNATURE)

	// Will catch these patterns
	const (
		updatePattern = "/api/i/update"
		createPattern = "/api/notes/create"
		inboxPattern  = "/inbox"
		maxBodySize   = 1048576
	)

	usersPattern := regexp.MustCompile(`^/users/.*`)

	_url := r.Request.RequestURI
	matched := _url == updatePattern || _url == createPattern || _url == inboxPattern || usersPattern.MatchString(_url)

	if matched {
		// Catch and inspect current request
		log.Printf("[ACCESS] %s - %s\n", r.Request.RequestURI, r.Request.RemoteAddr)

		if r.Request.Method != http.MethodPost {
			// No request body in this case
			// Passthrough this response
			return nil
		}

		if contentType := r.Request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
			// Invalid request body content type
			// Passthrough this response
			log.Printf("[WARN] %s - %s, %s: %s\n", r.Request.RequestURI, r.Request.RemoteAddr, "invalid content type", contentType)
			return nil
		}

		if r.ContentLength != -1 && r.ContentLength > maxBodySize {
			// Unknown or invalid response from upstream for current request
			// Passthrough this response
			log.Printf("[WARN] %s - %s, %s: %d of %d\n", r.Request.RequestURI, r.Request.RemoteAddr, "invalid or unknown content length", r.ContentLength, maxBodySize)
			return nil
		}

		bodyReader, err := r.Request.GetBody()
		if err != nil {
			return err
		}

		reqBody, err := io.ReadAll(bodyReader)
		if err != nil {
			return err
		}

		// TODO: deserialize request body and check

		// This request should be banned
		if true {
			// Set response status
			r.StatusCode = http.StatusForbidden
			r.Status = http.StatusText(http.StatusForbidden)

			// Clear upstream header and reassign an empty one
			r.Header = make(http.Header)

			// Reassign body
			buf := bytes.NewBufferString("TODO")
			buf.Write(reqBody)
			r.Body = io.NopCloser(buf)

			// Rebuild headers
			r.Header.Set("Server", SERVER_SIGNATURE)
			r.Header.Set("Content-Type", "application/json; charset=utf-8")
			r.Header.Set("Content-Length", fmt.Sprint(buf.Len()))

			log.Println(string(reqBody))
		}

		return nil
	}

	// Passthrough current response
	return nil
}
