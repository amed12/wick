package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// card is the subset of a Planban roadmap item this connector reads or writes.
// Planban also stores priority/ranks/docs; they are omitted deliberately so the
// LLM sees a stable, small shape.
type card struct {
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	Status     string         `json:"status"`
	Summary    string         `json:"summary"`
	NextAction string         `json:"nextAction"`
	Tags       []string       `json:"tags"`
	IsGroup    bool           `json:"isGroup"`
	ParentID   *string        `json:"parentId"`
	Metadata   map[string]any `json:"metadata"`
}

// boardState is the slice of GET /state this connector uses.
type boardState struct {
	Roadmap struct {
		Revision     int    `json:"revision"`
		RoadmapItems []card `json:"roadmapItems"`
	} `json:"roadmap"`
}

// client talks to one Planban server. Every call is bound to the operation's
// context so a cancelled MCP call aborts the HTTP request instead of leaking.
type client struct {
	base string
	hc   *http.Client
	ctx  context.Context
}

func newClient(ctx context.Context, hc *http.Client, base string) *client {
	return &client{base: strings.TrimRight(base, "/"), hc: hc, ctx: ctx}
}

func (c *client) do(method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(c.ctx, method, c.base+path, rdr)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Planban refuses mutating requests that do not come from its own origin
	// (see mutationOriginError in src/server/server.ts), so state it explicitly.
	if method != http.MethodGet {
		req.Header.Set("Origin", c.base)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s %s: %w", method, path, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, truncate(string(raw), 400))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s %s: %w (body: %s)", method, path, err, truncate(string(raw), 200))
	}
	return nil
}

func (c *client) cards(repo string) ([]card, error) {
	var out []card
	if err := c.do(http.MethodGet, boardPath(repo, "/cards"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) state(repo string) (boardState, error) {
	var out boardState
	if err := c.do(http.MethodGet, boardPath(repo, "/state"), nil, &out); err != nil {
		return boardState{}, err
	}
	return out, nil
}

func (c *client) createCard(repo string, body map[string]any) (boardState, error) {
	var out boardState
	err := c.do(http.MethodPost, boardPath(repo, "/cards"), body, &out)
	return out, err
}

func (c *client) patchCard(repo, id string, body map[string]any) (boardState, error) {
	var out boardState
	err := c.do(http.MethodPatch, boardPath(repo, "/cards/"+url.PathEscape(id)), body, &out)
	return out, err
}

func (c *client) moveCard(repo, id string, body map[string]any) (boardState, error) {
	var out boardState
	err := c.do(http.MethodPost, boardPath(repo, "/cards/"+url.PathEscape(id)+"/move"), body, &out)
	return out, err
}

func (c *client) createGroup(repo string, body map[string]any) (boardState, error) {
	var out boardState
	err := c.do(http.MethodPost, boardPath(repo, "/groups"), body, &out)
	return out, err
}

func (c *client) deleteCard(repo, id string) error {
	return c.do(http.MethodDelete, boardPath(repo, "/cards/"+url.PathEscape(id)), nil, nil)
}

// boardPath renders /api/boards/<repo>/<suffix>.
func boardPath(repo, suffix string) string {
	return "/api/boards/" + url.PathEscape(repo) + suffix
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
