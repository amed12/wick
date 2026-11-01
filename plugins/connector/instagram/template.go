package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	wickplugin "github.com/yogasw/wick/pkg/plugin"
)

// parseCollectionRef accepts what a human would paste:
//
//	https://www.instagram.com/<user>/saved/ai/18326269657221437/
//	ai/18326269657221437
func parseCollectionRef(raw string) (slug, id string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("collection is required (a saved-collection URL or slug/id)")
	}
	if strings.Contains(raw, "instagram.com") {
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", fmt.Errorf("parse collection URL: %w", perr)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := 0; i < len(parts); i++ {
			if parts[i] == "saved" && i+2 < len(parts) {
				return parts[i+1], parts[i+2], nil
			}
		}
		return "", "", fmt.Errorf("URL %q is not a saved-collection URL (expected /<user>/saved/<slug>/<id>/)", raw)
	}
	if slug, id, ok := strings.Cut(raw, "/"); ok {
		slug, id = strings.TrimSpace(slug), strings.TrimSpace(id)
		if slug != "" && id != "" {
			return slug, id, nil
		}
	}
	return "", "", fmt.Errorf("collection %q must be a saved-collection URL or slug/id, e.g. ai/18326269657221437", raw)
}

// collectionURL is the page a browser must open before the GraphQL template can
// be captured: the SPA only issues that request once it renders the grid.
func collectionURL(username, slug, id string) string {
	return fmt.Sprintf("https://www.instagram.com/%s/saved/%s/%s/", username, slug, id)
}

func savedURL(username string) string {
	return fmt.Sprintf("https://www.instagram.com/%s/saved/", username)
}

// ---------------------------------------------------------------- template cache

// cacheTTL bounds how long a captured GraphQL body is trusted. Instagram rotates
// doc_id and the fb_dtsg token, so the cache is a convenience, not a contract:
// every harvest that fails invalidates it and rediscovers once.
const cacheTTL = 12 * time.Hour

func templatePath(collectionID string) string {
	dir := wickplugin.DataDir("instagram")
	return filepath.Join(dir, "template-"+sanitize(collectionID)+".txt")
}

// loadTemplate returns a cached body when it is fresh.
func loadTemplate(collectionID string) string {
	path := templatePath(collectionID)
	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) > cacheTTL {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func saveTemplate(collectionID, body string) error {
	path := templatePath(collectionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create template dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write template cache: %w", err)
	}
	return nil
}

func dropTemplate(collectionID string) {
	_ = os.Remove(templatePath(collectionID))
}

// extractTemplate picks the collection query out of a captured request list.
// The body is the form-encoded POST payload the page itself sent.
func extractTemplate(requests []any, collectionID string) (string, error) {
	for _, raw := range requests {
		m := asMap(raw)
		if m == nil {
			continue
		}
		if !strings.Contains(strings.ToUpper(asString(m["method"])), "POST") {
			continue
		}
		body := asString(m["body"])
		if body == "" {
			continue
		}
		if !strings.Contains(body, "doc_id=") || !strings.Contains(body, "variables=") {
			continue
		}
		if collectionID != "" && !strings.Contains(body, collectionID) {
			continue
		}
		return body, nil
	}
	return "", fmt.Errorf("no saved-collection GraphQL request was recorded — is the profile logged in, and did the collection page render?")
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
