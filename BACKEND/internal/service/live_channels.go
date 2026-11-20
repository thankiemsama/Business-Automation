package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"businessautomation/internal/core"
)

type channelAccount struct {
	AccountID string `json:"account_id"`
	AppID     string `json:"app_id,omitempty"`
}

type channelCredentials struct {
	AccountID     string `json:"account_id"`
	AppID         string `json:"app_id,omitempty"`
	AccessToken   string `json:"access_token"`
	WebhookSecret string `json:"webhook_secret"`
	VerifyToken   string `json:"verify_token"`
	GraphVersion  string `json:"graph_version"`
	TwilioSID     string `json:"twilio_sid"`
	CallbackURL   string `json:"callback_url"`
}

type liveChannel struct {
	name        string
	credentials map[string]channelCredentials
	client      *http.Client
}

var channelID = regexp.MustCompile(`^[0-9]{1,32}$`)
var graphVersion = regexp.MustCompile(`^v[0-9]{2}\.0$`)

func (p *liveChannel) Name() string { return p.name }

func parseChannelAccount(name, content string) (channelAccount, error) {
	var account channelAccount
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&account) != nil || decoder.Decode(new(any)) != io.EOF || !validChannelAddress(name, account.AccountID) ||
		(name == core.ChannelZalo && !channelID.MatchString(account.AppID)) {
		return channelAccount{}, core.Fail(400, "Nhập account_id dạng số, app_id cho Zalo OA hoặc số E.164 cho SMS")
	}
	return account, nil
}

func (p *liveChannel) Validate(_ core.Reader, config *core.ChannelConfig) error {
	if !config.Enabled && strings.TrimSpace(config.Content) == "" {
		return nil
	}
	_, err := parseChannelAccount(p.name, config.Content)
	return err
}

