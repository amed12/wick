package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/yogasw/wick/pkg/connector"
)

// clientFor builds the board client for this call and resolves the repo id.
func clientFor(c *connector.Ctx, inputRepo string) (*client, string, error) {
	repo, err := repoID(c.Cfg("repo_id"), inputRepo)
	if err != nil {
		return nil, "", err
	}
	base := strings.TrimSpace(c.Cfg("base_url"))
	if base == "" {
		base = "http://127.0.0.1:4317"
	}
	return newClient(c.Context(), c.HTTP, base), repo, nil
}

// ---------------------------------------------------------------- board

func handleBoardState(c *connector.Ctx) (any, error) {
	cl, repo, err := clientFor(c, c.Input("repo_id"))
	if err != nil {
		return nil, err
	}
	cards, err := cl.cards(repo)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"board":    repo,
		"cards":    cardCount(cards),
		"statuses": statusCounts(cards),
		"groups":   groupCounts(cards),
	}, nil
}

func handleFindCards(c *connector.Ctx) (any, error) {
	cl, repo, err := clientFor(c, c.Input("repo_id"))
	if err != nil {
		return nil, err
	}
	cards, err := cl.cards(repo)
	if err != nil {
		return nil, err
	}

	query := strings.ToLower(strings.TrimSpace(c.Input("query")))
	code := strings.TrimSpace(c.Input("shortcode"))
	status := strings.TrimSpace(c.Input("status"))
	groupFilter := strings.TrimSpace(c.Input("group"))
	metaKey := strings.TrimSpace(c.Input("metadata_key"))
	metaValue := strings.TrimSpace(c.Input("metadata_value"))
	limit := c.InputInt("limit")
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	var groupID string
	if groupFilter != "" {
		g, titles := findGroup(cards, groupFilter)
		if g == nil {
			return nil, fmt.Errorf("group %q not found; available: %s", groupFilter, strings.Join(titles, ", "))
		}
		groupID = g.ID
	}

	out := make([]map[string]any, 0, limit)
	total := 0
	for _, card := range cards {
		if card.IsGroup {
			continue
		}
		if status != "" && card.Status != status {
			continue
		}
		if groupID != "" && (card.ParentID == nil || *card.ParentID != groupID) {
			continue
		}
		if metaKey != "" && asMetaString(card.Metadata[metaKey]) != metaValue {
			continue
		}
		if code != "" && shortcodeOf(card) != code {
			continue
		}
		if query != "" {
			hay := strings.ToLower(card.Title + " " + card.Summary + " " + groupName(cards, card.ParentID))
			if !strings.Contains(hay, query) {
				continue
			}
		}
		total++
		if len(out) < limit {
			out = append(out, cardView(card, cards))
		}
	}
	return map[string]any{"board": repo, "matched": total, "returned": len(out), "cards": out}, nil
}

// ---------------------------------------------------------------- upsert

