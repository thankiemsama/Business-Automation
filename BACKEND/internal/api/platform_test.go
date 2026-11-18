package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"businessautomation/internal/app"
	"businessautomation/internal/repository"
)

// request builds an HTTP request carrying an optional Bearer token.
func request(method, path, token string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(nil))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// TestPlatformFeatures exercises the differentiation features: public knowledge
// base, Customer 360° profile, multi-channel config, outgoing webhooks, and
// public API keys, plus the customer portal auth contract.
func TestPlatformFeatures(t *testing.T) {
	repo, err := repository.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	owner := root.account("platform@example.com")
	tenant := owner.call("GET", "/api/v1/auth/me", nil, 200)["tenant_id"].(string)

	// ---- Feature 9: public knowledge base with SEO slugs ----
	article := owner.call("POST", "/api/v1/knowledge", map[string]any{
		"subject": "Chính sách đổi trả", "content": "Đổi trả trong 7 ngày.", "enabled": true, "public": true,
	}, 201)
	if article["slug"] == nil || article["slug"].(string) != "chinh-sach-doi-tra" {
		t.Fatalf("knowledge slug not ASCII-folded: %v", article["slug"])
	}
	// Public (unauthenticated) knowledge read must work.
	owner.call("GET", "/api/v1/kb/"+tenant, nil, 200)
	owner.call("GET", "/api/v1/kb/"+tenant+"/chinh-sach-doi-tra", nil, 200)

	// ---- Feature 12: Customer 360° profile ----
	customer := owner.call("POST", "/api/v1/customers", map[string]any{"name": "Khách VIP", "email": "vip@example.com", "phone": "0901234567", "tags": []string{"VIP"}}, 201)
	customerID := customer["id"].(string)
	conv := owner.call("POST", "/api/v1/conversations", map[string]any{"customer_id": customerID, "channel": "web"}, 201)
	owner.call("POST", "/api/v1/tickets", map[string]any{"subject": "Vấn đề A", "customer_id": customerID, "conversation_id": conv["id"].(string)}, 201)
	note := owner.call("POST", "/api/v1/customers/"+customerID+"/notes", map[string]any{"content": "Khách quan trọng"}, 201)
	if note["content"].(string) != "Khách quan trọng" {
		t.Fatalf("note not persisted: %v", note)
	}
	profile := owner.call("GET", "/api/v1/customers/"+customerID+"/profile", nil, 200)
	if profile["total_tickets"] != float64(1) {
		t.Fatalf("profile ticket count wrong: %v", profile["total_tickets"])
	}
	if len(profile["timeline"].([]any)) != 2 || len(profile["tags"].([]any)) != 1 {
		t.Fatalf("profile timeline or tags missing: %v", profile)
	}
	if profile["customer"].(map[string]any)["id"] != customerID {
		t.Fatalf("profile did not embed the customer")
	}

	// ---- Feature 13: multi-channel configuration ----
	owner.call("POST", "/api/v1/channels/zalo", map[string]any{"name": "Zalo OA", "content": "abc", "enabled": true}, 201)
	status := owner.call("GET", "/api/v1/channels", nil, 200)
	if status["zalo"] == nil {
		t.Fatalf("channel status missing zalo: %v", status)
	}
	owner.call("POST", "/api/v1/channels/carrier-pigeon", map[string]any{"name": "X"}, 400)

	// ---- Feature 14: outgoing webhooks ----
	hook := owner.call("POST", "/api/v1/webhooks", map[string]any{
		"name": "n8n ticket", "url": "https://n8n.example.com/hooks/x", "events": []string{"ticket.created"},
	}, 201)
	if hook["secret"] == nil || hook["secret"].(string) == "" {
		t.Fatalf("webhook secret not generated")
	}
	hookID := hook["id"].(string)
	owner.call("POST", "/api/v1/webhooks/"+hookID, map[string]any{"name": "renamed", "url": "https://n8n.example.com/y", "events": []string{"message.new"}}, 200)
	owner.call("POST", "/api/v1/webhooks/"+hookID+"/remove", nil, 200)
	owner.call("POST", "/api/v1/webhooks", map[string]any{"name": "bad", "url": "ftp://nope"}, 400)

	// ---- Feature 15: public API keys ----
	key := owner.call("POST", "/api/v1/api-keys", map[string]any{"name": "External integration", "scope": "read", "rate_per_min": 120}, 201)
	if key["secret"] == nil || key["secret"].(string) == "" {
		t.Fatalf("api key secret not returned once")
	}
	owner.call("GET", "/api/v1/api-keys", nil, 200)
	owner.call("POST", "/api/v1/api-keys/"+key["id"].(string)+"/revoke", nil, 200)

	// ---- Agent is denied owner-only platform operations ----
	owner.call("POST", "/api/v1/team", map[string]any{"name": "Agent", "email": "agent-pf@example.com", "password": "a-strong-agent-password"}, 201)
	login := root.call("POST", "/api/v1/auth/login", map[string]any{"email": "agent-pf@example.com", "password": "a-strong-agent-password"}, 200)
	agent := root
	agent.token = login["access_token"].(string)
	agent.call("POST", "/api/v1/webhooks", map[string]any{"name": "x", "url": "https://x.example"}, 403)
	agent.call("POST", "/api/v1/api-keys", map[string]any{"name": "x"}, 403)
	agent.call("POST", "/api/v1/channels/zalo", map[string]any{"name": "x"}, 403)

	// ---- Feature 10: portal auth contract ----
	// Without a mailer configured, requesting a code degrades to 503.
	owner.call("POST", "/api/v1/portal/"+tenant+"/code", map[string]any{"email": "vip@example.com"}, 503)
	owner.call("POST", "/api/v1/portal/"+tenant+"/login", map[string]any{"email": "vip@example.com", "code": "AAAA"}, 401)

	// ---- Feature 15: public API key auth ----
	workingKey := owner.call("POST", "/api/v1/api-keys", map[string]any{"name": "public read", "scope": "read", "rate_per_min": 1}, 201)
	secret := workingKey["secret"].(string)
	w := httptest.NewRecorder()
	owner.h.ServeHTTP(w, request("GET", "/api/public/v1/tickets", "nexa_"+secret))
	if w.Code != 200 {
		t.Fatalf("public API with valid key: status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	owner.h.ServeHTTP(w, request("GET", "/api/public/v1/tickets", "nexa_"+secret))
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	owner.h.ServeHTTP(w, request("GET", "/api/public/v1/tickets", "nexa_does-not-exist"))
	if w.Code != 401 {
		t.Fatalf("public API with bad key: status=%d", w.Code)
	}
	// OpenAPI document is served without auth.
	w = httptest.NewRecorder()
	owner.h.ServeHTTP(w, request("GET", "/api/public/openapi.json", ""))
	if w.Code != 200 {
		t.Fatalf("openapi spec: status=%d", w.Code)
	}
}
