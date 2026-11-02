package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yogasw/wick/pkg/connector"
	"github.com/yogasw/wick/pkg/plugin/host"
	"github.com/yogasw/wick/pkg/plugin/proto/hostpb"
)

// fakeHost answers only the two Host API calls this plugin makes; every other
// method exists to satisfy the interface.
type fakeHost struct {
	instances []string
	sessions  []map[string]any
	opened    string
	captures  []any
	runCalls  []map[string]any
	runOut    func(call int, args map[string]any) map[string]any
}

func (f *fakeHost) Whoami(context.Context) (*hostpb.WhoamiResponse, error) { return nil, nil }
func (f *fakeHost) ListMethods(context.Context) (*hostpb.ListMethodsResponse, error) {
	return nil, nil
}
func (f *fakeHost) ListUsers(context.Context, *hostpb.ListUsersRequest) (*hostpb.ListUsersResponse, error) {
	return nil, nil
}
func (f *fakeHost) GetUser(context.Context, *hostpb.GetUserRequest) (*hostpb.User, error) {
	return nil, nil
}
func (f *fakeHost) GetUserByEmail(context.Context, *hostpb.GetUserByEmailRequest) (*hostpb.User, error) {
	return nil, nil
}
func (f *fakeHost) ConnectedAccounts(context.Context, *hostpb.ConnectedAccountsRequest) (*hostpb.ConnectedAccountsResponse, error) {
	return nil, nil
}
func (f *fakeHost) ListSessions(context.Context, *hostpb.ListSessionsRequest) (*hostpb.ListSessionsResponse, error) {
	return nil, nil
}
func (f *fakeHost) GetSessionMeta(context.Context, *hostpb.GetSessionMetaRequest) (*hostpb.SessionMeta, error) {
	return nil, nil
}
func (f *fakeHost) SendToSession(context.Context, *hostpb.SendToSessionRequest) (*hostpb.SendToSessionResponse, error) {
	return nil, nil
}
func (f *fakeHost) GetTicket(context.Context, *hostpb.GetTicketRequest) (*hostpb.Ticket, error) {
	return nil, nil
}
func (f *fakeHost) ListTickets(context.Context, *hostpb.ListTicketsRequest) (*hostpb.ListTicketsResponse, error) {
	return nil, nil
}
func (f *fakeHost) CreateTicket(context.Context, *hostpb.CreateTicketRequest) (*hostpb.Ticket, error) {
	return nil, nil
}
func (f *fakeHost) UpdateTicket(context.Context, *hostpb.UpdateTicketRequest) (*hostpb.Ticket, error) {
	return nil, nil
}
func (f *fakeHost) AttachSession(context.Context, *hostpb.AttachSessionRequest) error { return nil }
func (f *fakeHost) GetConfig(context.Context, *hostpb.GetConfigRequest) (*hostpb.GetConfigResponse, error) {
	return nil, nil
}
func (f *fakeHost) Invoke(context.Context, *hostpb.InvokeRequest) (*hostpb.InvokeResponse, error) {
	return nil, nil
}
func (f *fakeHost) Rest(context.Context, *hostpb.RestRequest) (*hostpb.RestResponse, error) {
	return nil, nil
}
func (f *fakeHost) Query(context.Context, *hostpb.QueryRequest) (*hostpb.QueryResponse, error) {
	return nil, nil
}

func (f *fakeHost) ListConnectorInstances(_ context.Context, in *hostpb.ListConnectorInstancesRequest) (*hostpb.ListConnectorInstancesResponse, error) {
	resp := &hostpb.ListConnectorInstancesResponse{}
	if in.GetConnector() != pwConnector {
		return resp, nil
	}
	for _, id := range f.instances {
		resp.Instances = append(resp.Instances, &hostpb.ConnectorInstance{Id: id, Connector: pwConnector})
	}
	return resp, nil
}

