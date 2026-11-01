package plan

import (
	"errors"
	"strings"
	"testing"

	"github.com/yogasw/wick/internal/agents/config"
	"github.com/yogasw/wick/internal/agents/project"
	"github.com/yogasw/wick/internal/agents/ticket"
	wfprovider "github.com/yogasw/wick/internal/agents/workflow/provider"
)

func testLayout(t *testing.T) config.Layout {
	t.Helper()
	l := config.NewLayout(t.TempDir())
	if _, err := project.Create(l, project.CreateOptions{ID: "p1", Name: "Demo"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return l
}

func TestCreateNumbersPlansAndVersions(t *testing.T) {
	l := testLayout(t)
	p1, err := Create(l, CreateOptions{ProjectID: "p1", Title: "Plan satu", Idea: "aplikasi kasir"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if p1.Number != 1 || p1.Status != StatusDraft || p1.Version != 0 {
		t.Fatalf("unexpected plan: %+v", p1)
	}
	p2, err := Create(l, CreateOptions{ProjectID: "p1", Title: "Plan dua"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if p2.Number != 2 {
		t.Fatalf("second plan number = %d, want 2", p2.Number)
	}

	if err := SetPRD(l, &p1, "# PRD\n\nisi", "draft awal"); err != nil {
		t.Fatalf("set prd: %v", err)
	}
	if p1.Version != 1 || len(p1.Versions) != 1 {
		t.Fatalf("version not recorded: %+v", p1)
	}
	// An unchanged document must not add a version.
	if err := SetPRD(l, &p1, "# PRD\n\nisi", "same"); err != nil {
		t.Fatalf("set prd same: %v", err)
	}
	if p1.Version != 1 || len(p1.Versions) != 1 {
		t.Fatalf("identical PRD created a version: %+v", p1)
	}
	if err := SetPRD(l, &p1, "# PRD v2", ""); err != nil {
		t.Fatalf("set prd v2: %v", err)
	}
	if p1.Version != 2 || len(p1.Versions) != 2 {
		t.Fatalf("second version not recorded: %+v", p1)
	}

	got, err := Load(l, "p1", p1.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.PRD != "# PRD v2" || got.Version != 2 || got.Versions[1].PRD != "# PRD v2" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestListIsPhaseOrdered(t *testing.T) {
	l := testLayout(t)
	var ids []string
	for _, title := range []string{"tiga", "satu", "dua"} {
		p, err := Create(l, CreateOptions{ProjectID: "p1", Title: title})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		ids = append(ids, p.ID)
	}
	list, err := List(l, "p1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	for i, p := range list {
		if p.Number != i+1 {
			t.Fatalf("plan %d number = %d", i, p.Number)
		}
	}
}

func TestApplyCreatesFeaturesAndTasks(t *testing.T) {
	l := testLayout(t)
	p, err := Create(l, CreateOptions{ProjectID: "p1", Title: "MVP"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	specs := []FeatureSpec{
		{
			Title:      "Autentikasi",
			Body:       "daftar, login, reset",
			Acceptance: "- [ ] bisa login",
			Tasks: []TaskSpec{
				{Title: "Halaman login", Acceptance: "- [ ] form muncul"},
				{Title: "Reset password"},
			},
		},
		{
			Title: "Dashboard",
			Tasks: []TaskSpec{{Title: "Kartu total"}},
		},
	}
	nodes, err := Apply(l, &p, specs, ticket.Actor{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("features = %d, want 2", len(nodes))
	}
	if len(nodes[0].Tasks) != 2 || nodes[0].Total != 2 || nodes[0].Done != 0 {
		t.Fatalf("task node: %+v", nodes[0])
	}
	if nodes[0].Feature.KindOf() != ticket.KindFeature {
		t.Fatalf("feature kind = %q", nodes[0].Feature.KindOf())
	}
	if nodes[0].Tasks[0].ParentID != nodes[0].Feature.ID {
		t.Fatalf("task parent = %q, want %q", nodes[0].Tasks[0].ParentID, nodes[0].Feature.ID)
	}
	if len(p.Features) != 2 {
		t.Fatalf("plan features = %v", p.Features)
	}

	// Completing every task moves progress, read from the tickets.
	proj, _ := project.Load(l, "p1")
	terminal := proj.Meta.Ticket.TerminalStatus()
	for _, task := range nodes[0].Tasks {
		tk, _ := ticket.Load(l, "p1", task.ID)
		tk.Status = terminal
		if err := ticket.Save(l, tk); err != nil {
			t.Fatalf("save task: %v", err)
		}
	}
	nodes, err = Build(l, p)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if nodes[0].Done != 2 || nodes[1].Done != 0 {
		t.Fatalf("progress = %d/%d, %d/%d", nodes[0].Done, nodes[0].Total, nodes[1].Done, nodes[1].Total)
	}
}

func TestAddTaskAppendsToFeature(t *testing.T) {
	l := testLayout(t)
	p, _ := Create(l, CreateOptions{ProjectID: "p1", Title: "MVP"})
	nodes, err := Apply(l, &p, []FeatureSpec{{Title: "Fitur", Tasks: []TaskSpec{{Title: "Tugas satu"}}}}, ticket.Actor{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	featureID := nodes[0].Feature.ID
	tk, err := AddTask(l, p, featureID, TaskSpec{Title: "Tugas dua", Acceptance: "- [ ] ok"}, ticket.Actor{})
	if err != nil {
		t.Fatalf("add task: %v", err)
	}
	if tk.ParentID != featureID || tk.PlanID != p.ID || tk.KindOf() != ticket.KindTask {
		t.Fatalf("task fields: %+v", tk)
	}
	nodes, _ = Build(l, p)
	if len(nodes[0].Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(nodes[0].Tasks))
	}
	// A ticket from another plan is not a valid feature here.
	other, _ := Create(l, CreateOptions{ProjectID: "p1", Title: "Other"})
	if _, err := AddTask(l, other, featureID, TaskSpec{Title: "x"}, ticket.Actor{}); err == nil {
		t.Fatal("expected cross-plan feature to be rejected")
	}
}

func TestApplyRollsBackOnBadSpec(t *testing.T) {
	l := testLayout(t)
	p, _ := Create(l, CreateOptions{ProjectID: "p1", Title: "MVP"})
	if _, err := Apply(l, &p, []FeatureSpec{{Title: "ok"}, {Title: "  "}}, ticket.Actor{}); err == nil {
		t.Fatal("expected validation error")
	}
	all, _ := ticket.List(l, "p1")
	if len(all) != 0 {
		t.Fatalf("validation left %d tickets behind", len(all))
	}
}

func TestContextAddRemove(t *testing.T) {
	l := testLayout(t)
	p, _ := Create(l, CreateOptions{ProjectID: "p1", Title: "MVP"})
	if err := AddContext(l, &p, ContextRef{Kind: "url", Ref: "https://example.com/spec", Label: "spec"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(p.Context) != 1 {
		t.Fatalf("context = %+v", p.Context)
	}
	// Adding the same ref twice must not duplicate it.
	if err := AddContext(l, &p, ContextRef{Kind: "url", Ref: "https://example.com/spec"}); err != nil {
		t.Fatalf("add again: %v", err)
	}
	if len(p.Context) != 1 {
		t.Fatalf("duplicate context: %+v", p.Context)
	}
	removed, err := RemoveContext(l, &p, "url", "https://example.com/spec")
	if err != nil || !removed {
		t.Fatalf("remove: %v %v", removed, err)
	}
	if len(p.Context) != 0 {
		t.Fatalf("context not removed: %+v", p.Context)
	}
}

func TestDeleteKeepsTickets(t *testing.T) {
	l := testLayout(t)
	p, _ := Create(l, CreateOptions{ProjectID: "p1", Title: "MVP"})
	if _, err := Apply(l, &p, []FeatureSpec{{Title: "Fitur", Tasks: []TaskSpec{{Title: "Tugas"}}}}, ticket.Actor{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := Delete(l, "p1", p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := Load(l, "p1", p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load after delete = %v", err)
	}
	all, _ := ticket.List(l, "p1")
	if len(all) != 2 {
		t.Fatalf("delete removed tickets: %d", len(all))
	}
}

func wfproviderResult(prd string) wfprovider.StructuredResult {
	return wfprovider.StructuredResult{
		OK: true,
		Parsed: map[string]any{
			"title":   "Aplikasi",
			"summary": "ringkas",
			"prd":     prd,
			"features": []any{
				map[string]any{
					"title": "Fitur",
					"tasks": []any{map[string]any{"title": "Tugas"}},
				},
			},
		},
	}
}

func TestDraftFromResultRejectsEmpty(t *testing.T) {
	if _, err := DraftFromResult(wfproviderResult("")); err == nil {
		t.Fatal("empty PRD should fail")
	}
	d, err := DraftFromResult(wfproviderResult("# PRD\n\nisi"))
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if !strings.Contains(d.PRD, "PRD") || len(d.Features) != 1 {
		t.Fatalf("draft = %+v", d)
	}
}