func (m *ChannelManager) registerLiveChannels() {
	var tenants map[string]map[string]channelCredentials
	if json.Unmarshal([]byte(os.Getenv("NEXA_CHANNEL_CREDENTIALS")), &tenants) != nil {
		tenants = nil
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	for _, name := range []string{core.ChannelZalo, core.ChannelWhatsApp, core.ChannelSMS} {
		provider := &liveChannel{name: name, credentials: map[string]channelCredentials{}, client: client}
		for tenant, channels := range tenants {
			credential := channels[name]
			if !validChannelAddress(name, credential.AccountID) || credential.WebhookSecret == "" || credential.AccessToken == "" ||
				(name == core.ChannelZalo && !channelID.MatchString(credential.AppID)) ||
				(name == core.ChannelWhatsApp && (credential.VerifyToken == "" || !graphVersion.MatchString(credential.GraphVersion))) ||
				(name == core.ChannelSMS && !validSMSCredentials(tenant, credential)) {
				continue
			}
			provider.credentials[tenant] = credential
		}
		m.Register(provider)
	}
}

func (m *ChannelManager) providerAccount(store core.Reader, tenant, channel string) (*liveChannel, channelAccount, channelCredentials, error) {
	provider, ok := m.Provider(channel).(*liveChannel)
	if !ok {
		return nil, channelAccount{}, channelCredentials{}, core.Fail(404, "Kênh chưa có adapter")
	}
	record, ok := store.Record(tenant, tokenHash("channel:"+tenant+":"+channel), core.KindChannelConfig)
	if !ok || !record.Enabled {
		return nil, channelAccount{}, channelCredentials{}, core.Fail(409, "Kênh chưa bật")
	}
	account, err := parseChannelAccount(channel, record.Content)
	if err != nil {
		return nil, channelAccount{}, channelCredentials{}, err
	}
	credentials, ok := provider.credentials[tenant]
	if !ok || credentials.AccountID != account.AccountID || (channel == core.ChannelZalo && credentials.AppID != account.AppID) {
		return nil, channelAccount{}, channelCredentials{}, core.Fail(503, "Thiếu thông tin kết nối trên máy chủ")
	}
	return provider, account, credentials, nil
}

func (m *ChannelManager) VerifyChallenge(tenant, channel, mode, candidate string) bool {
	var verified bool
	if channel != core.ChannelWhatsApp || mode != "subscribe" || candidate == "" {
		return false
	}
	err := viewTenant(m.repo, tenant, func(store core.Reader) error {
		_, _, credentials, err := m.providerAccount(store, tenant, channel)
		if err == nil {
			verified = hmac.Equal([]byte(credentials.VerifyToken), []byte(candidate))
		}
		return err
	})
	return err == nil && verified
}

type inboundChannelMessage struct {
	id, sender, text, attachment string
}

func validSignature(header, prefix string, digest []byte) bool {
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	supplied, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	return err == nil && hmac.Equal(supplied, digest)
}

func parseZalo(account channelAccount, secret string, header http.Header, raw []byte) ([]inboundChannelMessage, error) {
	var payload struct {
		AppID             string `json:"app_id"`
		Event             string `json:"event_name"`
		Timestamp         string `json:"timestamp"`
		Sender, Recipient struct{ ID string }
		Message           struct {
			ID          string `json:"msg_id"`
			Text        string
			Attachments []json.RawMessage
		}
	}
	invalid := core.Fail(401, "Webhook Zalo không hợp lệ")
	if json.Unmarshal(raw, &payload) != nil || payload.AppID != account.AppID || payload.Timestamp == "" {
		return nil, invalid
	}
	timestamp := header.Get("X-ZEvent-Timestamp")
	if timestamp == "" {
		timestamp = payload.Timestamp
	}
	sum := sha256.Sum256([]byte(account.AppID + string(raw) + timestamp + secret))
	signature := header.Get("X-ZEvent-Signature")
	if !validSignature(signature, "mac=", sum[:]) && !validSignature(signature, "", sum[:]) {
		return nil, invalid
	}
	switch payload.Event {
	case "user_send_text", "user_send_image", "user_send_file", "user_send_audio", "user_send_video", "user_send_sticker", "user_send_voice":
		if payload.Recipient.ID != account.AccountID {
			return nil, nil
		}
	default:
		return nil, nil
	}
	msg := inboundChannelMessage{id: payload.Message.ID, sender: payload.Sender.ID, text: payload.Message.Text}
	if payload.Event != "user_send_text" || len(payload.Message.Attachments) > 0 {
		msg.attachment = "Tệp đính kèm trên Zalo OA"
	}
	return []inboundChannelMessage{msg}, nil
}

func parseWhatsApp(account channelAccount, secret string, header http.Header, raw []byte) ([]inboundChannelMessage, error) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	if !validSignature(header.Get("X-Hub-Signature-256"), "sha256=", mac.Sum(nil)) {
		return nil, core.Fail(401, "Webhook WhatsApp không hợp lệ")
	}
	var payload struct {
		Object string
		Entry  []struct {
			Changes []struct {
				Field string
				Value struct {
					Metadata struct {
						PhoneID string `json:"phone_number_id"`
					}
					Messages []struct {
						ID, From, Type string
						Text           struct{ Body string }
					}
				}
			}
		}
	}
	if json.Unmarshal(raw, &payload) != nil || payload.Object != "whatsapp_business_account" {
		return nil, core.Fail(400, "Dữ liệu WhatsApp không hợp lệ")
	}
	var messages []inboundChannelMessage
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" || change.Value.Metadata.PhoneID != account.AccountID {
				continue
			}
			for _, original := range change.Value.Messages {
				if original.Type == "reaction" {
					continue
				}
				msg := inboundChannelMessage{id: original.ID, sender: original.From, text: original.Text.Body}
				if original.Type != "text" {
					msg.attachment = "Nội dung " + clipped(original.Type, 40) + " trên WhatsApp"
				}
				messages = append(messages, msg)
			}
		}
	}
	return messages, nil
}

func (m *ChannelManager) Receive(tenant, channel string, header http.Header, raw []byte) error {
	if m.conversations == nil {
		return core.Fail(503, "Kênh chưa sẵn sàng")
	}
	return updateTenant(m.repo, tenant, func(store core.Writer) error {
		_, account, credentials, err := m.providerAccount(store, tenant, channel)
		if err != nil {
			return err
		}
		parse := parseZalo
		if channel == core.ChannelWhatsApp {
			parse = parseWhatsApp
		}
		var messages []inboundChannelMessage
		if channel == core.ChannelSMS {
			messages, err = parseSMS(account, credentials, header, raw)
		} else {
			messages, err = parse(account, credentials.WebhookSecret, header, raw)
		}
		if err != nil {
			return err
		}
		for _, msg := range messages {
			if err := m.ingest(store, tenant, channel, account.AccountID, msg); err != nil {
				return err
			}
		}
		return nil
	})
}

