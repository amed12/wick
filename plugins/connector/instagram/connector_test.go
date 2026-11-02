package main

import (
	"strings"
	"testing"
)

func TestParseCollectionRef(t *testing.T) {
	cases := []struct {
		in        string
		slug, id  string
		shouldErr bool
	}{
		{in: "https://www.instagram.com/fathullahachmad/saved/ai/18326269657221437/", slug: "ai", id: "18326269657221437"},
		{in: "https://www.instagram.com/fathullahachmad/saved/ai/18326269657221437", slug: "ai", id: "18326269657221437"},
		{in: "ai/18326269657221437", slug: "ai", id: "18326269657221437"},
		{in: "   ", shouldErr: true},
		{in: "https://www.instagram.com/p/DfL5rjiGOKd/", shouldErr: true},
		{in: "18326269657221437", shouldErr: true},
	}
	for _, tc := range cases {
		slug, id, err := parseCollectionRef(tc.in)
		if tc.shouldErr {
			if err == nil {
				t.Errorf("%q: want error, got slug=%q id=%q", tc.in, slug, id)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", tc.in, err)
			continue
		}
		if slug != tc.slug || id != tc.id {
			t.Errorf("%q: got slug=%q id=%q, want %q/%q", tc.in, slug, id, tc.slug, tc.id)
		}
	}
}

func TestExtractTemplatePicksTheCollectionQuery(t *testing.T) {
	requests := []any{
		map[string]any{"method": "GET", "url": "https://www.instagram.com/api/graphql", "body": ""},
		map[string]any{"method": "POST", "url": "https://www.instagram.com/api/graphql",
			"body": "doc_id=111&variables=%7B%22collection_id%22%3A%22999%22%7D&fb_dtsg=x"},
		map[string]any{"method": "POST", "url": "https://www.instagram.com/api/graphql",
			"body": "av=1&doc_id=29009332198686334&variables=%7B%22collection_id%22%3A%2218326269657221437%22%7D&fb_dtsg=abc&lsd=def"},
	}
	got, err := extractTemplate(requests, "18326269657221437")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(got, "doc_id=29009332198686334") {
		t.Errorf("picked the wrong request: %q", got)
	}
}

func TestExtractTemplatePrefersConnectionQuery(t *testing.T) {
	requests := []any{
		map[string]any{"method": "POST", "body": "doc_id=1&variables=%7B%22collection_id%22%3A%227%22%7D&friendly=PolarisSavedCollectionPageWWWQuery"},
		map[string]any{"method": "POST", "body": "doc_id=2&variables=%7B%22collection_id%22%3A%227%22%7D&friendly=PolarisSavedCollectionPageWWWQuery_connection"},
	}
	got, err := extractTemplate(requests, "7")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !strings.Contains(got, "doc_id=2") {
		t.Errorf("want the connection query (doc_id=2), got %q", got)
	}
}

func TestExtractTemplateFailsLoudly(t *testing.T) {
	if _, err := extractTemplate([]any{map[string]any{"method": "POST", "body": "hello=1"}}, "42"); err == nil {
		t.Fatal("want an error when no collection query was recorded")
	}
}

func TestHarvestScriptCarriesInputs(t *testing.T) {
	script := harvestScript("doc_id=1&variables=%7B%7D", "18326269657221437", "CURSOR==", 4, 700)
	for _, want := range []string{"18326269657221437", "CURSOR==", "const maxPages = 4", "const delay = 700", "fetch('/api/graphql'"} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q", want)
		}
	}
	if strings.Contains(script, "after = \"\"") {
		t.Error("empty cursor must render as null, not an empty string")
	}
}

func TestRunFailureIgnoresOutputlessSteps(t *testing.T) {
	run := map[string]any{"steps": []any{
		map[string]any{"action": "eval", "ok": true, "output": map[string]any{"navigating": true}},
		map[string]any{"action": "wait", "ok": true},
	}}
	if err := runFailure(run); err != nil {
		t.Fatalf("a wait step with no output is not a failure: %v", err)
	}
	bad := map[string]any{"steps": []any{
		map[string]any{"action": "scroll", "ok": false, "error": "tab closed"},
	}}
	if err := runFailure(bad); err == nil || !strings.Contains(err.Error(), "tab closed") {
		t.Fatalf("want the failed step surfaced, got %v", err)
	}
}

func TestLastStepOutputSurfacesFailures(t *testing.T) {
	run := map[string]any{"steps": []any{
		map[string]any{"action": "eval", "ok": false, "error": "page crashed"},
	}}
	if _, err := lastStepOutput(run); err == nil || !strings.Contains(err.Error(), "page crashed") {
		t.Fatalf("want the step error surfaced, got %v", err)
	}

	ok := map[string]any{"steps": []any{
		map[string]any{"action": "eval", "ok": true, "output": map[string]any{"items": []any{}, "next_cursor": nil}},
	}}
	out, err := lastStepOutput(ok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, found := out["items"]; !found {
		t.Errorf("output lost its fields: %v", out)
	}
}

func TestSanitizeKeepsIdSafeForFilenames(t *testing.T) {
	if got := sanitize("../../etc/passwd"); strings.ContainsAny(got, "./") {
		t.Errorf("sanitize left path characters: %q", got)
	}
	if got := sanitize(""); got != "unknown" {
		t.Errorf("empty id = %q, want unknown", got)
	}
}
