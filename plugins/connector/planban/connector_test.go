package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/yogasw/wick/pkg/connector"
)

// fakeBoard is an in-memory stand-in for the Planban HTTP API, covering only
// the routes this connector calls.
type fakeBoard struct {
	mu    sync.Mutex
	cards []card
}

func (f *fakeBoard) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/boards/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		path := strings.TrimPrefix(r.URL.Path, "/api/boards/")
		repo, rest, _ := strings.Cut(path, "/")
		if repo == "" {
			http.Error(w, "repo required", http.StatusNotFound)
			return
		}
		// Planban refuses mutating requests that do not state the origin.
		if r.Method != http.MethodGet {
			if r.Header.Get("Sec-Fetch-Site") != "same-origin" || r.Header.Get("Origin") == "" {
				http.Error(w, `{"error":"mutating request must come from the planban origin"}`, http.StatusForbidden)
				return
			}
		}

		switch {
		case r.Method == http.MethodGet && rest == "cards":
			writeJSON(w, f.cards)

		case r.Method == http.MethodGet && rest == "state":
			writeJSON(w, boardState{Roadmap: struct {
				Revision     int    `json:"revision"`
				RoadmapItems []card `json:"roadmapItems"`
			}{Revision: len(f.cards), RoadmapItems: f.cards}})

		case r.Method == http.MethodPost && rest == "cards":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			c := card{
				ID:      slugID(asString(body["title"])),
				Title:   asString(body["title"]),
				Status:  firstNonEmpty(asString(body["status"]), "pending"),
				Summary: asString(body["summary"]),
			}
			if p, ok := body["parentId"].(string); ok && p != "" {
				c.ParentID = &p
			}
			if tags, ok := body["tags"].([]any); ok {
				for _, tg := range tags {
					c.Tags = append(c.Tags, asString(tg))
				}
			}
			if md, ok := body["metadata"].(map[string]any); ok {
				c.Metadata = md
			}
			f.cards = append(f.cards, c)
			writeJSON(w, map[string]any{"roadmap": map[string]any{"revision": len(f.cards), "roadmapItems": f.cards}})

		case r.Method == http.MethodPost && strings.HasSuffix(rest, "/move"):
			id := strings.TrimSuffix(strings.TrimPrefix(rest, "cards/"), "/move")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			for i := range f.cards {
				if f.cards[i].ID != id {
					continue
				}
				if p, ok := body["parentId"].(string); ok {
					f.cards[i].ParentID = &p
				}
				if s := asString(body["status"]); s != "" {
					f.cards[i].Status = s
				}
			}
			writeJSON(w, map[string]any{"roadmap": map[string]any{"revision": len(f.cards), "roadmapItems": f.cards}})

		case r.Method == http.MethodPost && rest == "groups":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			title := asString(body["title"])
			f.cards = append(f.cards, card{ID: slugID(title), Title: title, Status: "pending", IsGroup: true, Summary: asString(body["summary"])})
			writeJSON(w, map[string]any{"roadmap": map[string]any{"revision": len(f.cards), "roadmapItems": f.cards}})

		case r.Method == http.MethodPatch && strings.HasPrefix(rest, "cards/"):
			id := strings.TrimPrefix(rest, "cards/")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			for i := range f.cards {
				if f.cards[i].ID != id {
					continue
				}
				if v := asString(body["title"]); v != "" {
					f.cards[i].Title = v
				}
				if v := asString(body["summary"]); v != "" {
					f.cards[i].Summary = v
				}
				if tags, ok := body["tags"].([]any); ok {
					f.cards[i].Tags = nil
					for _, tg := range tags {
						f.cards[i].Tags = append(f.cards[i].Tags, asString(tg))
					}
				}
				if md, ok := body["metadata"].(map[string]any); ok {
					f.cards[i].Metadata = md
				}
			}
			writeJSON(w, map[string]any{"roadmap": map[string]any{"revision": len(f.cards), "roadmapItems": f.cards}})

		case r.Method == http.MethodDelete && strings.HasPrefix(rest, "cards/"):
			id := strings.TrimPrefix(rest, "cards/")
			kept := f.cards[:0]
			for _, c := range f.cards {
				if c.ID != id {
					kept = append(kept, c)
				}
			}
			f.cards = kept
			writeJSON(w, map[string]any{"ok": true})

		default:
			http.Error(w, "unexpected route "+r.Method+" "+rest, http.StatusNotFound)
		}
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// newCtx builds a connector.Ctx pointed at the fake board.
func newCtx(t *testing.T, srv *httptest.Server, input map[string]string) *connector.Ctx {
	t.Helper()
	cfg := map[string]string{"base_url": srv.URL, "repo_id": "demo"}
	return connector.NewCtx(context.Background(), "test-instance", cfg, input, nil, nil, nil)
}

