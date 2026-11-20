package service

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

type webhookTransport func(*http.Request) (*http.Response, error)

func (send webhookTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return send(request)
}

func TestWebhookRetriesNonSuccessAndKeepsSignature(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "webhooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	ids := RandomIDs{}
	workspace := NewWorkspace(repo, clock, ids)
	service := NewWebhooks(workspace, clock, ids)
	owner := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	hook, err := service.Save(owner, "", Webhook{Name: "Delivery", URL: "https://receiver.example.com", Events: []string{core.EventTicketCreated}})
	if err != nil {
		t.Fatal(err)
	}
	attempts := 0
	deliveryID := ""
	service.client.Transport = webhookTransport(func(request *http.Request) (*http.Response, error) {
		attempts++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if request.Header.Get("X-Nexa-Signature") != signWebhook(hook.Secret, string(body)) {
			t.Fatal("invalid webhook signature")
		}
		if attempts == 1 {
			deliveryID = request.Header.Get("X-Nexa-Delivery")
		}
		if request.Header.Get("X-Nexa-Delivery") != deliveryID {
			t.Fatal("delivery id changed on retry")
		}
		status := http.StatusServiceUnavailable
		if attempts > 8 {
			status = http.StatusNoContent
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(owner)
		store.PutRecord(core.Record{ID: ids.NewID(), TenantID: owner.TenantID, Kind: core.KindTickets})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service.dispatch(context.Background())
	service.dispatch(context.Background())
	if attempts != 1 {
		t.Fatal("retry ignored backoff", attempts)
	}
	for range 7 {
		clock.now = clock.now.Add(65 * time.Minute)
		service.dispatch(context.Background())
	}
	letters, err := service.DeadLetters(owner)
	if err != nil || len(letters) != 1 || letters[0].ID != deliveryID || letters[0].Attempts != 8 || !letters[0].Replayable {
		t.Fatalf("expected exhausted delivery in DLQ, got %+v, %v", letters, err)
	}
	otherOwner := owner
	otherOwner.TenantID = "other"
	if err := service.Replay(otherOwner, deliveryID); err == nil {
		t.Fatal("cross-tenant replay succeeded")
	}
	if err := service.Replay(owner, deliveryID); err != nil {
		t.Fatal(err)
	}
	if err := service.Replay(owner, deliveryID); err == nil {
		t.Fatal("duplicate replay succeeded")
	}
	service.dispatch(context.Background())
	service.dispatch(context.Background())
	if attempts != 9 {
		t.Fatal("delivery did not terminate on success", attempts)
	}
	letters, err = service.DeadLetters(owner)
	if err != nil || len(letters) != 0 {
		t.Fatalf("expected empty DLQ after replay, got %+v, %v", letters, err)
	}

	if err := repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{
			ID: "expired-delivery", TenantID: owner.TenantID, ConversationID: hook.ID,
			Kind: core.KindWebhookDeliveries, Status: "failed", Content: "retained payload",
			UpdatedAt: clock.now.Add(-webhookFailureRetention),
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Replay(owner, "expired-delivery"); err == nil {
		t.Fatal("expired delivery was replayable")
	}
	letters, err = service.DeadLetters(owner)
	if err != nil || len(letters) != 0 {
		t.Fatalf("expired delivery was visible: %+v, %v", letters, err)
	}
}
