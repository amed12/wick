package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/yogasw/wick/pkg/connector"
)

// connConfig is the resolved per-call configuration.
type connConfig struct {
	username string
	profile  string
	instance string
	maxPages int
	delayMS  int
}

func resolveConfig(c *connector.Ctx) (connConfig, error) {
	cfg := connConfig{
		username: strings.TrimSpace(c.Cfg("username")),
		profile:  strings.TrimSpace(c.Cfg("profile")),
		instance: strings.TrimSpace(c.Cfg("instance_id")),
		maxPages: c.CfgInt("max_pages"),
		delayMS:  c.CfgInt("delay_ms"),
	}
	if cfg.username == "" {
		return cfg, fmt.Errorf("username is required in the connector config")
	}
	if cfg.profile == "" {
		cfg.profile = "instagram"
	}
	if cfg.maxPages <= 0 {
		cfg.maxPages = 3
	}
	if cfg.maxPages > 10 {
		cfg.maxPages = 10
	}
	if cfg.delayMS < 300 {
		cfg.delayMS = 650
	}
	return cfg, nil
}

// ---------------------------------------------------------------- login_status

func handleLoginStatus(c *connector.Ctx) (any, error) {
	cfg, err := resolveConfig(c)
	if err != nil {
		return nil, err
	}
	inst, err := resolveInstance(c.Context(), cfg.instance)
	if err != nil {
		return nil, err
	}
	sid, err := ensureSession(c.Context(), inst, cfg.profile)
	if err != nil {
		return nil, err
	}
	run, err := runSteps(c.Context(), inst, sid, []map[string]any{
		{"action": "eval", "script": navScript(savedURL(cfg.username))},
		{"action": "wait", "ms": 5000},
		{"action": "eval", "script": probeScript},
	}, nil)
	if err != nil {
		return nil, err
	}
	probe, err := lastStepOutput(run)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"account":    cfg.username,
		"profile":    cfg.profile,
		"session_id": sid,
		"logged_in":  probe["logged_in"],
		"url":        probe["url"],
		"title":      probe["title"],
		"hint":       "when logged_in is false, open a live session on this profile and log in by hand; the profile keeps the cookies",
	}, nil
}

// ---------------------------------------------------------------- collection

func handleCollection(c *connector.Ctx) (any, error) {
	cfg, err := resolveConfig(c)
	if err != nil {
		return nil, err
	}
	slug, id, err := parseCollectionRef(c.Input("collection"))
	if err != nil {
		return nil, err
	}
	maxPages := c.InputInt("max_pages")
	if maxPages <= 0 || maxPages > 10 {
		maxPages = cfg.maxPages
	}
	after := strings.TrimSpace(c.Input("after"))

	inst, err := resolveInstance(c.Context(), cfg.instance)
	if err != nil {
		return nil, err
	}
	sid, err := ensureSession(c.Context(), inst, cfg.profile)
	if err != nil {
		return nil, err
	}

	tmpl := loadTemplate(id)
	if tmpl == "" {
		tmpl, err = discoverTemplate(c.Context(), inst, sid, cfg, slug, id)
		if err != nil {
			return nil, err
		}
	}

	out, err := harvest(c.Context(), inst, sid, tmpl, id, after, maxPages, cfg.delayMS)
	if err != nil || asString(out["error"]) != "" {
		// A stale doc_id/token looks exactly like a hard failure; rediscover once.
		dropTemplate(id)
		tmpl, derr := discoverTemplate(c.Context(), inst, sid, cfg, slug, id)
		if derr != nil {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("%s (rediscovery failed too: %w)", asString(out["error"]), derr)
		}
		out, err = harvest(c.Context(), inst, sid, tmpl, id, after, maxPages, cfg.delayMS)
		if err != nil {
			return nil, err
		}
		if msg := asString(out["error"]); msg != "" {
			return nil, fmt.Errorf("harvest failed after rediscovery: %s", msg)
		}
	}

	items := asSlice(out["items"])
	return map[string]any{
		"collection_id": id,
		"slug":          slug,
		"count":         len(items),
		"pages":         out["pages"],
		"next_cursor":   out["next_cursor"],
		"items":         items,
	}, nil
}

// discoverTemplate records one page load and lifts the GraphQL body out of it.
func discoverTemplate(ctx context.Context, inst, sid string, cfg connConfig, slug, id string) (string, error) {
	run, err := runSteps(ctx, inst, sid, []map[string]any{
		{"action": "eval", "script": navScript(collectionURL(cfg.username, slug, id))},
		{"action": "wait", "ms": 6000},
		{"action": "scroll", "delta_y": 3000},
		{"action": "wait", "ms": 2500},
		{"action": "scroll", "delta_y": 3000},
		{"action": "wait", "ms": 2500},
	}, map[string]any{
		"record_request":        true,
		"record_name":           captureName(id),
		"record_url_pattern":    "graphql",
		"record_include_assets": false,
	})
	if err != nil {
		return "", err
	}
	if err := runFailure(run); err != nil {
		return "", err
	}
	// The capture is keyed by record_name (not by profile), so ask for exactly
	// the name this discovery pass recorded under.
	cap, err := execOp(ctx, inst, "get_request", map[string]any{"name": captureName(id)})
	if err != nil {
		return "", err
	}
	tmpl, err := extractTemplate(asSlice(cap["requests"]), id)
	if err != nil {
		return "", err
	}
	_ = saveTemplate(id, tmpl)
	return tmpl, nil
}

func harvest(ctx context.Context, inst, sid, tmpl, id, after string, maxPages, delayMS int) (map[string]any, error) {
	return evalOutput(ctx, inst, sid, harvestScript(tmpl, id, after, maxPages, delayMS))
}

// ---------------------------------------------------------------- post

func handlePost(c *connector.Ctx) (any, error) {
	cfg, err := resolveConfig(c)
	if err != nil {
		return nil, err
	}
	target := strings.TrimSpace(c.Input("url"))
	if target == "" {
		return nil, fmt.Errorf("url is required")
	}
	inst, err := resolveInstance(c.Context(), cfg.instance)
	if err != nil {
		return nil, err
	}
	sid, err := ensureSession(c.Context(), inst, cfg.profile)
	if err != nil {
		return nil, err
	}
	run, err := runSteps(c.Context(), inst, sid, []map[string]any{
		{"action": "eval", "script": navScript(target)},
		{"action": "wait", "ms": 4000},
		{"action": "eval", "script": postScript},
	}, nil)
	if err != nil {
		return nil, err
	}
	return lastStepOutput(run)
}