func TestUpsertIsIdempotent(t *testing.T) {
	board := &fakeBoard{}
	srv := httptest.NewServer(board.handler(t))
	defer srv.Close()

	first, err := handleUpsertCard(newCtx(t, srv, map[string]string{
		"shortcode": "ABC123", "title": "DeepSeek V4.1", "summary": "rilis model", "tags": "ai,model",
	}))
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if got := first.(map[string]any)["action"]; got != "created" {
		t.Fatalf("first action = %v, want created", got)
	}

	second, err := handleUpsertCard(newCtx(t, srv, map[string]string{
		"shortcode": "ABC123", "title": "DeepSeek V4.1 Flash", "tags": "rilis",
	}))
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if got := second.(map[string]any)["action"]; got != "updated" {
		t.Fatalf("second action = %v, want updated", got)
	}
	if len(board.cards) != 1 {
		t.Fatalf("board holds %d cards, want 1 (upsert must not duplicate)", len(board.cards))
	}
	got := board.cards[0]
	if got.Title != "DeepSeek V4.1 Flash" {
		t.Errorf("title = %q, want updated title", got.Title)
	}
	if strings.Join(got.Tags, ",") != "ai,model,rilis" {
		t.Errorf("tags = %v, want merged ai,model,rilis", got.Tags)
	}
	if got.Metadata["instagramShortcode"] != "ABC123" {
		t.Errorf("metadata lost the idempotency key: %v", got.Metadata)
	}
}

func TestUpsertRejectsUnknownGroup(t *testing.T) {
	board := &fakeBoard{cards: []card{{ID: "ai", Title: "AI", IsGroup: true, Status: "pending"}}}
	srv := httptest.NewServer(board.handler(t))
	defer srv.Close()

	_, err := handleUpsertCard(newCtx(t, srv, map[string]string{
		"shortcode": "X1", "title": "nope", "group": "Topik Tidak Ada",
	}))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("want a group-not-found error, got %v", err)
	}
	if !strings.Contains(err.Error(), "AI") {
		t.Errorf("error should list available groups, got: %v", err)
	}
}

func TestFindCardsByShortcodeAndStatus(t *testing.T) {
	parent := "ai"
	board := &fakeBoard{cards: []card{
		{ID: "ai", Title: "AI", IsGroup: true, Status: "pending"},
		{ID: "one", Title: "Kartu satu", Status: "pending", ParentID: &parent, Metadata: map[string]any{"instagramShortcode": "AAA"}},
		{ID: "two", Title: "Kartu dua", Status: "up-next", ParentID: &parent, Metadata: map[string]any{"instagramShortcode": "BBB"}},
	}}
	srv := httptest.NewServer(board.handler(t))
	defer srv.Close()

	out, err := handleFindCards(newCtx(t, srv, map[string]string{"shortcode": "BBB"}))
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	got := out.(map[string]any)
	if got["matched"].(int) != 1 {
		t.Fatalf("matched = %v, want 1", got["matched"])
	}
	view := got["cards"].([]map[string]any)[0]
	if view["id"] != "two" || view["group"] != "AI" {
		t.Errorf("view = %v, want card two under group AI", view)
	}

	out, err = handleFindCards(newCtx(t, srv, map[string]string{"group": "ai", "status": "pending"}))
	if err != nil {
		t.Fatalf("find by group: %v", err)
	}
	if got := out.(map[string]any)["matched"].(int); got != 1 {
		t.Errorf("matched under group+status = %d, want 1", got)
	}
}

