package api_test

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"businessautomation/internal/app"
	"businessautomation/internal/repository"
)

func TestChatAssignmentAndReadState(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "ops.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	owner := root.account("ops-owner@example.test")
	me := owner.call("GET", "/api/v1/auth/me", nil, 200)
	path := "/api/v1/chat/" + me["tenant_id"].(string)
	session := root.call("POST", path, map[string]any{"name": "Khách", "content": "Cần trợ giúp"}, 201)
	guest := root
	guest.token = session["token"].(string)
	guest.call("GET", path, nil, 200)
	list := owner.call("GET", "/api/v1/conversations?assignment=unassigned", nil, 200)
	conversation := list["items"].([]any)[0].(map[string]any)
	if conversation["unread"] != true {
		t.Fatal("expected unread", conversation)
	}
	base := "/api/v1/conversations/" + conversation["id"].(string)
	assignment := owner.call("POST", base+"/assignment", map[string]any{"claim": true}, 200)
	if assignment["assignee_id"] != me["id"] {
		t.Fatal("claim was not persisted", assignment)
	}
	owner.call("POST", base+"/read", map[string]any{"sequence": 1}, 200)
	if read := owner.call("GET", base, nil, 200); read["unread"] == true {
		t.Fatal("conversation is still unread", read)
	}
	guest.call("POST", path, map[string]any{"content": "Tin mới"}, 200)
	if unread := owner.call("GET", base, nil, 200); unread["unread"] != true {
		t.Fatal("new customer message did not mark unread", unread)
	}
	owner.call("POST", base+"/messages", map[string]any{"content": "Ghi chú riêng", "sender": "note"}, 201)
	request := httptest.NewRequest("GET", path, nil)
	request.Header.Set("Authorization", "Bearer "+guest.token)
	response := httptest.NewRecorder()
	root.h.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "Tin mới") || strings.Contains(response.Body.String(), "Ghi chú riêng") {
		t.Fatal("invalid guest transcript", response.Body.String())
	}
	guest.call("POST", path, map[string]any{"rating": 5}, 409)
	owner.call("PUT", base, map[string]any{"customer_id": conversation["customer_id"], "status": "closed"}, 200)
	guest.call("POST", path, map[string]any{"rating": 5}, 200)
	guest.call("POST", path, map[string]any{"rating": 5}, 409)
}
