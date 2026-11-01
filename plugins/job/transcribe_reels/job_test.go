package main

import "testing"

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
