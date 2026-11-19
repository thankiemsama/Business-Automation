package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"businessautomation/internal/core"
)

type ResendInbound struct {
	secret       []byte
	key, address string
	client       *http.Client
}

var resendEmailID = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

func ResendInboundFromEnvironment() *ResendInbound {
	encoded := os.Getenv("RESEND_INBOUND_WEBHOOK_SECRET")
	key := os.Getenv("RESEND_INBOUND_API_KEY")
	address, addressErr := mail.ParseAddress(os.Getenv("RESEND_INBOUND_ADDRESS"))
	secret, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "whsec_"))
	if err != nil || len(secret) < 16 || key == "" || addressErr != nil {
		return nil
	}
	return &ResendInbound{secret: secret, key: key, address: address.Address, client: &http.Client{
		Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (s *ResendInbound) Receive(ctx context.Context, header http.Header, raw []byte, now time.Time) (*InboundEmail, error) {
	eventID := header.Get("Svix-Id")
	timestamp := header.Get("Svix-Timestamp")
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	age := now.Sub(time.Unix(unix, 0))
	invalid := core.Fail(401, "Chữ ký Resend không hợp lệ")
	if err != nil || eventID == "" || len(eventID) > 512 || age > 5*time.Minute || age < -5*time.Minute {
		return nil, invalid
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(eventID + "." + timestamp + "."))
	mac.Write(raw)
	digest := mac.Sum(nil)
	valid := false
	for _, signature := range strings.Fields(header.Get("Svix-Signature")) {
		version, encoded, ok := strings.Cut(signature, ",")
		if version != "v1" || !ok {
			continue
		}
		if supplied, err := base64.StdEncoding.DecodeString(encoded); err == nil && hmac.Equal(supplied, digest) {
			valid = true
			break
		}
	}
	if !valid {
		return nil, invalid
	}
	var event struct {
		Type string
		Data struct {
			ID string `json:"email_id"`
			To []string
		}
	}
	if json.Unmarshal(raw, &event) != nil {
		return nil, core.Fail(400, "Dữ liệu Resend không hợp lệ")
	}
	if event.Type != "email.received" || !hasEmailAddress(event.Data.To, s.address) {
		return nil, nil
	}
	if !resendEmailID.MatchString(event.Data.ID) {
		return nil, core.Fail(400, "Mã email Resend không hợp lệ")
	}
	return s.retrieve(ctx, event.Data.ID, eventID)
}

func hasEmailAddress(recipients []string, address string) bool {
	for _, recipient := range recipients {
		if parsed, err := mail.ParseAddress(recipient); err == nil && strings.EqualFold(parsed.Address, address) {
			return true
		}
	}
	return false
}

func (s *ResendInbound) retrieve(ctx context.Context, id, eventID string) (*InboundEmail, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.resend.com/emails/receiving/"+id, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	failed := core.Fail(502, "Không đọc được email nhận từ Resend")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, failed
	}
	defer response.Body.Close()
	var email struct {
		ID, From, Subject, Text string
		To                      []string
		MessageID               string `json:"message_id"`
		Headers                 map[string]string
		Attachments             []json.RawMessage
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&email) != nil ||
		email.ID != id || !hasEmailAddress(email.To, s.address) {
		return nil, failed
	}
	text := strings.TrimSpace(email.Text)
	if text == "" {
		text = "Email không có phần văn bản thuần. Xem thư gốc trong Resend."
	}
	if len(email.Attachments) > 0 {
		text = clipped(text, 3900) + "\n\nEmail có tệp đính kèm. Xem tệp trong Resend."
	}
	in := &InboundEmail{ID: id, DeliveryID: eventID, From: email.From, Subject: clipped(email.Subject, 100), Text: clipped(text, 4000)}
	in.MessageID = emailMessageID(email.MessageID)
	var inReplyTo, references string
	for key, value := range email.Headers {
		switch strings.ToLower(key) {
		case "in-reply-to":
			inReplyTo = emailMessageID(value)
		case "references":
			parts := strings.Fields(value)
			if len(parts) > 0 {
				references = emailMessageID(parts[0])
			}
		}
	}
	in.ThreadID = references
	if in.ThreadID == "" {
		in.ThreadID = inReplyTo
	}
	if in.ThreadID == "" {
		in.ThreadID = in.MessageID
	}
	return in, nil
}

func emailMessageID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 3 || len(value) > 512 || value[0] != '<' || value[len(value)-1] != '>' {
		return ""
	}
	for _, char := range value[1 : len(value)-1] {
		if char <= ' ' || char >= 127 || char == '<' || char == '>' {
			return ""
		}
	}
	return value
}