func (f *fakeHost) ExecuteConnector(_ context.Context, in *hostpb.ExecuteConnectorRequest) (*hostpb.ExecuteConnectorResponse, error) {
	var args map[string]any
	_ = json.Unmarshal(in.GetArgsJson(), &args)
	var out map[string]any
	switch in.GetOp() {
	case "session_list":
		out = map[string]any{"sessions": toAny(f.sessions)}
	case "session_open":
		out = map[string]any{"session_id": f.opened}
	case "get_request":
		out = map[string]any{"requests": f.captures, "count": len(f.captures)}
	case "run":
		f.runCalls = append(f.runCalls, args)
		if f.runOut != nil {
			out = f.runOut(len(f.runCalls), args)
		}
	default:
		out = map[string]any{}
	}
	raw, _ := json.Marshal(out)
	return &hostpb.ExecuteConnectorResponse{ResultJson: raw}, nil
}

// toAny converts []map[string]any to []any so JSON round-trips the same way a
// real host response does.
func toAny(in []map[string]any) []any {
	out := make([]any, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}

func runOK(output map[string]any) map[string]any {
	return map[string]any{
		"completed": true,
		"steps":     []any{map[string]any{"step": 0, "action": "eval", "ok": true, "output": output}},
	}
}

func newTestCtx(t *testing.T, input map[string]string) *connector.Ctx {
	t.Helper()
	t.Setenv("WICK_PLUGIN_DATA_DIR", t.TempDir())
	cfg := map[string]string{
		"username": "someone", "profile": "instagram", "max_pages": "3", "delay_ms": "650",
	}
	return connector.NewCtx(context.Background(), "test-instance", cfg, input, nil, nil, nil)
}

const capturedBody = "av=1&doc_id=29009332198686334&variables=%7B%22collection_id%22%3A%22183%22%7D&fb_dtsg=abc&lsd=def"

func TestCollectionDiscoversThenHarvests(t *testing.T) {
	fake := &fakeHost{
		instances: []string{"pw1"},
		opened:    "sess-1",
		captures: []any{map[string]any{
			"method": "POST", "url": "https://www.instagram.com/api/graphql", "body": capturedBody,
		}},
		runOut: func(call int, args map[string]any) map[string]any {
			// Call 1 = discovery (records the request); call 2 = harvest.
			if call == 1 {
				return runOK(map[string]any{"ok": true})
			}
			return runOK(map[string]any{
				"items": []any{
					map[string]any{"code": "AAA", "owner": "someone", "url": "https://www.instagram.com/someone/reel/AAA/"},
					map[string]any{"code": "BBB", "owner": "someone", "url": "https://www.instagram.com/someone/p/BBB/"},
				},
				"pages":       1,
				"next_cursor": "CURSOR1",
				"error":       nil,
			})
		},
	}
	hostFor = func(context.Context) (host.Host, error) { return fake, nil }

	out, err := handleCollection(newTestCtx(t, map[string]string{"collection": "ai/183"}))
	if err != nil {
		t.Fatalf("collection: %v", err)
	}
	got := out.(map[string]any)
	if got["count"].(int) != 2 {
		t.Fatalf("count = %v, want 2", got["count"])
	}
	if got["next_cursor"] != "CURSOR1" {
		t.Errorf("next_cursor = %v, want CURSOR1", got["next_cursor"])
	}
	if len(fake.runCalls) != 2 {
		t.Fatalf("browser runs = %d, want 2 (discover + harvest)", len(fake.runCalls))
	}
	harvestArgs := fake.runCalls[1]
	actions := asString(harvestArgs["actions"])
	if !strings.Contains(actions, "183") {
		t.Errorf("harvest script lost the collection id: %s", actions)
	}
	if harvestArgs["session_id"] != "sess-1" {
		t.Errorf("harvest ran without the live session: %v", harvestArgs["session_id"])
	}
	if _, recorded := fake.runCalls[0]["record_request"]; !recorded {
		t.Errorf("discovery did not ask for request recording")
	}
}

func TestCollectionRetriesOnceWhenCachedTemplateIsStale(t *testing.T) {
	fake := &fakeHost{
		instances: []string{"pw1"},
		opened:    "sess-1",
		captures: []any{map[string]any{
			"method": "POST", "url": "https://www.instagram.com/api/graphql", "body": capturedBody,
		}},
		runOut: func(call int, args map[string]any) map[string]any {
			switch call {
			case 1: // discovery
				return runOK(map[string]any{"ok": true})
			case 2: // first harvest: Instagram rotated doc_id
				return runOK(map[string]any{"items": []any{}, "pages": 0, "next_cursor": nil, "error": "non-json response 404: <html>"})
			case 3: // rediscovery
				return runOK(map[string]any{"ok": true})
			default: // second harvest
				return runOK(map[string]any{
					"items":       []any{map[string]any{"code": "CCC"}},
					"pages":       1,
					"next_cursor": nil,
					"error":       nil,
				})
			}
		},
	}
	hostFor = func(context.Context) (host.Host, error) { return fake, nil }

	out, err := handleCollection(newTestCtx(t, map[string]string{"collection": "ai/183"}))
	if err != nil {
		t.Fatalf("collection should recover from a stale template: %v", err)
	}
	if got := out.(map[string]any)["count"].(int); got != 1 {
		t.Fatalf("count = %d, want 1 after recovery", got)
	}
	if len(fake.runCalls) != 4 {
		t.Fatalf("browser runs = %d, want 4 (discover, harvest, rediscover, harvest)", len(fake.runCalls))
	}
}

func TestCollectionReusesLiveSessionWithTheSameProfile(t *testing.T) {
	fake := &fakeHost{
		instances: []string{"pw1"},
		captures: []any{map[string]any{
			"method": "POST", "url": "https://www.instagram.com/api/graphql", "body": capturedBody,
		}},
		sessions: []map[string]any{
			{"session_id": "old", "profile": "other"},
			{"session_id": "keep", "profile": "Instagram"},
		},
		runOut: func(_ int, args map[string]any) map[string]any {
			if _, discovering := args["record_request"]; discovering {
				return runOK(map[string]any{"ok": true})
			}
			return runOK(map[string]any{"items": []any{}, "pages": 1, "next_cursor": nil, "error": nil})
		},
	}
	hostFor = func(context.Context) (host.Host, error) { return fake, nil }

	if _, err := handleCollection(newTestCtx(t, map[string]string{"collection": "ai/183"})); err != nil {
		t.Fatalf("collection: %v", err)
	}
	for _, call := range fake.runCalls {
		if call["session_id"] != "keep" {
			t.Fatalf("expected reuse of the matching profile session, got %v", call["session_id"])
		}
	}
}

func TestLoginStatusReportsLoggedOut(t *testing.T) {
	fake := &fakeHost{
		instances: []string{"pw1"},
		opened:    "sess-9",
		runOut: func(call int, args map[string]any) map[string]any {
			return runOK(map[string]any{"logged_in": false, "url": "https://www.instagram.com/accounts/login/", "title": "Instagram"})
		},
	}
	hostFor = func(context.Context) (host.Host, error) { return fake, nil }

	out, err := handleLoginStatus(newTestCtx(t, nil))
	if err != nil {
		t.Fatalf("login_status: %v", err)
	}
	got := out.(map[string]any)
	if got["logged_in"] != false {
		t.Errorf("logged_in = %v, want false", got["logged_in"])
	}
	if got["session_id"] != "sess-9" {
		t.Errorf("session_id = %v", got["session_id"])
	}
}

func TestMissingInstanceIsAnActionableError(t *testing.T) {
	fake := &fakeHost{}
	hostFor = func(context.Context) (host.Host, error) { return fake, nil }

	_, err := handleLoginStatus(newTestCtx(t, nil))
	if err == nil || !strings.Contains(err.Error(), "playwright_browser") {
		t.Fatalf("want an error naming the missing connector, got %v", err)
	}
}