func handleUpsertCard(c *connector.Ctx) (any, error) {
	cl, repo, err := clientFor(c, c.Input("repo_id"))
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(c.Input("title"))
	code := strings.TrimSpace(c.Input("shortcode"))
	groupTitle := strings.TrimSpace(c.Input("group"))
	status := strings.TrimSpace(c.Input("status"))
	if status != "" && !validStatus(status) {
		return nil, fmt.Errorf("invalid status %q (pending|up-next|in-progress|complete|archived)", status)
	}

	cards, err := cl.cards(repo)
	if err != nil {
		return nil, err
	}
	existing := (*card)(nil)
	if code != "" {
		existing = findCardByShortcode(cards, code)
	}

	var parentID *string
	if groupTitle != "" {
		g, titles := findGroup(cards, groupTitle)
		if g == nil {
			return nil, fmt.Errorf("group %q not found; create it first or use one of: %s", groupTitle, strings.Join(titles, ", "))
		}
		id := g.ID
		parentID = &id
	}

	// ---- update
	if existing != nil {
		patch := map[string]any{}
		if title != "" {
			patch["title"] = title
		}
		if s := strings.TrimSpace(c.Input("summary")); s != "" {
			patch["summary"] = s
		}
		if tags := parseTags(c.Input("tags")); len(tags) > 0 {
			patch["tags"] = mergeTags(existing.Tags, tags)
		}
		meta := existing.Metadata
		if src := strings.TrimSpace(c.Input("source_url")); src != "" {
			if meta == nil {
				meta = map[string]any{}
			}
			meta["sourceUrl"] = src
		}
		merged, err := mergeMetadata(meta, c.Input("metadata"))
		if err != nil {
			return nil, err
		}
		if len(merged) > 0 {
			patch["metadata"] = merged
		}
		if len(patch) > 0 {
			if _, err := cl.patchCard(repo, existing.ID, patch); err != nil {
				return nil, err
			}
		}
		if parentID != nil || status != "" {
			move := map[string]any{}
			if parentID != nil {
				move["parentId"] = *parentID
			}
			if status != "" {
				move["status"] = status
			}
			if _, err := cl.moveCard(repo, existing.ID, move); err != nil {
				return nil, err
			}
		}
		return finish(cl, repo, existing.ID, "updated")
	}

	// ---- create
	if title == "" {
		return nil, fmt.Errorf("title is required: no card carries shortcode %q yet", code)
	}
	meta, err := mergeMetadata(nil, c.Input("metadata"))
	if err != nil {
		return nil, err
	}
	if code != "" {
		meta["instagramShortcode"] = code
	}
	if src := strings.TrimSpace(c.Input("source_url")); src != "" {
		meta["sourceUrl"] = src
	}
	body := map[string]any{"title": title, "metadata": meta}
	if s := strings.TrimSpace(c.Input("summary")); s != "" {
		body["summary"] = s
	}
	if tags := parseTags(c.Input("tags")); len(tags) > 0 {
		body["tags"] = tags
	}
	if parentID != nil {
		body["parentId"] = *parentID
	}
	if status != "" {
		body["status"] = status
	}
	if _, err := cl.createCard(repo, body); err != nil {
		return nil, err
	}
	// Planban generates the card id from the title, so read the board back to
	// report the row that now carries this shortcode.
	after, err := cl.cards(repo)
	if err != nil {
		return nil, err
	}
	created := findCardByShortcode(after, code)
	if created == nil {
		created = findCardByID(after, slugID(title))
	}
	if created == nil {
		return map[string]any{"board": repo, "action": "created", "shortcode": code}, nil
	}
	return finish(cl, repo, created.ID, "created")
}

// finish re-reads the board and returns the view of one card.
func finish(cl *client, repo, id, action string) (any, error) {
	cards, err := cl.cards(repo)
	if err != nil {
		return nil, err
	}
	target := findCardByID(cards, id)
	if target == nil {
		return map[string]any{"board": repo, "action": action, "card_id": id}, nil
	}
	return map[string]any{"board": repo, "action": action, "card": cardView(*target, cards)}, nil
}

// ---------------------------------------------------------------- group

func handleCreateGroup(c *connector.Ctx) (any, error) {
	cl, repo, err := clientFor(c, c.Input("repo_id"))
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(c.Input("title"))
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}
	cards, err := cl.cards(repo)
	if err != nil {
		return nil, err
	}
	if g, _ := findGroup(cards, title); g != nil {
		return map[string]any{"board": repo, "action": "exists", "group_id": g.ID, "title": g.Title}, nil
	}
	body := map[string]any{"title": title}
	if s := strings.TrimSpace(c.Input("summary")); s != "" {
		body["summary"] = s
	}
	if _, err := cl.createGroup(repo, body); err != nil {
		return nil, err
	}
	after, err := cl.cards(repo)
	if err != nil {
		return nil, err
	}
	if g, _ := findGroup(after, title); g != nil {
		return map[string]any{"board": repo, "action": "created", "group_id": g.ID, "title": g.Title}, nil
	}
	return map[string]any{"board": repo, "action": "created", "title": title}, nil
}

