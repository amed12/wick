// Package plans exposes a project's planning documents (PRD + feature/
// task breakdown) to agents over MCP.
//
// Features and tasks are ordinary tickets; this connector only owns the
// document and the breakdown that creates them. An agent reads the plan,
// picks a task from the roadmap, and updates the ticket it is working on
// through the tickets connector — no copy-pasted prompts.
package plans

import (
	"encoding/json"
	"fmt"
	"strings"

	agentconfig "github.com/yogasw/wick/internal/agents/config"
	"github.com/yogasw/wick/internal/agents/plan"
	"github.com/yogasw/wick/internal/agents/session"
	"github.com/yogasw/wick/internal/agents/team"
	"github.com/yogasw/wick/internal/agents/ticket"
	"github.com/yogasw/wick/internal/tags"
	"github.com/yogasw/wick/pkg/connector"
	"github.com/yogasw/wick/pkg/tool"
	"github.com/yogasw/wick/pkg/wickdocs"
)

// Key is the connector's stable identifier.
const Key = "plans"

// Configs is empty: plans live on wick's own disk.
type Configs struct{}

// Meta returns the static metadata block.
func Meta() connector.Meta {
	return connector.Meta{
		Key:  Key,
		Name: "Plans",
		Description: "Read and write a project's planning documents: the PRD for one phase, " +
			"the features it breaks into, their tasks, and the progress an agent reports back.",
		Icon:  "🗺️",
		Fixed: true,
	}
}

// Module returns the wired connector.
func Module(layout agentconfig.Layout) connector.Module {
	m := Meta()
	m.DefaultTags = []tool.DefaultTag{tags.Connector, tags.Platform}
	return connector.Module{Meta: m, Operations: Operations(layout)}
}

// Operations builds the op list, capturing layout so handlers reach the
// on-disk store.
func Operations(layout agentconfig.Layout) []connector.Category {
	h := &handlers{layout: layout}
	return []connector.Category{connector.Cat("Plans",
		"A plan is one phase of a project: a PRD, the features it decomposes into, and the tasks under each feature. Features and tasks are tickets.",
		connector.Op("plan_list", "List Plans",
			"List a project's plans in phase order (Plan 1, Plan 2, ...), each with feature counts and task progress. "+
				"Omit project_id to use the project of the calling session.",
			listInput{}, h.list, wickdocs.Docs{}),
		connector.Op("plan_get", "Get Plan",
			"Read one plan: the full PRD markdown, every feature with its tasks, each task's status, acceptance criteria and dependencies. "+
				"Use this before starting work so you know what the plan requires and what is already done.",
			getInput{}, h.get, wickdocs.Docs{}),
		connector.Op("plan_create", "Create Plan",
			"Start the next plan phase in a project. Give it a title and the user's idea; write the PRD with plan_update_prd afterwards. "+
				"Returns the new plan id and its phase number.",
			createInput{}, h.create, wickdocs.Docs{}),
		connector.Op("plan_update_prd", "Update PRD",
			"Replace a plan's PRD markdown and record a version. Pass note to say what changed. "+
				"An unchanged document is a no-op. Use plan_add_features afterwards to turn the document into work.",
			updatePRDInput{}, h.updatePRD, wickdocs.Docs{}),
		connector.Op("plan_add_features", "Add Features & Tasks",
			"Turn PRD sections into work: creates one feature ticket per entry and one task ticket per task, all linked to the plan. "+
				"features is a JSON array like [{\"title\":\"Auth\",\"body\":\"...\",\"acceptance\":\"- [ ] ...\",\"tasks\":[{\"title\":\"Login page\",\"acceptance\":\"- [ ] ...\"}]}]. "+
				"Validate first: a payload that fails leaves nothing behind. "+
				"Creating the same breakdown twice creates duplicate tickets — read plan_get first.",
			addFeaturesInput{}, h.addFeatures, wickdocs.Docs{}),
		connector.Op("plan_add_task", "Add Task",
			"Add one task to an existing feature of a plan. Use this for \"add one more thing\" after a breakdown; "+
				"plan_add_features would duplicate every sibling. Returns the new ticket id.",
			addTaskInput{}, h.addTask, wickdocs.Docs{}),
		connector.Op("plan_set_status", "Set Plan Status",
			"Move a plan between draft, active and done. Marking done is how the next phase starts from a finished baseline.",
			setStatusInput{}, h.setStatus, wickdocs.Docs{}),
		connector.Op("plan_context_add", "Add Plan Context",
			"Attach source material the plan must honour: kind=file with a path relative to the project, kind=url, kind=note, or kind=text with the content itself. "+
				"Context is what makes a PRD specific instead of generic.",
			contextInput{}, h.addContext, wickdocs.Docs{}),
	)}
}

