package plan

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yogasw/wick/internal/agents/aigen"
	wfprovider "github.com/yogasw/wick/internal/agents/workflow/provider"
)

// PRDKind is the aigen kind behind the planning workspace's Generate
// button. One call produces the document AND the feature/task breakdown,
// because a PRD nobody decomposed is a document an agent cannot start
// from.
const PRDKind = "plan-prd"

// Draft is one generated planning document.
type Draft struct {
	Title    string        `json:"title"`
	Summary  string        `json:"summary"`
	PRD      string        `json:"prd"`
	Features []FeatureSpec `json:"features"`
}

var prdSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"title":   map[string]any{"type": "string"},
		"summary": map[string]any{"type": "string"},
		"prd":     map[string]any{"type": "string"},
		"features": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":      map[string]any{"type": "string"},
					"body":       map[string]any{"type": "string"},
					"acceptance": map[string]any{"type": "string"},
					"tasks": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"title":      map[string]any{"type": "string"},
								"body":       map[string]any{"type": "string"},
								"acceptance": map[string]any{"type": "string"},
							},
							"required": []string{"title"},
						},
					},
				},
				"required": []string{"title", "tasks"},
			},
		},
	},
	"required": []string{"title", "summary", "prd", "features"},
}

const prdPromptHead = `You are a senior product engineer writing a planning document that an autonomous coding agent will execute.

Reply with the JSON object only. Rules:
- Write in the language of the user's idea (Indonesian when the idea is Indonesian, English otherwise).
- prd: a complete markdown document with these sections, in order: "# <product name>", "## 1. Overview", "## 2. Goals", "## 3. Non-goals", "## 4. Users", "## 5. Requirements", "## 6. Core features", "## 7. User flow", "## 8. Architecture", "## 9. Data model", "## 10. Risks & open questions". Use Mermaid fenced blocks for the architecture diagram. Be concrete; the agent has no other context than this document.
- features: 3-8 features, each independently shippable. title is short (2-5 words). body explains the user-visible behaviour.
- tasks: 2-8 per feature, each small enough for one agent turn. Write acceptance as a checklist ("- [ ] ...") that a reviewer can verify without reading the code.
- Never invent integrations, credentials or infrastructure the idea did not mention. If something is unknown, put it under Risks instead of guessing.
- No prose outside the JSON.
`

// PRDKindSpec is the aigen.Kind for PRDKind.
func PRDKindSpec() aigen.Kind {
	return aigen.Kind{
		Name:     PRDKind,
		MaxInput: 24 * 1024,
		Validate: validatePRDInput,
		Build: func(in aigen.Input) (wfprovider.StructuredRequest, error) {
			return wfprovider.StructuredRequest{Prompt: prdPrompt(in), Schema: prdSchema}, nil
		},
		Finish: func(res wfprovider.StructuredResult) (any, error) { return DraftFromResult(res) },
	}
}

func validatePRDInput(in aigen.Input) error {
	if strings.TrimSpace(in.Text) == "" && strings.TrimSpace(in.Fields["idea"]) == "" {
		return errors.New("describe the idea first")
	}
	return nil
}

func prdPrompt(in aigen.Input) string {
	var b strings.Builder
	b.WriteString(prdPromptHead)
	if t := strings.TrimSpace(in.Fields["title"]); t != "" {
		fmt.Fprintf(&b, "\nPLAN TITLE: %s\n", t)
	}
	if p := strings.TrimSpace(in.Fields["project_name"]); p != "" {
		fmt.Fprintf(&b, "PROJECT: %s\n", p)
	}
	if e := strings.TrimSpace(in.Fields["existing_prd"]); e != "" {
		fmt.Fprintf(&b, "\nCURRENT PRD (revise it, keep what still holds):\n%s\n", clamp(e, 12*1024))
	}
	if c := strings.TrimSpace(in.Fields["context"]); c != "" {
		fmt.Fprintf(&b, "\nCONTEXT (source material the plan must honour):\n%s\n", clamp(c, 8*1024))
	}
	idea := strings.TrimSpace(in.Text)
	if idea == "" {
		idea = strings.TrimSpace(in.Fields["idea"])
	}
	fmt.Fprintf(&b, "\nIDEA:\n%s\n", clamp(idea, 8*1024))
	return b.String()
}

func clamp(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…[truncated]"
}

// DraftFromResult shapes the provider answer into a Draft and validates it
// enough that an empty answer fails loudly instead of overwriting a plan
// with nothing.
func DraftFromResult(res wfprovider.StructuredResult) (Draft, error) {
	if !res.OK || res.Parsed == nil {
		msg := res.Error
		if msg == "" {
			msg = "no structured result"
		}
		return Draft{}, fmt.Errorf("generate failed: %s", msg)
	}
	str := func(k string) string {
		s, _ := res.Parsed[k].(string)
		return strings.TrimSpace(s)
	}
	d := Draft{
		Title:   str("title"),
		Summary: str("summary"),
		PRD:     str("prd"),
	}
	if d.PRD == "" {
		return Draft{}, errors.New("generate returned an empty PRD — try a more specific idea")
	}
	raw, _ := res.Parsed["features"].([]any)
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		f := FeatureSpec{
			Title:      strings.TrimSpace(asString(m["title"])),
			Body:       strings.TrimSpace(asString(m["body"])),
			Acceptance: strings.TrimSpace(asString(m["acceptance"])),
		}
		if f.Title == "" {
			continue
		}
		if tasks, ok := m["tasks"].([]any); ok {
			for _, ti := range tasks {
				tm, ok := ti.(map[string]any)
				if !ok {
					continue
				}
				t := TaskSpec{
					Title:      strings.TrimSpace(asString(tm["title"])),
					Body:       strings.TrimSpace(asString(tm["body"])),
					Acceptance: strings.TrimSpace(asString(tm["acceptance"])),
				}
				if t.Title == "" {
					continue
				}
				f.Tasks = append(f.Tasks, t)
			}
		}
		d.Features = append(d.Features, f)
	}
	if len(d.Features) == 0 {
		return Draft{}, errors.New("generate returned no features — try a more specific idea")
	}
	// The schema is advisory: a model can still exceed the caps, and the
	// Apply path must not reject a draft the user just watched arrive.
	if len(d.Features) > MaxFeatures {
		d.Features = d.Features[:MaxFeatures]
	}
	for i := range d.Features {
		if len(d.Features[i].Tasks) > MaxTasksPerFtr {
			d.Features[i].Tasks = d.Features[i].Tasks[:MaxTasksPerFtr]
		}
	}
	if len(d.PRD) > MaxPRDBytes {
		d.PRD = d.PRD[:MaxPRDBytes]
	}
	return d, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
