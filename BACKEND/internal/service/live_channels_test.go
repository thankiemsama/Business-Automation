package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
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

func TestLiveChannelInboundAndOutbox(t *testing.T) {
	const tenant = "workspace"
	t.Setenv("NEXA_CHANNEL_CREDENTIALS", `{"workspace":{
		"zalo":{"account_id":"111","app_id":"222","access_token":"zalo-token","webhook_secret":"zalo-secret"},
		"whatsapp":{"account_id":"333","access_token":"wa-token","webhook_secret":"wa-secret","verify_token":"verify","graph_version":"v25.0"}
	}}`)
	for _, tc := range []struct {
		channel, config, payload, endpoint, receipt string
	}{
		{core.ChannelZalo, `{"account_id":"111","app_id":"222"}`, `{"app_id":"222","event_name":"user_send_text","timestamp":"1713456789000","sender":{"id":"12345"},"recipient":{"id":"111"},"message":{"msg_id":"provider-id","text":"Xin chào"}}`,
			"https://openapi.zalo.me/v3.0/oa/message/cs", `{"error":0,"data":{"message_id":"receipt"}}`},
		{core.ChannelWhatsApp, `{"account_id":"333"}`, `{"object":"whatsapp_business_account","entry":[{"changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"333"},"messages":[{"from":"12345","id":"provider-id","type":"text","text":{"body":"Xin chào"}}]}}]}]}`,
			"https://graph.facebook.com/v25.0/333/messages", `{"messages":[{"id":"receipt"}]}`},
	} {
		t.Run(tc.channel, func(t *testing.T) {
			base, err := repository.Open(filepath.Join(t.TempDir(), "channels.json"))
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
			if _, err := manager.Configure(owner, tc.channel, core.Record{Enabled: true, Content: tc.config}); err != nil {
				t.Fatal(err)
			}
			headers := http.Header{}
			if err := manager.Receive(tenant, tc.channel, headers, []byte(tc.payload)); err == nil {
				t.Fatal("unsigned payload accepted")
			}
			if tc.channel == core.ChannelZalo {
				digest := sha256.Sum256([]byte("222" + tc.payload + "1713456789000" + "zalo-secret"))
				headers.Set("X-ZEvent-Signature", "mac="+hex.EncodeToString(digest[:]))
			} else {
				mac := hmac.New(sha256.New, []byte("wa-secret"))
				mac.Write([]byte(tc.payload))
				headers.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
				if !manager.VerifyChallenge(tenant, tc.channel, "subscribe", "verify") || manager.VerifyChallenge(tenant, tc.channel, "subscribe", "wrong") {
					t.Fatal("invalid verification behavior")
				}
			}
			for _, delivery := range []string{"d1", "d2"} {
				headers.Set("X-Nexa-Delivery", delivery)
				if err := manager.Receive(tenant, tc.channel, headers, []byte(tc.payload)); err != nil {
					t.Fatal(err)
				}
			}
			var conversationID string
			if err := workspace.View(func(store core.Reader) error {
				messages := store.Records(tenant, core.KindMessages)
				if len(messages) != 1 || messages[0].Content != "Xin chào" {
					t.Fatal(messages)
				}
				conversationID = messages[0].ConversationID
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			reply, err := conversations.Message(owner, conversationID, "Chào bạn", core.SenderAgent)
			if err != nil || reply.Status != "pending" {
				t.Fatal(reply, err)
			}
			calls := 0
			var delivery string
			provider := manager.Provider(tc.channel).(*liveChannel)
			provider.client.Transport = webhookTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				body, _ := io.ReadAll(req.Body)
				gotDelivery := req.Header.Get("X-Nexa-Delivery")
				if req.URL.String() != tc.endpoint || gotDelivery == "" || !strings.Contains(string(body), "12345") || !strings.Contains(string(body), "Chào bạn") {
					t.Fatalf("invalid provider request: %v %s", req.URL, body)
				}
				if delivery != "" && delivery != gotDelivery {
					t.Fatal("unstable delivery ID")
				}
				delivery = gotDelivery
				status := http.StatusOK
				if calls == 1 {
					status = http.StatusTooManyRequests
				} else if calls == 2 {
					status = http.StatusBadRequest
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(tc.receipt))}, nil
			})
			ctx := context.Background()
			manager.processChannelOutbox(ctx)
			manager.processChannelOutbox(ctx)
			if calls != 1 {
				t.Fatal("backoff ignored")
			}
			clock.now = clock.now.Add(time.Minute)
			manager.processChannelOutbox(ctx)
			jobs, err := manager.Deliveries(owner, tc.channel)
			if err != nil || len(jobs) != 1 || jobs[0].Status != "failed" || jobs[0].ID != delivery {
				t.Fatal(jobs, err)
			}
			if err := manager.Retry(owner, tc.channel, jobs[0].ID); err != nil {
				t.Fatal(err)
			}
			manager.processChannelOutbox(ctx)
			manager.processChannelOutbox(ctx)
			if err := workspace.View(func(store core.Reader) error {
				msg, exists := store.Record(tenant, reply.ID, core.KindMessages)
				if !exists || msg.Status != "sent" || msg.EmailProviderID != "receipt" || calls != 3 {
					t.Fatal(msg, calls)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
