package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSafeNameStripsPathCharacters(t *testing.T) {
	if got := safeName("../../etc/passwd"); got == "" || got == "../../etc/passwd" {
		t.Fatalf("safeName must not keep path separators, got %q", got)
	}
	if got := safeName(""); got != "reel" {
		t.Fatalf("empty name = %q, want reel", got)
	}
}

func TestFirstLineAndTail(t *testing.T) {
	if got := firstLine("boom\nstack"); got != "boom" {
		t.Errorf("firstLine = %q", got)
	}
	if got := tail("0123456789", 4); got != "6789" {
		t.Errorf("tail = %q", got)
	}
}

func TestOrAuto(t *testing.T) {
	if got := orAuto("  "); got != "auto" {
		t.Errorf("blank language = %q, want auto", got)
	}
	if got := orAuto("id"); got != "id" {
		t.Errorf("language = %q, want id", got)
	}
}

func TestFetchCardsAndMarkRoundTrip(t *testing.T) {
	var patched, doc string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/boards/demo/cards/a":
			_, _ = w.Write([]byte(`{"id":"a","metadata":{"instagramShortcode":"XYZ","analysisStatus":"pending"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/boards/demo/cards":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"a","title":"A","metadata":{"analysisStatus":"pending","sourceUrl":"https://x/reel/1/"}}]`))
		case r.Method == http.MethodPatch:
			if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			patched = r.URL.Path
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPut:
			if r.Header.Get("Origin") == "" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			doc = r.URL.Path
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cards, err := fetchCards(context.Background(), srv.URL, "demo")
	if err != nil {
		t.Fatalf("fetchCards: %v", err)
	}
	if len(cards) != 1 || cards[0].ID != "a" {
		t.Fatalf("cards = %+v", cards)
	}
	if err := putDoc(context.Background(), srv.URL, "demo", "a", "# doc"); err != nil {
		t.Fatalf("putDoc: %v", err)
	}
	if doc != "/api/boards/demo/cards/a/docs/spec" {
		t.Errorf("doc path = %q", doc)
	}
	mark(context.Background(), srv.URL, "demo", "a", "done")
	if patched != "/api/boards/demo/cards/a" {
		t.Errorf("patch path = %q", patched)
	}
}
