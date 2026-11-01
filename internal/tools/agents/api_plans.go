package agents

import (
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/yogasw/wick/internal/agents/plan"
	"github.com/yogasw/wick/internal/agents/ticket"
	"github.com/yogasw/wick/pkg/tool"
)

// registerPlanRoutes wires the whole planning workspace: the SPA page and
// its JSON API. It is called from Register as one line so this fork's diff
// against upstream stays small (see local/README.md).
func registerPlanRoutes(r tool.Router) {
	// Page: the SPA owns its client-side routes, so one shell entry is
	// enough. The /plan/ subtree is served by registerSPAPlan.
	r.GET("/plan", planPage)

	// JSON API — a plan belongs to a project, so every route sits under
	// /api/projects/{id} and projectAccessMW covers it. The plan's own id is
	// only unique within its project.
	r.GET("/api/projects/{id}/plans", apiPlanList)
	r.POST("/api/projects/{id}/plans", apiPlanCreate)
	r.GET("/api/projects/{id}/plans/{planID}", apiPlanDetail)
	r.PATCH("/api/projects/{id}/plans/{planID}", apiPlanUpdate)
	r.DELETE("/api/projects/{id}/plans/{planID}", apiPlanDelete)
	r.POST("/api/projects/{id}/plans/{planID}/features", apiPlanFeatures)
	r.POST("/api/projects/{id}/plans/{planID}/context", apiPlanContextAdd)
	r.DELETE("/api/projects/{id}/plans/{planID}/context", apiPlanContextRemove)
}

// apiPlanList returns a project's plans in phase order, each with its
// feature count and progress so the picker does not need N follow-up calls.
func apiPlanList(c *tool.Ctx) {
	if notReady(c) {
		return
	}
	projectID := c.PathValue("id")
	plans, err := plan.List(globalLayout, projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	type row struct {
		plan.Plan
		Features int `json:"feature_count"`
		Done     int `json:"done"`
		Total    int `json:"total"`
	}
	out := make([]row, 0, len(plans))
	for _, p := range plans {
		nodes, _ := plan.Build(globalLayout, p)
		r := row{Plan: p, Features: len(nodes)}
		for _, n := range nodes {
			r.Done += n.Done
			r.Total += n.Total
		}
		out = append(out, r)
	}
	c.JSON(http.StatusOK, map[string]any{"plans": out})
}

// apiPlanCreate starts the next phase for a project.
func apiPlanCreate(c *tool.Ctx) {
	if notReady(c) {
		return
	}
	var req struct {
		Title   string `json:"title"`
		Idea    string `json:"idea"`
		PRD     string `json:"prd"`
		Summary string `json:"summary"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	p, err := plan.Create(globalLayout, plan.CreateOptions{
		ProjectID: c.PathValue("id"),
		Title:     req.Title,
		Idea:      req.Idea,
		PRD:       req.PRD,
		Summary:   req.Summary,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, map[string]any{"plan": p})
}

// apiPlanDetail returns one plan plus its feature → task tree.
func apiPlanDetail(c *tool.Ctx) {
	if notReady(c) {
		return
	}
	p, ok := loadPlan(c)
	if !ok {
		return
	}
	nodes, err := plan.Build(globalLayout, p)
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]any{"plan": p, "features": nodes})
}

// apiPlanUpdate edits the document, status or metadata. A present prd
// field records a version; an unchanged document does not.
func apiPlanUpdate(c *tool.Ctx) {
	if notReady(c) {
		return
	}
	p, ok := loadPlan(c)
	if !ok {
		return
	}
	var req struct {
		Title   *string `json:"title"`
		Idea    *string `json:"idea"`
		Summary *string `json:"summary"`
		Status  *string `json:"status"`
		PRD     *string `json:"prd"`
		Note    string  `json:"note"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Title != nil {
		p.Title = strings.TrimSpace(*req.Title)
	}
	if req.Idea != nil {
		p.Idea = strings.TrimSpace(*req.Idea)
	}
	if req.Summary != nil {
		p.Summary = strings.TrimSpace(*req.Summary)
	}
	if req.Status != nil {
		p.Status = strings.TrimSpace(*req.Status)
	}
	if req.PRD != nil {
		if err := plan.SetPRD(globalLayout, &p, *req.PRD, req.Note); err != nil {
			c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := plan.Save(globalLayout, p); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]any{"plan": p})
}

// apiPlanDelete removes the document. The tickets it produced stay on the
// board — deleting a plan must not erase work in flight.
func apiPlanDelete(c *tool.Ctx) {
	if notReady(c) {
		return
	}
	p, ok := loadPlan(c)
	if !ok {
		return
	}
	if err := plan.Delete(globalLayout, p.ProjectID, p.ID); err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]any{"deleted": p.ID})
}