type handlers struct{ layout agentconfig.Layout }

/* ── inputs ─────────────────────────────────────────────────────────────── */

type listInput struct {
	ProjectID string `wick:"required;desc=Project id. Omit to use the calling session's project."`
}

type getInput struct {
	ProjectID string `wick:"required;desc=Project id. Omit to use the calling session's project."`
	PlanID    string `wick:"required;desc=Plan id from plan_list, e.g. P-7QK2."`
}

type createInput struct {
	ProjectID string `wick:"required;desc=Project id. Omit to use the calling session's project."`
	Title     string `wick:"required;desc=Short phase title, e.g. \"MVP kasir\"."`
	Idea      string `wick:"desc=The user's idea or goal in their own words."`
}

type updatePRDInput struct {
	ProjectID string `wick:"required;desc=Project id. Omit to use the calling session's project."`
	PlanID    string `wick:"required;desc=Plan id."`
	PRD       string `wick:"required;desc=The complete PRD markdown. This replaces the document, it does not append."`
	Note      string `wick:"desc=What changed in this version."`
}

type addFeaturesInput struct {
	ProjectID string `wick:"required;desc=Project id. Omit to use the calling session's project."`
	PlanID    string `wick:"required;desc=Plan id."`
	Features  string `wick:"required;desc=JSON array of features (see the operation description for the shape)."`
}

type addTaskInput struct {
	ProjectID  string `wick:"required;desc=Project id. Omit to use the calling session's project."`
	PlanID     string `wick:"required;desc=Plan id."`
	FeatureID  string `wick:"required;desc=Feature ticket id from plan_get, e.g. T-Y9CU."`
	Title      string `wick:"required;desc=Short description of the task."`
	Body       string `wick:"textarea;desc=Markdown detail: what to build and why."`
	Acceptance string `wick:"textarea;desc=Done-when checklist, e.g. \"- [ ] login works\"."`
}

type setStatusInput struct {
	ProjectID string `wick:"required;desc=Project id. Omit to use the calling session's project."`
	PlanID    string `wick:"required;desc=Plan id."`
	Status    string `wick:"required;dropdown=draft|active|done;desc=New status."`
}

type contextInput struct {
	ProjectID string `wick:"required;desc=Project id. Omit to use the calling session's project."`
	PlanID    string `wick:"required;desc=Plan id."`
	Kind      string `wick:"required;dropdown=file|url|note|text;desc=What kind of source this is."`
	Ref       string `wick:"required;desc=Path, URL, note id, or the text itself."`
	Label     string `wick:"desc=Short label shown next to it."`
}

/* ── ops ────────────────────────────────────────────────────────────────── */

func (h *handlers) list(c *connector.Ctx) (any, error) {
	projectID, err := h.project(c, c.Input("project_id"))
	if err != nil {
		return nil, err
	}
	plans, err := plan.List(h.layout, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		nodes, _ := plan.Build(h.layout, p)
		done, total := 0, 0
		for _, n := range nodes {
			done += n.Done
			total += n.Total
		}
		out = append(out, map[string]any{
			"id": p.ID, "number": p.Number, "title": p.Title, "status": p.Status,
			"summary": p.Summary, "version": p.Version,
			"feature_count": len(nodes), "tasks_done": done, "tasks_total": total,
		})
	}
	return map[string]any{"project_id": projectID, "plans": out}, nil
}

func (h *handlers) get(c *connector.Ctx) (any, error) {
	p, err := h.planOf(c)
	if err != nil {
		return nil, err
	}
	nodes, err := plan.Build(h.layout, p)
	if err != nil {
		return nil, err
	}
	return map[string]any{"plan": p, "features": nodes}, nil
}