func (m *ChannelManager) ingest(store core.Writer, tenant, channel, account string, msg inboundChannelMessage) error {
	msg.text = strings.TrimSpace(msg.text)
	if !validChannelAddress(channel, msg.sender) || msg.id == "" || len(msg.id) > 512 || (msg.text == "" && msg.attachment == "") {
		return core.Fail(400, "Tin nhắn ngoài không hợp lệ")
	}
	binding := tenant + ":" + channel + ":" + account + ":"
	delivery := tokenHash("channel-event:" + binding + msg.id)
	if _, seen := store.Record(tenant, delivery, "channel_inbound"); seen {
		return nil
	}
	now := m.conversations.clock.Now().UTC()
	user := core.User{TenantID: tenant}
	contact := tokenHash("channel-contact:" + binding + msg.sender)
	link, _ := store.Record(tenant, contact, "channel_contacts")
	customer, found := store.Record(tenant, link.CustomerID, core.KindCustomers)
	if !found {
		customer = core.Record{ID: m.conversations.ids.NewID(), TenantID: tenant, Kind: core.KindCustomers, Name: msg.sender, CreatedAt: now, UpdatedAt: now}
		store.PutRecord(customer)
	}
	conversation, found := store.Record(tenant, link.ConversationID, core.KindConversations)
	if found && m.acceptChannelRating(store, conversation, msg, now) {
		store.PutRecord(core.Record{ID: delivery, TenantID: tenant, Kind: "channel_inbound", ConversationID: conversation.ID, CreatedAt: now})
		return nil
	}
	if !found || conversation.Status == core.StatusClosed {
		conversation = core.Record{ID: m.conversations.ids.NewID(), TenantID: tenant, Kind: core.KindConversations, Channel: channel,
			CustomerID: customer.ID, Subject: strings.ToUpper(channel) + " · " + msg.sender, Status: core.StatusOpen, CreatedAt: now, UpdatedAt: now}
		store.PutRecord(conversation)
		store.PutRecord(core.Record{ID: contact, TenantID: tenant, Kind: "channel_contacts", CustomerID: customer.ID, ConversationID: conversation.ID})
		store.PutRecord(core.Record{ID: tokenHash("channel-thread:" + conversation.ID), TenantID: tenant, Kind: "channel_threads", Name: msg.sender, Content: account, Channel: channel, ConversationID: conversation.ID})
	}
	previous := latestPublicMessage(store, tenant, conversation.ID)
	result := m.conversations.storeMessage(store, user, conversation, clipped(msg.text, 4000), core.SenderCustomer)
	if msg.attachment != "" {
		result.AttachmentName = msg.attachment
		if result.Content == "" {
			result.Content = msg.attachment
		}
		store.PutRecord(result)
	}
	store.PutRecord(core.Record{ID: delivery, TenantID: tenant, Kind: "channel_inbound", ConversationID: conversation.ID, CreatedAt: now})
	if m.handleCampaignMessage(store, conversation, result, account) {
		result.AutomationID = "lead_campaign_command"
		store.PutRecord(result)
		return nil
	}
	if msg.attachment == "" && confirmBotResolution(store, conversation, previous, result) {
		m.queueChannelSurvey(store, conversation)
		return nil
	}
	if msg.attachment == "" && resolveOnCustomerConfirmation(store, conversation, previous, result) {
		return nil
	}
	return m.conversations.automations.Run(store, user, conversation, result)
}

type channelSendResult struct {
	receipt, failure string
	retry            bool
}

func (p *liveChannel) send(ctx context.Context, account channelAccount, creds channelCredentials, to, content, delivery string) channelSendResult {
	if p.name == core.ChannelSMS {
		return sendSMS(ctx, p.client, account, creds, to, content)
	}
	endpoint := "https://openapi.zalo.me/v3.0/oa/message/cs"
	payload := map[string]any{"recipient": map[string]string{"user_id": to}, "message": map[string]string{"text": content}}
	if p.name == core.ChannelWhatsApp {
		endpoint = "https://graph.facebook.com/" + creds.GraphVersion + "/" + account.AccountID + "/messages"
		payload = map[string]any{"messaging_product": "whatsapp", "to": to, "type": "text", "text": map[string]string{"body": content}, "biz_opaque_callback_data": delivery}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return channelSendResult{failure: "invalid_payload"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return channelSendResult{failure: "invalid_request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nexa-Delivery", delivery)
	if p.name == core.ChannelWhatsApp {
		req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	} else {
		req.Header.Set("access_token", creds.AccessToken)
	}
	response, err := p.client.Do(req)
	if err != nil {
		return channelSendResult{failure: "delivery_unknown"}
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusTooManyRequests:
		return channelSendResult{failure: "rate_limited", retry: true}
	case response.StatusCode >= 500:
		return channelSendResult{failure: "delivery_unknown"}
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return channelSendResult{failure: "provider_rejected"}
	}
	var res struct {
		Error *int
		Data  struct {
			ID string `json:"message_id"`
		}
		Messages []struct{ ID string }
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&res) != nil {
		return channelSendResult{failure: "delivery_unknown"}
	}
	if p.name == core.ChannelZalo {
		if res.Error != nil && *res.Error != 0 {
			return channelSendResult{failure: "provider_rejected"}
		}
		if res.Data.ID != "" {
			return channelSendResult{receipt: res.Data.ID}
		}
	} else if len(res.Messages) == 1 && res.Messages[0].ID != "" {
		return channelSendResult{receipt: res.Messages[0].ID}
	}
	return channelSendResult{failure: "delivery_unknown"}
}