// apiPlanFeatures applies a feature/task breakdown to the plan.
func apiPlanFeatures(c *tool.Ctx) {
	if notReady(c) {
		return
	}
	p, ok := loadPlan(c)
	if !ok {
		return
	}
	var req struct {
		Features []plan.FeatureSpec `json:"features"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	nodes, err := plan.Apply(globalLayout, &p, req.Features, callerActor(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// A plan that produced tickets uses the board, whether or not the
	// project had ticket mode on before. Without this the work exists but
	// its execution surface is hidden, which reads as "nothing happened".
	if proj, ok := globalMgr.Registry().Project(p.ProjectID); ok && !proj.Meta.Ticket.Enabled {
		meta := proj.Meta
		meta.Ticket.Enabled = true
		if _, uerr := globalMgr.UpdateProject(c.Context(), p.ProjectID, meta); uerr != nil {
			log.Ctx(c.Context()).Warn().Err(uerr).Str("project", p.ProjectID).
				Msg("plans: could not enable ticket mode after breakdown")
		}
	}
	c.JSON(http.StatusOK, map[string]any{"plan": p, "features": nodes})
}

// apiPlanContext adds a source item to the plan.
func apiPlanContextAdd(c *tool.Ctx) {
	if notReady(c) {
		return
	}
	p, ok := loadPlan(c)
	if !ok {
		return
	}
	var req struct {
		Kind  string `json:"kind"`
		Ref   string `json:"ref"`
		Label string `json:"label"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if err := plan.AddContext(globalLayout, &p, plan.ContextRef{Kind: req.Kind, Ref: req.Ref, Label: req.Label}); err != nil {
		c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]any{"plan": p})
}

// apiPlanContextRemove drops a source item.
func apiPlanContextRemove(c *tool.Ctx) {
	if notReady(c) {
		return
	}
	p, ok := loadPlan(c)
	if !ok {
		return
	}
	removed, err := plan.RemoveContext(globalLayout, &p, c.Query("kind"), c.Query("ref"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, map[string]any{"removed": removed, "plan": p})
}

// loadPlan resolves the {planID} in the URL inside the project in {id},
// answering the HTTP error itself when it cannot.
func loadPlan(c *tool.Ctx) (plan.Plan, bool) {
	projectID := c.PathValue("id")
	planID := c.PathValue("planID")
	p, err := plan.Load(globalLayout, projectID, planID)
	if err != nil {
		if errors.Is(err, plan.ErrNotFound) {
			c.JSON(http.StatusNotFound, map[string]string{"error": "plan not found"})
		} else {
			c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return plan.Plan{}, false
	}
	return p, true
}

// apiTicketPlanningFields is the small helper the SPA uses to render a
// ticket that belongs to a plan (kind, parent, acceptance, dependencies).
func ticketPlanningPayload(tk ticket.Ticket) map[string]any {
	return map[string]any{
		"kind":       tk.KindOf(),
		"parent_id":  tk.ParentID,
		"plan_id":    tk.PlanID,
		"acceptance": tk.Acceptance,
		"depends_on": tk.DependsOn,
	}
}
