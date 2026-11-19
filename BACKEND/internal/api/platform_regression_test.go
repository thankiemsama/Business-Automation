package api_test

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"businessautomation/internal/app"
	"businessautomation/internal/core"
	"businessautomation/internal/repository"
	"businessautomation/internal/service"
)

func TestEmailReplyReturnsAcceptedAfterQueueCommit(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "test-key")
	t.Setenv("EMAIL_FROM", "support@example.test")
	repo, err := repository.Open(filepath.Join(t.TempDir(), "platform.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	owner := root.account("email-owner@example.test")
	tenant := owner.call("GET", "/api/v1/auth/me", nil, 200)["tenant_id"].(string)
	const conversation = "email-conversation"
	if err := repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{ID: conversation, TenantID: tenant, Kind: core.KindConversations,
			Channel: core.ChannelEmail, Status: core.StatusOpen, Email: "customer@example.test"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	message := owner.call("POST", "/api/v1/conversations/"+conversation+"/messages",
		map[string]any{"sender": core.SenderAgent, "content": "Queued reply"}, http.StatusAccepted)
	if err := repo.View(func(store core.Reader) error {
		due := store.DueEmailReplies(time.Now(), 8)
		if message["status"] != "pending" || len(due) != 1 || due[0].ID != message["id"] {
			t.Errorf("accepted response did not match the persisted email queue")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPortalCodeAttachmentsAndCSAT(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "platform.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	owner := root.account("owner-regression@example.test")
	tenant := owner.call("GET", "/api/v1/auth/me", nil, 200)["tenant_id"].(string)
	customer := owner.call("POST", "/api/v1/customers", map[string]any{"name": "Khách thử", "email": "customer@example.test"}, 201)
	customerID := customer["id"].(string)
	hash := func(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }
	if err = repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{ID: hash("portal:" + tenant + ":customer@example.test"), Kind: core.KindPortalCodes, TenantID: tenant, CustomerID: customerID, Enabled: true, Content: hash("portal-code:abcd1234"), UpdatedAt: time.Now().Add(time.Hour)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	portal := "/api/v1/portal/" + tenant
	for _, code := range []string{"", "abcd", "abcd1235"} {
		root.call("POST", portal+"/login", map[string]any{"email": "customer@example.test", "code": code}, 401)
	}
	credentials := map[string]any{"email": "customer@example.test", "code": "ABCD1234"}
	login := root.call("POST", portal+"/login", credentials, 200)
	root.call("POST", portal+"/login", credentials, 401)
	guest := root
	guest.token = login["token"].(string)
	ticket := guest.call("POST", portal+"/tickets", map[string]any{"subject": "Hỗ trợ thử", "content": "Có ảnh", "attachment_name": "image.png", "attachment_data": "iVBORw0KGgo="}, 201)
	id := ticket["id"].(string)
	conversation := ticket["conversation_id"].(string)
	owner.call("POST", "/api/v1/conversations/"+conversation+"/messages", map[string]any{"sender": "note", "content": "confidential"}, 201)
	detail := guest.call("GET", portal+"/tickets/"+id, nil, 200)
	messages := detail["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["attachment_name"] != "image.png" {
		t.Fatal(detail)
	}
	other := owner.call("POST", "/api/v1/tickets", map[string]any{"subject": "Phiếu riêng"}, 201)
	guest.call("GET", portal+"/tickets/"+other["id"].(string), nil, 404)
	ticketPath := "/api/v1/tickets/" + id
	latest := owner.call("GET", ticketPath, nil, 200)
	owner.call("PUT", ticketPath, map[string]any{"subject": "Hỗ trợ thử", "conversation_id": conversation,
		"status": "resolved", "version": latest["version"]}, 200)
	surveys := service.NewCSAT(repo, service.SystemClock{}, service.RandomIDs{})
	link, err := surveys.IssueLink(core.Record{ID: id, TenantID: tenant, Subject: "Hỗ trợ thử"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/sat/" + tenant + "." + link.Token
	if survey := root.call("GET", path, nil, 200); survey["subject"] != "Hỗ trợ thử" || survey["rated"] != false {
		t.Fatal(survey)
	}
	root.call("POST", path, map[string]any{"rating": 5, "comment": "Rất tốt"}, 201)
	root.call("POST", path, map[string]any{"rating": 1}, 409)
	detail = guest.call("GET", portal+"/tickets/"+id, nil, 200)
	if rated := detail["ticket"].(map[string]any); rated["rating"] != float64(5) || rated["rating_comment"] != "Rất tốt" {
		t.Fatal(rated)
	}
}

func TestMessageEndpointRejectsAnOutdatedDraftSource(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "platform.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	owner := root.account("draft-owner@example.test")
	customer := owner.call("POST", "/api/v1/customers", map[string]any{"name": "Customer"}, http.StatusCreated)
	conversation := owner.call("POST", "/api/v1/conversations", map[string]any{"customer_id": customer["id"]}, http.StatusCreated)
	path := "/api/v1/conversations/" + conversation["id"].(string) + "/messages"
	first := owner.call("POST", path, map[string]any{"sender": core.SenderCustomer, "content": "First question"}, http.StatusCreated)
	second := owner.call("POST", path, map[string]any{"sender": core.SenderCustomer, "content": "New question"}, http.StatusCreated)
	input := map[string]any{"sender": core.SenderAgent, "content": "Draft reply", "source_message_id": first["id"]}
	owner.call("POST", path, input, http.StatusConflict)
	input["source_message_id"] = second["id"]
	owner.call("POST", path, input, http.StatusCreated)
}
