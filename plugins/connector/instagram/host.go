package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	wickplugin "github.com/yogasw/wick/pkg/plugin"
	"github.com/yogasw/wick/pkg/plugin/host"
	"github.com/yogasw/wick/pkg/plugin/proto/hostpb"
)

// pwConnector is the connector this plugin drives. It is declared in main.go's
// HostResources, which the host enforces: an op outside that list is refused.
const pwConnector = "playwright_browser"

// hostFor is swappable so tests can inject a fake Host API.
var hostFor = func(ctx context.Context) (host.Host, error) { return wickplugin.Host(ctx) }

// resolveInstance returns the playwright_browser instance id to use: the
// configured one, or the first the caller can see.
func resolveInstance(ctx context.Context, configured string) (string, error) {
	if s := strings.TrimSpace(configured); s != "" {
		return s, nil
	}
	h, err := hostFor(ctx)
	if err != nil {
		return "", err
	}
	resp, err := h.ListConnectorInstances(ctx, &hostpb.ListConnectorInstancesRequest{Connector: pwConnector})
	if err != nil {
		return "", fmt.Errorf("list %s instances: %w", pwConnector, err)
	}
	if len(resp.GetInstances()) == 0 {
		return "", fmt.Errorf("no %s instance is visible; install and configure that connector first (browser=chromium)", pwConnector)
	}
	return resp.GetInstances()[0].GetId(), nil
}

// execOp runs one op on the browser connector and decodes its JSON result.
func execOp(ctx context.Context, instance, op string, args map[string]any) (map[string]any, error) {
	h, err := hostFor(ctx)
	if err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encode %s arguments: %w", op, err)
	}
	resp, err := h.ExecuteConnector(ctx, &hostpb.ExecuteConnectorRequest{
		Connector:  pwConnector,
		InstanceId: instance,
		Op:         op,
		ArgsJson:   raw,
	})
	if err != nil {
		return nil, fmt.Errorf("%s.%s: %w", pwConnector, op, err)
	}
	out := map[string]any{}
	if len(resp.GetResultJson()) > 0 {
		if err := json.Unmarshal(resp.GetResultJson(), &out); err != nil {
			return nil, fmt.Errorf("decode %s.%s result: %w", pwConnector, op, err)
		}
	}
	return out, nil
}

// ensureSession reuses a live browser carrying our profile, or opens one. A
// named profile keeps the Instagram login across restarts — that is the whole
// reason this connector never asks for a password.
func ensureSession(ctx context.Context, instance, profile string) (string, error) {
	if list, err := execOp(ctx, instance, "session_list", nil); err == nil {
		for _, raw := range asSlice(list["sessions"]) {
			m := asMap(raw)
			if !strings.EqualFold(asString(m["profile"]), profile) {
				continue
			}
			if id := asString(m["session_id"]); id != "" {
				return id, nil
			}
		}
	}
	opened, err := execOp(ctx, instance, "session_open", map[string]any{"profile": profile})
	if err != nil {
		return "", err
	}
	id := asString(opened["session_id"])
	if id == "" {
		return "", fmt.Errorf("session_open returned no session_id (got %v)", opened)
	}
	return id, nil
}

// runSteps executes a browser action list and returns the raw run result.
func runSteps(ctx context.Context, instance, sid string, actions []map[string]any, extra map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(actions)
	if err != nil {
		return nil, fmt.Errorf("encode actions: %w", err)
	}
	args := map[string]any{"actions": string(raw)}
	if sid != "" {
		args["session_id"] = sid
	}
	for k, v := range extra {
		args[k] = v
	}
	return execOp(ctx, instance, "run", args)
}

// evalOutput runs a single eval action and returns its JSON output.
func evalOutput(ctx context.Context, instance, sid, script string) (map[string]any, error) {
	run, err := runSteps(ctx, instance, sid, []map[string]any{{"action": "eval", "script": script}}, nil)
	if err != nil {
		return nil, err
	}
	return lastStepOutput(run)
}

// lastStepOutput digs the final step's output out of a run result and turns a
// failed step into a readable error instead of a silent zero value.
func lastStepOutput(run map[string]any) (map[string]any, error) {
	steps := asSlice(run["steps"])
	if len(steps) == 0 {
		return nil, fmt.Errorf("browser run produced no steps")
	}
	last := asMap(steps[len(steps)-1])
	if last == nil {
		return nil, fmt.Errorf("malformed browser step result")
	}
	if ok, _ := last["ok"].(bool); !ok {
		return nil, fmt.Errorf("browser step %q failed: %s", asString(last["action"]), asString(last["error"]))
	}
	out := asMap(last["output"])
	if out == nil {
		return nil, fmt.Errorf("browser step %q returned no object", asString(last["action"]))
	}
	return out, nil
}

// ---------------------------------------------------------------- any helpers

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}
