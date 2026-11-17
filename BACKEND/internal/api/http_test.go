package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"businessautomation/internal/app"
	"businessautomation/internal/repository"
)

type client struct {
	t     *testing.T
	h     http.Handler
	token string
}

func (c client) call(method, path string, body any, status int) map[string]any {
	c.t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	if w.Code != status {
		c.t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, w.Code, status, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		c.t.Fatal(err)
	}
	if status >= 400 {
		return out
	}
	data, _ := out["data"].(map[string]any)
	return data
}

func (c client) account(email string) client {
	c.t.Helper()
	credentials := map[string]any{"name": "Test Owner", "email": email, "password": "a-strong-password-2026"}
	u := c.call("POST", "/api/v1/auth/register", credentials, 201)
	if _, ok := u["password_hash"]; ok {
		c.t.Fatal("password hash exposed")
	}
	login := c.call("POST", "/api/v1/auth/login", credentials, 200)
	c.token = login["access_token"].(string)
	return c
}

func TestWorkflowIsolationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	repo, err := repository.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	a := root.account("owner-a@example.com")
	b := root.account("owner-b@example.com")
	root.call("GET", "/api/v1/customers", nil, 401)
	customer := a.call("POST", "/api/v1/customers", map[string]any{"name": "Khách A", "tenant_id": "spoof"}, 201)
	customerID := customer["id"].(string)
	if customer["tenant_id"] == "spoof" {
		t.Fatal("tenant override accepted")
	}
	b.call("GET", "/api/v1/customers/"+customerID, nil, 404)
	b.call("POST", "/api/v1/conversations", map[string]any{"customer_id": customerID}, 404)
	conv := a.call("POST", "/api/v1/conversations", map[string]any{"customer_id": customerID}, 201)
	convID := conv["id"].(string)
	base := "/api/v1/conversations/" + convID
	a.call("POST", "/api/v1/automations", map[string]any{"name": "Khiếu nại", "keywords": []string{"refund"}, "enabled": true}, 201)
	article := a.call("POST", "/api/v1/knowledge", map[string]any{"subject": "Refund policy", "content": "Refund within seven days.", "enabled": true}, 201)
	a.call("POST", base+"/messages", map[string]any{"content": "refund policy please", "sender": "customer"}, 201)
	a.call("POST", base+"/messages", map[string]any{"content": "refund again", "sender": "customer"}, 201)
	dash := a.call("GET", "/api/v1/dashboard", nil, 200)
	if dash["tickets"] != float64(1) {
		t.Fatalf("automation duplicated ticket: %v", dash)
	}
	reply := a.call("POST", base+"/reply", nil, 201)
	if reply["source_id"] != article["id"] || reply["needs_human"] != false {
		t.Fatalf("bad knowledge reply: %v", reply)
	}
	a.call("POST", base+"/reply", nil, 409)
	b.call("GET", base+"/messages", nil, 404)
	b.call("POST", base+"/reply", nil, 404)
	a.call("POST", base+"/messages", map[string]any{"content": "xyzabcunique", "sender": "customer"}, 201)
	fallback := a.call("POST", base+"/reply", nil, 201)
	if fallback["needs_human"] != true {
		t.Fatal("expected handoff")
	}
	a.call("PUT", base, map[string]any{"customer_id": customerID, "status": "closed"}, 200)
	a.call("POST", base+"/messages", map[string]any{"content": "hello", "sender": "customer"}, 409)
	a.call("GET", "/api/v1/customers?limit=0", nil, 400)
	a.call("POST", "/api/v1/tickets", map[string]any{"subject": "Bad", "status": "invalid"}, 400)
	a.call("POST", "/api/v1/team", map[string]any{"name": "Agent", "email": "agent@example.com", "password": "a-strong-agent-password"}, 201)
	login := root.call("POST", "/api/v1/auth/login", map[string]any{"email": "agent@example.com", "password": "a-strong-agent-password"}, 200)
	agent := root
	agent.token = login["access_token"].(string)
	agent.call("POST", "/api/v1/knowledge", map[string]any{"subject": "x", "content": "y"}, 403)
	agent.call("POST", "/api/v1/automations", map[string]any{"name": "x", "keywords": []string{"x"}}, 403)
	agent.call("POST", "/api/v1/team", map[string]any{"name": "x"}, 403)
	agent.call("GET", "/api/v1/customers/"+customerID, nil, 200)
	reloaded, err := repository.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a.h = app.NewHandler(reloaded)
	a.call("GET", "/api/v1/customers/"+customerID, nil, 200)
	a.call("POST", "/api/v1/auth/logout", nil, 200)
	a.call("GET", "/api/v1/auth/me", nil, 401)
}

func TestRequestValidation(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := client{t: t, h: app.NewHandler(repo)}
	c.call("POST", "/api/v1/auth/register", map[string]any{"email": "bad", "password": "short"}, 400)
	c.call("POST", "/api/v1/auth/login", map[string]any{"email": "missing@example.com", "password": "password"}, 401)
	r := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(`{"email":"x","unknown":1}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(`{}`))
	w = httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatal(w.Code)
	}
	c.call("GET", "/healthz", nil, 200)
	c.call("DELETE", "/healthz", nil, 405)
}

func TestAuthRateLimit(t *testing.T) {
	repo, _ := repository.Open(filepath.Join(t.TempDir(), "state.json"))
	c := client{t: t, h: app.NewHandler(repo)}
	for i := 0; i < 20; i++ {
		c.call("POST", "/api/v1/auth/register", map[string]any{}, 400)
	}
	c.call("POST", "/api/v1/auth/register", map[string]any{}, 429)
}

func TestTypedInputsAndConversationOwnership(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	c := root.account("typed@example.com")
	c.call("POST", "/api/v1/customers", map[string]any{"name": "Customer", "priority": "urgent"}, 400)
	first := c.call("POST", "/api/v1/customers", map[string]any{"name": "First"}, 201)
	second := c.call("POST", "/api/v1/customers", map[string]any{"name": "Second"}, 201)
	conversation := c.call("POST", "/api/v1/conversations", map[string]any{"customer_id": first["id"]}, 201)
	c.call("PUT", "/api/v1/conversations/"+conversation["id"].(string), map[string]any{"customer_id": second["id"]}, 409)
	c.call("POST", "/api/v1/automations", map[string]any{"name": "Bad", "keywords": []string{" "}}, 400)
	c.call("POST", "/api/v1/customers", nil, 400)
	root.call("POST", "/api/v1/auth/register", map[string]any{"name": "Bad", "email": "Display <bad@example.com>", "password": "a-strong-password-2026"}, 400)
}

func TestPaginationFilteringContract(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	user := root.account("pagination@example.com")
	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		user.call("POST", "/api/v1/customers", map[string]any{"name": name}, 201)
	}
	cases := []struct {
		name, path   string
		total, items int
	}{
		{"second page", "?page=2&limit=2", 3, 1},
		{"case insensitive search", "?q=ALPHA", 1, 1},
		{"past end", "?page=10", 3, 0},
		{"status filter", "?status=resolved", 0, 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := user.call("GET", "/api/v1/customers"+testCase.path, nil, 200)
			if response["total"] != float64(testCase.total) {
				t.Errorf("total=%v want=%d", response["total"], testCase.total)
			}
			items, ok := response["items"].([]any)
			if !ok {
				t.Fatalf("items should be a JSON array, got %T", response["items"])
			}
			if len(items) != testCase.items {
				t.Errorf("items=%d want=%d", len(items), testCase.items)
			}
		})
	}
}
