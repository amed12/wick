// Package plan stores a project's planning documents: the PRD for one
// phase, the features and tasks it breaks down into, and the context the
// plan was written from.
//
// A plan does NOT duplicate the work. Features and tasks are ordinary
// tickets (see internal/agents/ticket) — a feature is a ticket with
// Kind=feature, a task is a ticket pointing at its feature through
// ParentID. That keeps the board, notes, sessions, webhooks and MCP tools
// working unchanged, and makes progress a query over tickets rather than a
// second source of truth that can drift.
//
// One project has many plans over its life (Plan 1, Plan 2, ...). Each is
// a phase: the next plan starts from what the previous one shipped.
package plan

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/yogasw/wick/internal/agents/config"
	"github.com/yogasw/wick/internal/agents/project"
	"github.com/yogasw/wick/internal/agents/storage"
)

// Plan statuses.
const (
	StatusDraft  = "draft"
	StatusActive = "active"
	StatusDone   = "done"
)

// Limits. The PRD is a document a person edits in a browser; anything past
// this is a paste accident, not a plan.
const (
	MaxPRDBytes     = 256 * 1024
	MaxTitleRunes   = 200
	MaxVersions     = 50
	MaxContextItems = 50
)

// ErrNotFound is returned when a plan id does not resolve within a project.
var ErrNotFound = errors.New("plan not found")

