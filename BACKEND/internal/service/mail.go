package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"businessautomation/internal/core"
)

type EmailMailer struct {
	key, from, replyTo string
	client             *http.Client
}

func EmailFromEnvironment() *EmailMailer {
	key, from := os.Getenv("RESEND_API_KEY"), os.Getenv("EMAIL_FROM")
	if key == "" || from == "" {
		return nil
	}
	return &EmailMailer{key: key, from: from, replyTo: os.Getenv("SUPPORT_REPLY_TO"), client: &http.Client{Timeout: 10 * time.Second}}
}

func (m *EmailMailer) Send(to, subject, content, key, name, attachment string) error {
	if m == nil {
		return core.Fail(503, "Chưa cấu hình gửi email")
	}
	input := map[string]any{"from": m.from, "to": []string{to}, "subject": subject, "text": content}
	if m.replyTo != "" {
		input["reply_to"] = m.replyTo
	}
	if attachment != "" {
		input["attachments"] = []map[string]string{{"filename": name, "content": attachment}}
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	response, err := m.client.Do(req)
	if err != nil {
		return core.Fail(502, "Không kết nối được dịch vụ email")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return core.Fail(502, "Dịch vụ email từ chối yêu cầu")
	}
	return nil
}
