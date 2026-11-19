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

func TestEmailReplyRetriesAfterRestartAndSharesClaimWithManualDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	repo, err := repository.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	user := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	ids := RandomIDs{}
	workspace := NewWorkspace(repo, clock, ids)
	const conversationID = "conversation"
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(user)
		store.PutRecord(core.Record{ID: conversationID, TenantID: user.TenantID, Kind: core.KindConversations,
			Channel: core.ChannelEmail, Email: "customer@example.test", Status: core.StatusOpen})
		store.PutRecord(core.Record{ID: "ticket", TenantID: user.TenantID, Kind: core.KindTickets,
			ConversationID: conversationID, Status: core.StatusOpen})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var sentID string
	sending, release := make(chan struct{}), make(chan struct{})
	calls := 0
	mailer := &EmailMailer{from: "support@example.test", client: &http.Client{
		Transport: webhookTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Header.Get("Idempotency-Key") != sentID {
				t.Error("reply did not reuse its provider key")
			}
			if calls == 1 {
				return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			}
			if calls == 2 {
				close(sending)
				select {
				case <-release:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
			} else {
				t.Error("claimed reply sent more than once")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"resend-123"}`))}, nil
		}),
	}}
	svc := NewConversations(workspace, clock, ids, nil, nil)
	svc.ConfigureEmail(mailer)
	message, err := svc.Message(user, conversationID, "Support reply", core.SenderAgent)
	if err != nil || message.Status != "pending" || calls != 0 {
		t.Fatalf("message was not queued: status=%s calls=%d err=%v", message.Status, calls, err)
	}
	sentID = message.ID
	ctx := context.Background()
	svc.processEmailReplies(ctx)

	reopened, err := repository.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	workspace = NewWorkspace(reopened, clock, ids)
	svc = NewConversations(workspace, clock, ids, nil, nil)
	svc.ConfigureEmail(mailer)
	pending, err := svc.DeliverEmail(ctx, user, conversationID, sentID)
	if err != nil || pending.EmailLastError != "provider_http_503" || pending.EmailRetryAt == nil || calls != 1 {
		t.Fatalf("persisted backoff was not honored: %+v, calls=%d, err=%v", pending, calls, err)
	}

	clock.now = *pending.EmailRetryAt
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.processEmailReplies(ctx)
	}()
	select {
	case <-sending:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not send the due reply")
	}
	claimed, err := svc.DeliverEmail(ctx, user, conversationID, sentID)
	close(release)
	<-done
	if err != nil || claimed.Status != "pending" || calls != 2 {
		t.Fatalf("manual delivery did not honor the lease: status=%s calls=%d err=%v", claimed.Status, calls, err)
	}
	if err := workspace.View(func(store core.Reader) error {
		sent, _ := store.Record(user.TenantID, sentID, core.KindMessages)
		ticket, _ := store.Record(user.TenantID, "ticket", core.KindTickets)
		if sent.Status != "sent" || sent.EmailProviderID != "resend-123" || sent.EmailRetryAt != nil ||
			ticket.FirstResponseAt == nil || ticket.RespondedBy != user.ID {
			t.Error("provider receipt and ticket response were not finalized together")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	expired, err := svc.Message(user, conversationID, "Old reply", core.SenderAgent)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(emailRetryWindow)
	svc.processEmailReplies(ctx)
	failed, err := svc.DeliverEmail(ctx, user, conversationID, expired.ID)
	notices, noticeErr := workspace.Notifications(user)
	if err != nil || noticeErr != nil || failed.Status != "failed" || calls != 2 ||
		len(notices) != 1 || notices[0].Status != "email_failed" {
		t.Fatalf("expired message should alert once and stop retrying: status=%s calls=%d err=%v notices=%v", failed.Status, calls, err, notices)
	}
}