func (h *handlers) create(c *connector.Ctx) (any, error) {
	projectID, err := h.project(c, c.Input("project_id"))
	if err != nil {
		return nil, err
	}
	p, err := plan.Create(h.layout, plan.CreateOptions{
		ProjectID: projectID,
		Title:     c.Input("title"),
		Idea:      c.Input("idea"),
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"plan": p}, nil
}

func (h *handlers) updatePRD(c *connector.Ctx) (any, error) {
	p, err := h.planOf(c)
	if err != nil {
		return nil, err
	}
	if err := plan.SetPRD(h.layout, &p, c.Input("prd"), c.Input("note")); err != nil {
		return nil, err
	}
	return map[string]any{"plan_id": p.ID, "version": p.Version}, nil
}

func (h *handlers) addFeatures(c *connector.Ctx) (any, error) {
	p, err := h.planOf(c)
	if err != nil {
		return nil, err
	}
	var specs []plan.FeatureSpec
	raw := strings.TrimSpace(c.Input("features"))
	if raw == "" {
		return nil, fmt.Errorf("features is required")
	}
	if err := json.Unmarshal([]byte(raw), &specs); err != nil {
		return nil, fmt.Errorf("features must be a JSON array: %w", err)
	}
	nodes, err := plan.Apply(h.layout, &p, specs, actor(c))
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		tasks := make([]map[string]any, 0, len(n.Tasks))
		for _, t := range n.Tasks {
			tasks = append(tasks, map[string]any{"id": t.ID, "title": t.Title, "status": t.Status})
		}
		out = append(out, map[string]any{
			"feature_id": n.Feature.ID, "title": n.Feature.Title, "tasks": tasks,
		})
	}
	return map[string]any{"plan_id": p.ID, "features": out}, nil
}

func (h *handlers) addTask(c *connector.Ctx) (any, error) {
	p, err := h.planOf(c)
	if err != nil {
		return nil, err
	}
	tk, err := plan.AddTask(h.layout, p, c.Input("feature_id"), plan.TaskSpec{
		Title:      c.Input("title"),
		Body:       c.Input("body"),
		Acceptance: c.Input("acceptance"),
	}, actor(c))
	if err != nil {
		return nil, err
	}
	return map[string]any{"task_id": tk.ID, "feature_id": tk.ParentID, "status": tk.Status}, nil
}

func (h *handlers) setStatus(c *connector.Ctx) (any, error) {
	p, err := h.planOf(c)
	if err != nil {
		return nil, err
	}
	p.Status = strings.TrimSpace(c.Input("status"))
	if err := plan.Save(h.layout, p); err != nil {
		return nil, err
	}
	return map[string]any{"plan_id": p.ID, "status": p.Status}, nil
}

func (h *handlers) addContext(c *connector.Ctx) (any, error) {
	p, err := h.planOf(c)
	if err != nil {
		return nil, err
	}
	if err := plan.AddContext(h.layout, &p, plan.ContextRef{
		Kind: c.Input("kind"), Ref: c.Input("ref"), Label: c.Input("label"),
	}); err != nil {
		return nil, err
	}
	return map[string]any{"plan_id": p.ID, "context_count": len(p.Context)}, nil
}

/* ── resolution ─────────────────────────────────────────────────────────── */

// project returns the explicit project or the calling session's.
func (h *handlers) project(c *connector.Ctx, explicit string) (string, error) {
	if p := strings.TrimSpace(explicit); p != "" {
		if err := team.CheckProjectTarget(c.Context(), h.layout, c.CallerUserID(), c.SessionID(), p); err != nil {
			return "", err
		}
		return p, nil
	}
	sid := c.SessionID()
	if sid == "" {
		return "", fmt.Errorf("project_id is required (no calling session to infer it from)")
	}
	sess, err := session.Load(h.layout, sid)
	if err != nil {
		return "", fmt.Errorf("load calling session: %w", err)
	}
	if sess.Meta.ProjectID == "" {
		return "", fmt.Errorf("this session belongs to no project, so project_id must be given")
	}
	return sess.Meta.ProjectID, nil
}

func (h *handlers) planOf(c *connector.Ctx) (plan.Plan, error) {
	projectID, err := h.project(c, c.Input("project_id"))
	if err != nil {
		return plan.Plan{}, err
	}
	planID := strings.TrimSpace(c.Input("plan_id"))
	if planID == "" {
		return plan.Plan{}, fmt.Errorf("plan_id is required")
	}
	p, err := plan.Load(h.layout, projectID, planID)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("plan %q not found in project %s", planID, projectID)
	}
	return p, nil
}

func actor(c *connector.Ctx) ticket.Actor {
	id := c.CallerUserID()
	if id == "" {
		return ticket.Actor{}
	}
	return ticket.Actor{Type: "user", ID: id, Name: c.UserName(id)}
}
