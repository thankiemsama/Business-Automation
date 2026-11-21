package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

// TestMessengerChannelInboundAndOutbound verifies the P2 Facebook Messenger
// live-channel adapter: webhook signature validation, message parsing, and
// outbound delivery through the Facebook Graph API.
func TestMessengerChannelInboundAndOutbound(t *testing.T) {
	const tenant = "workspace"
	t.Setenv("NEXA_CHANNEL_CREDENTIALS", `{"workspace":{
		"messenger":{"account_id":"555","access_token":"fb-token","webhook_secret":"fb-secret","verify_token":"verify","graph_version":"v25.0"}
	}}`)

	base, err := repository.Open(filepath.Join(t.TempDir(), "messenger.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	workspace := NewWorkspace(base, clock, RandomIDs{})
	conversations := NewConversations(workspace, clock, workspace.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(workspace.IDs, clock))
	manager := NewChannels(workspace)
	manager.ConfigureConversations(conversations)
	owner := core.User{ID: "owner", TenantID: tenant, Role: core.RoleOwner}
	if err := workspace.Update(func(store core.Writer) error { store.PutUser(owner); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Configure(owner, core.ChannelMessenger, core.Record{Enabled: true, Content: `{"account_id":"555"}`}); err != nil {
		t.Fatal(err)
	}

	// Build a realistic Messenger webhook payload (mid lives inside message).
	const payload = `{"object":"page","entry":[{"id":"555","time":1458790886423,"messaging":[{"sender":{"id":"666"},"recipient":{"id":"555"},"timestamp":1458790886423,"message":{"mid":"provider-mid","text":"Xin chào"}}]}]}`

	// Unsigned payload must be rejected.
	headers := http.Header{"Content-Type": {"application/json"}}
	if err := manager.Receive(tenant, core.ChannelMessenger, headers, []byte(payload)); err == nil {
		t.Fatal("unsigned Messenger payload accepted")
	}

	// Sign the payload: X-Hub-Signature = "sha1=" + hex(sha1(raw_body, webhook_secret)).
	mac := hmac.New(sha1.New, []byte("fb-secret"))
	mac.Write([]byte(payload))
	headers.Set("X-Hub-Signature", "sha1="+hex.EncodeToString(mac.Sum(nil)))

	if err := manager.Receive(tenant, core.ChannelMessenger, headers, []byte(payload)); err != nil {
		t.Fatal(err)
	}

	// Verify the conversation was created with the parsed message.
	var conversationID string
	if err := workspace.View(func(store core.Reader) error {
		messages := store.Records(tenant, core.KindMessages)
		if len(messages) != 1 || messages[0].Content != "Xin chào" {
			t.Fatalf("unexpected messages: %+v", messages)
		}
		conversationID = messages[0].ConversationID
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Agent reply should queue into the Messenger outbox.
	reply, err := conversations.Message(owner, conversationID, "Chào bạn", core.SenderAgent)
	if err != nil || reply.Status != "pending" {
		t.Fatalf("reply did not enter Messenger outbox: %+v %v", reply, err)
	}

	// Intercept the outbound Graph API call and verify the payload shape.
	calls := 0
	provider := manager.Provider(core.ChannelMessenger).(*liveChannel)
	provider.client.Transport = webhookTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(req.Body)
		if req.URL.String() != "https://graph.facebook.com/v25.0/555/messages" {
			t.Fatalf("unexpected endpoint: %s", req.URL)
		}
		if !strings.Contains(string(body), `"recipient":{"id":"666"}`) || !strings.Contains(string(body), `"text":"Chào bạn"`) {
			t.Fatalf("unexpected Messenger payload: %s", body)
		}
		if req.Header.Get("Authorization") != "Bearer fb-token" {
			t.Fatal("missing Bearer auth header")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"messages":[{"id":"receipt-mid"}]}`))}, nil
	})

	manager.processChannelOutbox(context.Background())

	if err := workspace.View(func(store core.Reader) error {
		msg, exists := store.Record(tenant, reply.ID, core.KindMessages)
		if !exists || msg.Status != "sent" || msg.EmailProviderID != "receipt-mid" {
			t.Fatalf("reply not sent: %+v", msg)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = calls
}

// TestMessengerAttachmentInbound verifies that an attachment message is parsed
// with the attachment marker attached.
func TestMessengerAttachmentInbound(t *testing.T) {
	const tenant = "workspace"
	t.Setenv("NEXA_CHANNEL_CREDENTIALS", `{"workspace":{
		"messenger":{"account_id":"555","access_token":"fb-token","webhook_secret":"fb-secret","verify_token":"verify","graph_version":"v25.0"}
	}}`)

	base, err := repository.Open(filepath.Join(t.TempDir(), "messenger-attach.json"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := NewWorkspace(base, &workspaceClock{now: time.Now().UTC()}, RandomIDs{})
	conversations := NewConversations(workspace, &workspaceClock{now: time.Now().UTC()}, workspace.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(workspace.IDs, &workspaceClock{now: time.Now().UTC()}))
	manager := NewChannels(workspace)
	manager.ConfigureConversations(conversations)
	owner := core.User{ID: "owner", TenantID: tenant, Role: core.RoleOwner}
	_ = workspace.Update(func(store core.Writer) error { store.PutUser(owner); return nil })
	_, _ = manager.Configure(owner, core.ChannelMessenger, core.Record{Enabled: true, Content: `{"account_id":"555"}`})

	const payload = `{"object":"page","entry":[{"id":"555","messaging":[{"sender":{"id":"777"},"recipient":{"id":"555"},"message":{"mid":"attach-mid","attachments":[{"type":"image","url":"https://example.com/img.jpg"}]}}]}]}`

	headers := http.Header{"Content-Type": {"application/json"}}
	mac := hmac.New(sha1.New, []byte("fb-secret"))
	mac.Write([]byte(payload))
	headers.Set("X-Hub-Signature", "sha1="+hex.EncodeToString(mac.Sum(nil)))

	if err := manager.Receive(tenant, core.ChannelMessenger, headers, []byte(payload)); err != nil {
		t.Fatal(err)
	}

	if err := workspace.View(func(store core.Reader) error {
		msgs := store.Records(tenant, core.KindMessages)
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if !strings.Contains(msgs[0].AttachmentName, "image") {
			t.Fatalf("attachment marker missing: %s", msgs[0].AttachmentName)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