// ---------------------------------------------------------------- move / delete

func handleMoveCard(c *connector.Ctx) (any, error) {
	cl, repo, err := clientFor(c, c.Input("repo_id"))
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(c.Input("card_id"))
	if id == "" {
		return nil, fmt.Errorf("card_id is required")
	}
	status := strings.TrimSpace(c.Input("status"))
	if status != "" && !validStatus(status) {
		return nil, fmt.Errorf("invalid status %q (pending|up-next|in-progress|complete|archived)", status)
	}
	groupTitle := strings.TrimSpace(c.Input("group"))
	if groupTitle == "" && status == "" {
		return nil, fmt.Errorf("pass group and/or status — nothing to change")
	}
	body := map[string]any{}
	if groupTitle != "" {
		cards, err := cl.cards(repo)
		if err != nil {
			return nil, err
		}
		g, titles := findGroup(cards, groupTitle)
		if g == nil {
			return nil, fmt.Errorf("group %q not found; available: %s", groupTitle, strings.Join(titles, ", "))
		}
		body["parentId"] = g.ID
	}
	if status != "" {
		body["status"] = status
	}
	if _, err := cl.moveCard(repo, id, body); err != nil {
		return nil, err
	}
	return finish(cl, repo, id, "moved")
}

func handleDeleteCard(c *connector.Ctx) (any, error) {
	cl, repo, err := clientFor(c, c.Input("repo_id"))
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(c.Input("card_id"))
	if id == "" {
		return nil, fmt.Errorf("card_id is required")
	}
	if err := cl.deleteCard(repo, id); err != nil {
		return nil, err
	}
	return map[string]any{"board": repo, "deleted": true, "card_id": id}, nil
}

func asMetaString(v any) string {
	s, _ := v.(string)
	return s
}

// slugID mirrors Planban's card-id derivation, used only as a fallback lookup
// immediately after a create.
func slugID(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// handleWriteDoc replaces one card document. Planban's PUT is a full replace, so
// the caller supplies the whole body.
func handleWriteDoc(c *connector.Ctx) (any, error) {
	cl, repo, err := clientFor(c, c.Input("repo_id"))
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(c.Input("card_id"))
	if id == "" {
		return nil, fmt.Errorf("card_id is required")
	}
	kind := strings.TrimSpace(c.Input("kind"))
	if kind == "" {
		kind = "spec"
	}
	if kind != "spec" && kind != "plan" {
		return nil, fmt.Errorf("kind must be spec or plan")
	}
	markdown := c.Input("markdown")
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("markdown is required")
	}
	var out map[string]any
	err = cl.do(http.MethodPut, boardPath(repo, "/cards/"+url.PathEscape(id)+"/docs/"+kind),
		map[string]any{"markdown": markdown}, &out)
	if err != nil {
		return nil, err
	}
	return map[string]any{"board": repo, "card_id": id, "kind": kind, "written": true, "path": out["path"]}, nil
}

// handleSetMetadata merges metadata into one card, keyed by card id so cards
// without an external shortcode can still be updated.
func handleSetMetadata(c *connector.Ctx) (any, error) {
	cl, repo, err := clientFor(c, c.Input("repo_id"))
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(c.Input("card_id"))
	if id == "" {
		return nil, fmt.Errorf("card_id is required")
	}
	cards, err := cl.cards(repo)
	if err != nil {
		return nil, err
	}
	target := findCardByID(cards, id)
	if target == nil {
		return nil, fmt.Errorf("card %q not found on board %s", id, repo)
	}
	merged, err := mergeMetadata(target.Metadata, c.Input("metadata"))
	if err != nil {
		return nil, err
	}
	if _, err := cl.patchCard(repo, id, map[string]any{"metadata": merged}); err != nil {
		return nil, err
	}
	return finish(cl, repo, id, "metadata_updated")
}
