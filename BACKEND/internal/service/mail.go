package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	_, _, err := m.send(context.Background(), to, subject, content, key, name, attachment)
	return err
}

func (m *EmailMailer) send(ctx context.Context, to, subject, content, key, name, attachment string, replyTo ...string) (string, int, error) {
	if m == nil {
		return "", 0, core.Fail(503, "Chưa cấu hình gửi email")
	}
	input := map[string]any{"from": m.from, "to": []string{to}, "subject": subject, "text": content}
	if m.replyTo != "" {
		input["reply_to"] = m.replyTo
	}
	if len(replyTo) > 0 && emailMessageID(replyTo[0]) != "" {
		references := replyTo[0]
		if len(replyTo) > 1 && emailMessageID(replyTo[1]) != "" && replyTo[1] != replyTo[0] {
			references = replyTo[1] + " " + references
		}
		input["headers"] = map[string]string{"In-Reply-To": replyTo[0], "References": references}
	}
	if attachment != "" {
		input["attachments"] = []map[string]string{{"filename": name, "content": attachment}}
	}
	body, err := json.Marshal(input)
	if err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+m.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	response, err := m.client.Do(req)
	if err != nil {
		return "", 0, core.Fail(502, "Không kết nối được dịch vụ email")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", response.StatusCode, core.Fail(502, "Dịch vụ email từ chối yêu cầu")
	}
	var receipt struct {
		ID string `json:"id"`
	}
	// Existing mail callers only need the HTTP outcome. Replies also reconcile by receipt ID.
	_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&receipt)
	return receipt.ID, response.StatusCode, nil
}