// Version is one saved snapshot of the PRD. The current text lives in
// Plan.PRD; Versions is history, newest last.
type Version struct {
	Number    int       `json:"number"`
	PRD       string    `json:"prd"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ContextRef is one piece of source material the plan was written from: a
// file in the project, a note, a URL, or pasted text.
type ContextRef struct {
	Kind    string    `json:"kind"` // file | note | url | text
	Ref     string    `json:"ref"`  // path, note id, URL, or the text itself
	Label   string    `json:"label,omitempty"`
	AddedAt time.Time `json:"added_at"`
}

// Plan is one planning phase of a project.
type Plan struct {
	// ID is a short, human-quotable code ("P-7QK2") because it appears in
	// URLs and in agent prompts.
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	// Number is the phase number within the project (1, 2, 3...). It is
	// what "Plan 3" means in conversation; IDs are random so a re-created
	// plan never reuses a code another record points at.
	Number int    `json:"number"`
	Title  string `json:"title"`
	Idea   string `json:"idea,omitempty"`
	PRD    string `json:"prd,omitempty"`
	Status string `json:"status"`
	// Version is the current PRD version number; 0 means "never saved a
	// version yet". SetPRD bumps it.
	Version  int          `json:"version"`
	Versions []Version    `json:"versions,omitempty"`
	Features []string     `json:"features,omitempty"` // feature ticket IDs, in creation order
	Context  []ContextRef `json:"context,omitempty"`
	// Summary is a one-line "what this phase is about", written by the
	// generator or the planner. It survives PRD rewrites.
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateOptions describes a new plan.
type CreateOptions struct {
	ProjectID string
	Title     string
	Idea      string
	PRD       string
	Summary   string
	Status    string
}

// Create materialises a plan under its project. The phase number is the
// next free one, so a project's plans read 1, 2, 3 in the order they were
// started.
func Create(layout config.Layout, opt CreateOptions) (Plan, error) {
	projectID := strings.TrimSpace(opt.ProjectID)
	if projectID == "" {
		return Plan{}, fmt.Errorf("project id is required")
	}
	if !project.Exists(layout, projectID) {
		return Plan{}, fmt.Errorf("project %q not found", projectID)
	}
	title := strings.TrimSpace(opt.Title)
	if title == "" {
		return Plan{}, fmt.Errorf("plan title is required")
	}
	if len([]rune(title)) > MaxTitleRunes {
		return Plan{}, fmt.Errorf("plan title is too long (max %d characters)", MaxTitleRunes)
	}
	if len(opt.PRD) > MaxPRDBytes {
		return Plan{}, fmt.Errorf("PRD is too large (max %d KB)", MaxPRDBytes/1024)
	}
	status := strings.TrimSpace(opt.Status)
	if status == "" {
		status = StatusDraft
	}
	if !validStatus(status) {
		return Plan{}, fmt.Errorf("invalid status %q (want draft, active, or done)", status)
	}
	existing, err := List(layout, projectID)
	if err != nil {
		return Plan{}, err
	}
	number := 1
	for _, p := range existing {
		if p.Number >= number {
			number = p.Number + 1
		}
	}
	now := time.Now().UTC()
	for attempt := 0; attempt < 8; attempt++ {
		id, err := newID()
		if err != nil {
			return Plan{}, err
		}
		if storage.PathExists(layout.PlanFile(projectID, id)) {
			continue
		}
		p := Plan{
			ID:        id,
			ProjectID: projectID,
			Number:    number,
			Title:     title,
			Idea:      strings.TrimSpace(opt.Idea),
			PRD:       strings.TrimSpace(opt.PRD),
			Status:    status,
			Summary:   strings.TrimSpace(opt.Summary),
			CreatedAt: now,
			UpdatedAt: now,
		}
		if p.PRD != "" {
			p.Version = 1
			p.Versions = []Version{{Number: 1, PRD: p.PRD, Note: "initial", CreatedAt: now}}
		}
		if err := os.MkdirAll(layout.PlanDir(projectID, id), 0o755); err != nil {
			return Plan{}, err
		}
		if err := storage.WriteJSON(layout.PlanFile(projectID, id), &p); err != nil {
			return Plan{}, err
		}
		return p, nil
	}
	return Plan{}, fmt.Errorf("could not allocate a plan id — try again")
}

// Load reads one plan. A missing or unreadable plan reports ErrNotFound so
// an HTTP caller cannot probe whether a plan exists in a project it may not
// read.
func Load(layout config.Layout, projectID, planID string) (Plan, error) {
	if !project.ValidID(projectID) || !project.ValidID(planID) {
		return Plan{}, ErrNotFound
	}
	var p Plan
	if err := storage.ReadJSON(layout.PlanFile(projectID, planID), &p); err != nil {
		if os.IsNotExist(err) {
			return Plan{}, ErrNotFound
		}
		return Plan{}, err
	}
	p.normalise()
	return p, nil
}

// Find locates a plan by id across the projects the caller names. The API
// addresses plans by their own id, so it needs the owning project; callers
// that already know it should use Load.
func Find(layout config.Layout, projectIDs []string, planID string) (Plan, error) {
	for _, pid := range projectIDs {
		p, err := Load(layout, pid, planID)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return Plan{}, err
		}
	}
	return Plan{}, ErrNotFound
}

// Save writes the plan and bumps UpdatedAt.
func Save(layout config.Layout, p Plan) error {
	if !project.ValidID(p.ProjectID) || !project.ValidID(p.ID) {
		return fmt.Errorf("invalid plan identity")
	}
	if strings.TrimSpace(p.Title) == "" {
		return fmt.Errorf("plan title is required")
	}
	if !validStatus(p.Status) {
		return fmt.Errorf("invalid status %q", p.Status)
	}
	if len(p.PRD) > MaxPRDBytes {
		return fmt.Errorf("PRD is too large (max %d KB)", MaxPRDBytes/1024)
	}
	p.normalise()
	p.UpdatedAt = time.Now().UTC()
	return storage.WriteJSON(layout.PlanFile(p.ProjectID, p.ID), &p)
}

// SetPRD replaces the document and records a version. An unchanged
// document is a no-op so an autosave on every keystroke does not fill the
// history with identical snapshots.
func SetPRD(layout config.Layout, p *Plan, prd, note string) error {
	prd = strings.TrimSpace(prd)
	if len(prd) > MaxPRDBytes {
		return fmt.Errorf("PRD is too large (max %d KB)", MaxPRDBytes/1024)
	}
	if prd == strings.TrimSpace(p.PRD) {
		return nil
	}
	now := time.Now().UTC()
	p.PRD = prd
	p.Version++
	p.Versions = append(p.Versions, Version{
		Number:    p.Version,
		PRD:       prd,
		Note:      strings.TrimSpace(note),
		CreatedAt: now,
	})
	if len(p.Versions) > MaxVersions {
		p.Versions = p.Versions[len(p.Versions)-MaxVersions:]
	}
	return Save(layout, *p)
}

// AddContext appends one source item, replacing an identical kind+ref.
func AddContext(layout config.Layout, p *Plan, ref ContextRef) error {
	ref.Kind = strings.TrimSpace(strings.ToLower(ref.Kind))
	ref.Ref = strings.TrimSpace(ref.Ref)
	if ref.Kind == "" || ref.Ref == "" {
		return fmt.Errorf("context kind and ref are required")
	}
	ref.AddedAt = time.Now().UTC()
	out := make([]ContextRef, 0, len(p.Context)+1)
	for _, c := range p.Context {
		if c.Kind == ref.Kind && c.Ref == ref.Ref {
			continue
		}
		out = append(out, c)
	}
	out = append(out, ref)
	if len(out) > MaxContextItems {
		out = out[len(out)-MaxContextItems:]
	}
	p.Context = out
	return Save(layout, *p)
}

// RemoveContext drops one item by kind+ref. Returns whether anything was
// removed.
func RemoveContext(layout config.Layout, p *Plan, kind, ref string) (bool, error) {
	out := make([]ContextRef, 0, len(p.Context))
	removed := false
	for _, c := range p.Context {
		if c.Kind == kind && c.Ref == ref {
			removed = true
			continue
		}
		out = append(out, c)
	}
	if !removed {
		return false, nil
	}
	p.Context = out
	return true, Save(layout, *p)
}

// List returns a project's plans in phase order. A project with no plans
// directory is empty, not an error.
func List(layout config.Layout, projectID string) ([]Plan, error) {
	if !project.ValidID(projectID) {
		return nil, nil
	}
	ids, err := storage.ScanDirNames(layout.PlansDir(projectID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Plan, 0, len(ids))
	for _, id := range ids {
		p, lerr := Load(layout, projectID, id)
		if lerr != nil {
			continue // one unreadable plan must not hide the rest
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Number == out[j].Number {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Number < out[j].Number
	})
	return out, nil
}

// Delete removes a plan and its directory. The tickets it produced stay:
// they are work, and deleting the document must not erase the board.
func Delete(layout config.Layout, projectID, planID string) error {
	if !project.ValidID(projectID) || !project.ValidID(planID) {
		return ErrNotFound
	}
	if !storage.PathExists(layout.PlanFile(projectID, planID)) {
		return ErrNotFound
	}
	return os.RemoveAll(layout.PlanDir(projectID, planID))
}

func (p *Plan) normalise() {
	p.Title = strings.TrimSpace(p.Title)
	p.Idea = strings.TrimSpace(p.Idea)
	p.Summary = strings.TrimSpace(p.Summary)
	if p.Status == "" {
		p.Status = StatusDraft
	}
	if p.Version < 0 {
		p.Version = 0
	}
}

func validStatus(s string) bool {
	switch s {
	case StatusDraft, StatusActive, StatusDone:
		return true
	}
	return false
}

// idAlphabet excludes look-alikes so a plan code survives being read aloud.
const idAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := []byte("P-")
	for _, c := range b {
		out = append(out, idAlphabet[int(c)%len(idAlphabet)])
	}
	return string(out), nil
}
