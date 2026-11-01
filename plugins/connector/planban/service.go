package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

var statuses = map[string]bool{
	"pending": true, "up-next": true, "in-progress": true, "complete": true, "archived": true,
}

func validStatus(s string) bool { return statuses[strings.TrimSpace(s)] }

// repoID resolves the board: an explicit input wins over the instance config.
func repoID(cfgRepo, inputRepo string) (string, error) {
	for _, v := range []string{inputRepo, cfgRepo} {
		if v = strings.TrimSpace(v); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("repo_id is empty: set it in the connector config or pass repo_id")
}

// parseTags splits a comma-separated tag list, dropping blanks and duplicates.
func parseTags(raw string) []string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		t := strings.TrimSpace(part)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// mergeTags unions existing tags with new ones, preserving order.
func mergeTags(existing, add []string) []string {
	out := append([]string(nil), existing...)
	seen := map[string]bool{}
	for _, t := range existing {
		seen[t] = true
	}
	for _, t := range add {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// mergeMetadata copies existing metadata and overlays the JSON object in raw.
// A malformed raw is a caller error, not a silent drop.
func mergeMetadata(existing map[string]any, raw string) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range existing {
		out[k] = v
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out, nil
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(raw), &extra); err != nil {
		return nil, fmt.Errorf("metadata must be a JSON object: %w", err)
	}
	for k, v := range extra {
		out[k] = v
	}
	return out, nil
}

// shortcodeOf is the idempotency key: metadata.instagramShortcode, else
// metadata.externalId. sourcePosts only mirrors already-merged duplicates and
// must not decide identity.
func shortcodeOf(card card) string {
	for _, key := range []string{"instagramShortcode", "externalId"} {
		if v, ok := card.Metadata[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func findCardByShortcode(cards []card, code string) *card {
	for i := range cards {
		if shortcodeOf(cards[i]) == code {
			return &cards[i]
		}
	}
	return nil
}

func findCardByID(cards []card, id string) *card {
	for i := range cards {
		if cards[i].ID == id {
			return &cards[i]
		}
	}
	return nil
}

// findGroup matches a group title case-insensitively and reports the titles that
// exist, so an unknown group produces an actionable error instead of a bare miss.
func findGroup(cards []card, title string) (*card, []string) {
	want := strings.ToLower(strings.TrimSpace(title))
	var titles []string
	var hit *card
	for i := range cards {
		if !cards[i].IsGroup {
			continue
		}
		titles = append(titles, cards[i].Title)
		if hit == nil && strings.ToLower(cards[i].Title) == want {
			hit = &cards[i]
		}
	}
	sort.Strings(titles)
	return hit, titles
}

func groupName(cards []card, parentID *string) string {
	if parentID == nil {
		return ""
	}
	if g := findCardByID(cards, *parentID); g != nil {
		return g.Title
	}
	return *parentID
}

// cardView is the stable shape returned to the model.
func cardView(c card, cards []card) map[string]any {
	view := map[string]any{
		"id":      c.ID,
		"title":   c.Title,
		"status":  c.Status,
		"group":   groupName(cards, c.ParentID),
		"tags":    c.Tags,
		"summary": c.Summary,
	}
	if code := shortcodeOf(c); code != "" {
		view["shortcode"] = code
	}
	if url, ok := c.Metadata["sourceUrl"].(string); ok && url != "" {
		view["source_url"] = url
	}
	return view
}

// groupCounts returns one row per group with the number of cards it holds.
// Two passes on purpose: card order in the roadmap is not guaranteed to place
// groups before their children.
func groupCounts(cards []card) []map[string]any {
	counts := map[string]int{}
	for _, c := range cards {
		if c.IsGroup || c.ParentID == nil {
			continue
		}
		counts[*c.ParentID]++
	}
	out := make([]map[string]any, 0, len(counts)+1)
	for _, g := range cards {
		if !g.IsGroup {
			continue
		}
		out = append(out, map[string]any{"id": g.ID, "title": g.Title, "cards": counts[g.ID]})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i]["cards"].(int) > out[j]["cards"].(int)
	})
	return out
}

func statusCounts(cards []card) map[string]int {
	out := map[string]int{}
	for _, c := range cards {
		if c.IsGroup {
			continue
		}
		out[c.Status]++
	}
	return out
}

func cardCount(cards []card) int {
	n := 0
	for _, c := range cards {
		if !c.IsGroup {
			n++
		}
	}
	return n
}
