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
			return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		if request.Header.Get("X-Nexa-Delivery") != deliveryID {
			t.Fatal("delivery id changed on retry")
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
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
	clock.now = clock.now.Add(3 * time.Minute)
	service.dispatch(context.Background())
	service.dispatch(context.Background())
	if attempts != 2 {
		t.Fatal("delivery did not terminate on success", attempts)
	}
}
