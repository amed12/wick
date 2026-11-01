package plan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yogasw/wick/internal/agents/config"
	"github.com/yogasw/wick/internal/agents/project"
	"github.com/yogasw/wick/internal/agents/ticket"
)

// Limits on one breakdown. A planning document that produces more than
// this is not a plan, it is a backlog dump — and an agent that has to read
// it will not.
const (
	MaxFeatures    = 40
	MaxTasksPerFtr = 40
)

// TaskSpec is one task under a feature.
type TaskSpec struct {
	Title      string   `json:"title"`
	Body       string   `json:"body,omitempty"`
	Acceptance string   `json:"acceptance,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
}

// FeatureSpec is one feature and the tasks it breaks into.
type FeatureSpec struct {
	Title      string     `json:"title"`
	Body       string     `json:"body,omitempty"`
	Acceptance string     `json:"acceptance,omitempty"`
	DependsOn  []string   `json:"depends_on,omitempty"`
	Tasks      []TaskSpec `json:"tasks,omitempty"`
}

// FeatureNode is a feature with the tasks it owns and its progress.
type FeatureNode struct {
	Feature ticket.Ticket   `json:"feature"`
	Tasks   []ticket.Ticket `json:"tasks"`
	Done    int             `json:"done"`
	Total   int             `json:"total"`
}

// Roadmap is the planning view of one plan: its document plus the tickets
// it produced, grouped feature → tasks.
type Roadmap struct {
	Plan     Plan          `json:"plan"`
	Features []FeatureNode `json:"features"`
}

// ValidateSpecs rejects a breakdown before anything is written, so a bad
// payload cannot leave half a plan on disk.
func ValidateSpecs(specs []FeatureSpec) error {
	if len(specs) == 0 {
		return fmt.Errorf("a plan needs at least one feature")
	}
	if len(specs) > MaxFeatures {
		return fmt.Errorf("too many features (max %d)", MaxFeatures)
	}
	for i, f := range specs {
		if strings.TrimSpace(f.Title) == "" {
			return fmt.Errorf("feature %d: title is required", i+1)
		}
		if len(f.Tasks) > MaxTasksPerFtr {
			return fmt.Errorf("feature %q: too many tasks (max %d)", f.Title, MaxTasksPerFtr)
		}
		for j, t := range f.Tasks {
			if strings.TrimSpace(t.Title) == "" {
				return fmt.Errorf("feature %q: task %d: title is required", f.Title, j+1)
			}
		}
	}
	return nil
}

// Apply creates the feature and task tickets for a breakdown and records
// the feature ids on the plan. It validates first; if a write fails midway
// the already-created tickets are removed, so the call is all-or-nothing.
func Apply(layout config.Layout, p *Plan, specs []FeatureSpec, actor ticket.Actor) ([]FeatureNode, error) {
	if err := ValidateSpecs(specs); err != nil {
		return nil, err
	}
	proj, err := project.Load(layout, p.ProjectID)
	if err != nil {
		return nil, err
	}
	status := proj.Meta.Ticket.FirstStatus()

	var created []ticket.Ticket
	rollback := func() {
		for _, tk := range created {
			_ = ticket.Delete(layout, tk.ProjectID, tk.ID)
		}
	}

	for _, f := range specs {
		ftr, err := ticket.Create(layout, ticket.CreateOptions{
			ProjectID:  p.ProjectID,
			Title:      f.Title,
			Body:       f.Body,
			Acceptance: f.Acceptance,
			Status:     status,
			Kind:       ticket.KindFeature,
			PlanID:     p.ID,
			DependsOn:  f.DependsOn,
			Actor:      actor,
		})
		if err != nil {
			rollback()
			return nil, fmt.Errorf("create feature %q: %w", f.Title, err)
		}
		created = append(created, ftr)
		for _, t := range f.Tasks {
			task, err := ticket.Create(layout, ticket.CreateOptions{
				ProjectID:  p.ProjectID,
				Title:      t.Title,
				Body:       t.Body,
				Acceptance: t.Acceptance,
				Status:     status,
				Kind:       ticket.KindTask,
				PlanID:     p.ID,
				ParentID:   ftr.ID,
				DependsOn:  t.DependsOn,
				Actor:      actor,
			})
			if err != nil {
				rollback()
				return nil, fmt.Errorf("create task %q: %w", t.Title, err)
			}
			created = append(created, task)
		}
		p.Features = append(p.Features, ftr.ID)
	}
	if err := Save(layout, *p); err != nil {
		rollback()
		return nil, err
	}
	return Build(layout, *p)
}

// AddTask appends one task to an existing feature of this plan. It exists
// because the common request after a breakdown is "add one more thing to
// that feature" — re-applying a whole spec would duplicate every sibling.
func AddTask(layout config.Layout, p Plan, featureID string, spec TaskSpec, actor ticket.Actor) (ticket.Ticket, error) {
	featureID = strings.TrimSpace(featureID)
	if featureID == "" {
		return ticket.Ticket{}, fmt.Errorf("feature_id is required")
	}
	if strings.TrimSpace(spec.Title) == "" {
		return ticket.Ticket{}, fmt.Errorf("task title is required")
	}
	feature, err := ticket.Load(layout, p.ProjectID, featureID)
	if err != nil {
		return ticket.Ticket{}, fmt.Errorf("feature %q not found", featureID)
	}
	if feature.KindOf() != ticket.KindFeature || feature.PlanID != p.ID {
		return ticket.Ticket{}, fmt.Errorf("%q is not a feature of this plan", featureID)
	}
	proj, err := project.Load(layout, p.ProjectID)
	if err != nil {
		return ticket.Ticket{}, err
	}
	return ticket.Create(layout, ticket.CreateOptions{
		ProjectID:  p.ProjectID,
		Title:      spec.Title,
		Body:       spec.Body,
		Acceptance: spec.Acceptance,
		Status:     proj.Meta.Ticket.FirstStatus(),
		Kind:       ticket.KindTask,
		PlanID:     p.ID,
		ParentID:   feature.ID,
		DependsOn:  spec.DependsOn,
		Actor:      actor,
	})
}

// Build assembles the roadmap for a plan from the ticket store. It never
// mutates anything: the tickets are the source of truth for progress.
func Build(layout config.Layout, p Plan) ([]FeatureNode, error) {
	proj, err := project.Load(layout, p.ProjectID)
	if err != nil {
		return nil, err
	}
	terminal := proj.Meta.Ticket.TerminalStatus()
	all, err := ticket.List(layout, p.ProjectID)
	if err != nil {
		return nil, err
	}
	byParent := map[string][]ticket.Ticket{}
	features := map[string]ticket.Ticket{}
	for _, tk := range all {
		if tk.PlanID != p.ID {
			continue
		}
		if tk.KindOf() == ticket.KindFeature {
			features[tk.ID] = tk
		}
		if tk.ParentID != "" {
			byParent[tk.ParentID] = append(byParent[tk.ParentID], tk)
		}
	}
	// A feature referenced by the plan is shown even when its ticket is
	// missing, so a deleted card does not silently shrink the map.
	order := append([]string(nil), p.Features...)
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	var extra []string
	for id := range features {
		if !seen[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	order = append(order, extra...)

	out := make([]FeatureNode, 0, len(order))
	for _, id := range order {
		ftr, ok := features[id]
		if !ok {
			continue
		}
		tasks := byParent[id]
		sort.Slice(tasks, func(i, j int) bool {
			if tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
				return tasks[i].ID < tasks[j].ID
			}
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		})
		n := FeatureNode{Feature: ftr, Tasks: tasks}
		for _, t := range tasks {
			n.Total++
			if t.Status == terminal {
				n.Done++
			}
		}
		out = append(out, n)
	}
	return out, nil
}

// Unplanned returns tickets in the project that no plan owns. The planning
// workspace offers them so existing work can be pulled into a plan instead
// of being invisible to it.
func Unplanned(layout config.Layout, projectID string) ([]ticket.Ticket, error) {
	all, err := ticket.List(layout, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]ticket.Ticket, 0, len(all))
	for _, tk := range all {
		if tk.PlanID == "" {
			out = append(out, tk)
		}
	}
	return out, nil
}
