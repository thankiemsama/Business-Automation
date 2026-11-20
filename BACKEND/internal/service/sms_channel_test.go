package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

func TestSMSChannelSignedInboundAndOutbound(t *testing.T) {
	account := channelAccount{AccountID: "+12025550123"}
	credentials := channelCredentials{
		AccountID: account.AccountID,
		TwilioSID: "AC" + strings.Repeat("a", 32), AccessToken: "auth-token",
		WebhookSecret: "auth-token", CallbackURL: "https://example.com/api/v1/channel-webhooks/tenant/sms",
	}
	sender := "+12025550456"
	messageID := "SM" + strings.Repeat("b", 32)
	form := url.Values{"AccountSid": {credentials.TwilioSID}, "Body": {"Xin chào"}, "From": {sender}, "MessageSid": {messageID}, "To": {account.AccountID}}
	mac := hmac.New(sha1.New, []byte(credentials.WebhookSecret))
	_, _ = io.WriteString(mac, credentials.CallbackURL+"AccountSid"+credentials.TwilioSID+"BodyXin chàoFrom"+sender+"MessageSid"+messageID+"To"+account.AccountID)
	headers := http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}
	if _, err := parseSMS(account, credentials, headers, []byte(form.Encode())); err == nil {
		t.Fatal("unsigned inbound SMS accepted")
	}
	headers.Set("X-Twilio-Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	messages, err := parseSMS(account, credentials, headers, []byte(form.Encode()))
	if err != nil || len(messages) != 1 || messages[0].id != messageID || messages[0].sender != sender || messages[0].text != "Xin chào" {
		t.Fatalf("unexpected inbound SMS: %+v %v", messages, err)
	}
	if _, err := parseSMS(channelAccount{AccountID: sender}, credentials, headers, []byte(form.Encode())); err == nil {
		t.Fatal("SMS accepted on the wrong phone number")
	}
	client := &http.Client{Transport: webhookTransport(func(req *http.Request) (*http.Response, error) {
		user, password, ok := req.BasicAuth()
		if !ok || user != credentials.TwilioSID || password != credentials.AccessToken ||
			req.URL.Host != "api.twilio.com" || req.ParseForm() != nil || req.Form.Get("To") != sender ||
			req.Form.Get("From") != account.AccountID || req.Form.Get("Body") != "Chào bạn" {
			t.Fatal("invalid Twilio provider request")
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"sid":"` + messageID + `","status":"queued"}`))}, nil
	})}
	if result := sendSMS(context.Background(), client, account, credentials, sender, "Chào bạn"); result.receipt != messageID {
		t.Fatalf("unexpected send outcome: %+v", result)
	}
	base, err := repository.Open(filepath.Join(t.TempDir(), "sms.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	workspace := NewWorkspace(base, clock, RandomIDs{})
	conversations := NewConversations(workspace, clock, workspace.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(workspace.IDs, clock))
	config, err := json.Marshal(map[string]map[string]channelCredentials{"tenant": {core.ChannelSMS: credentials}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEXA_CHANNEL_CREDENTIALS", string(config))
	manager := NewChannels(workspace)
	manager.ConfigureConversations(conversations)
	provider := manager.Provider(core.ChannelSMS).(*liveChannel)
	provider.client = client
	owner := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	if err := workspace.Update(func(store core.Writer) error { store.PutUser(owner); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Configure(owner, core.ChannelSMS, core.Record{Enabled: true, Content: `{"account_id":"+12025550123"}`}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := manager.Receive(owner.TenantID, core.ChannelSMS, headers, []byte(form.Encode())); err != nil {
			t.Fatal(err)
		}
	}
	var threadID string
	if err := workspace.ViewTenant(owner.TenantID, func(store core.Reader) error {
		inbound := store.Records(owner.TenantID, core.KindMessages)
		if len(inbound) != 1 || inbound[0].Content != "Xin chào" {
			t.Fatalf("unexpected inbound records: %+v", inbound)
		}
		threadID = inbound[0].ConversationID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	reply, err := conversations.Message(owner, threadID, "Chào bạn", core.SenderAgent)
	if err != nil || reply.Status != "pending" {
		t.Fatalf("reply did not enter SMS outbox: %+v %v", reply, err)
	}
	manager.processChannelOutbox(context.Background())
	if err := workspace.ViewTenant(owner.TenantID, func(store core.Reader) error {
		message, found := store.Record(owner.TenantID, reply.ID, core.KindMessages)
		if !found || message.Status != "sent" {
			t.Fatalf("reply was not sent through SMS outbox: %+v", message)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