func TestDeleteCardRemovesIt(t *testing.T) {
	board := &fakeBoard{cards: []card{{ID: "gone", Title: "Hapus", Status: "pending"}}}
	srv := httptest.NewServer(board.handler(t))
	defer srv.Close()

	if _, err := handleDeleteCard(newCtx(t, srv, map[string]string{"card_id": "gone"})); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(board.cards) != 0 {
		t.Fatalf("card survived delete: %v", board.cards)
	}
}

func TestMutationRequiresOriginHeader(t *testing.T) {
	board := &fakeBoard{}
	handler := board.handler(t)
	req := httptest.NewRequest(http.MethodPost, "http://planban.local/api/boards/demo/cards", strings.NewReader(`{"title":"x"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 without origin headers", rec.Code)
	}
}

// TestLiveBoard exercises the read-only ops against a real Planban server.
// Opt in with: PLANBAN_LIVE_URL=http://127.0.0.1:4317 PLANBAN_LIVE_REPO=<repoId> go test -run Live
func TestLiveBoard(t *testing.T) {
	base := os.Getenv("PLANBAN_LIVE_URL")
	if base == "" {
		t.Skip("PLANBAN_LIVE_URL not set")
	}
	repo := os.Getenv("PLANBAN_LIVE_REPO")
	if repo == "" {
		t.Fatal("PLANBAN_LIVE_REPO must name the board to read")
	}
	c := connector.NewCtx(context.Background(), "live",
		map[string]string{"base_url": base, "repo_id": repo}, nil, nil, nil, nil)

	out, err := handleBoardState(c)
	if err != nil {
		t.Fatalf("board_state: %v", err)
	}
	state := out.(map[string]any)
	if state["cards"].(int) == 0 {
		t.Fatalf("live board reports zero cards: %v", state)
	}
	groups := state["groups"].([]map[string]any)
	t.Logf("live board %s: %d cards, %d groups, first group %v", repo, state["cards"], len(groups), groups[0])

	found, err := handleFindCards(connector.NewCtx(context.Background(), "live",
		map[string]string{"base_url": base, "repo_id": repo},
		map[string]string{"limit": "3"}, nil, nil, nil))
	if err != nil {
		t.Fatalf("find_cards: %v", err)
	}
	if got := found.(map[string]any)["returned"]; got.(int) == 0 {
		t.Fatalf("find_cards returned nothing on a non-empty board")
	}
}

func TestBoardStateCountsEveryGroupRegardlessOfOrder(t *testing.T) {
	p1, p2 := "g1", "g2"
	board := &fakeBoard{cards: []card{
		{ID: "c1", Title: "kartu 1", Status: "pending", ParentID: &p1},
		{ID: "g1", Title: "Grup satu", IsGroup: true, Status: "pending"},
		{ID: "c2", Title: "kartu 2", Status: "pending", ParentID: &p2},
		{ID: "g2", Title: "Grup dua", IsGroup: true, Status: "pending"},
	}}
	srv := httptest.NewServer(board.handler(t))
	defer srv.Close()

	out, err := handleBoardState(newCtx(t, srv, nil))
	if err != nil {
		t.Fatalf("board_state: %v", err)
	}
	groups := out.(map[string]any)["groups"].([]map[string]any)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (children listed before groups must not hide a group)", len(groups))
	}
	for _, g := range groups {
		if g["cards"].(int) != 1 {
			t.Errorf("group %v cards = %v, want 1", g["title"], g["cards"])
		}
	}
}
