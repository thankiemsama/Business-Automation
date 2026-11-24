package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"businessautomation/internal/core"
)

type EmailMailer struct {
	key, from, replyTo string
	client             *http.Client
	devSink            bool // dev-only: log mail to stdout instead of Resend
}

func EmailFromEnvironment() *EmailMailer {
	key, from := os.Getenv("RESEND_API_KEY"), os.Getenv("EMAIL_FROM")
	if key == "" || from == "" {
		// Dev-only sink: with NEXA_DEV_EMAIL_SINK=true, emails are written to
		// the backend log instead of failing. G5 guard: a public https origin
		// means production, so the sink is refused to keep reset/verify links
		// out of container logs. Configure RESEND_API_KEY instead.
		if os.Getenv("NEXA_DEV_EMAIL_SINK") == "true" {
			if strings.HasPrefix(strings.TrimRight(os.Getenv("APP_ORIGIN"), "/"), "https://") {
				slog.Warn("NEXA_DEV_EMAIL_SINK Ä‘Æ°á»£c bá» qua: APP_ORIGIN lÃ  production (https), cáº§n cáº¥u hÃ¬nh RESEND_API_KEY.")
				return nil
			}
			if from == "" {
				from = "dev@nexa.local"
			}
			return &EmailMailer{from: from, devSink: true}
		}
		return nil
	}
	return &EmailMailer{key: key, from: from, replyTo: os.Getenv("SUPPORT_REPLY_TO"), client: &http.Client{Timeout: 10 * time.Second}}
}

// IsDevSink reports whether this mailer only writes to the backend log.
func (m *EmailMailer) IsDevSink() bool { return m != nil && m.devSink }

func (m *EmailMailer) Send(to, subject, content, key, name, attachment string) error {
	_, _, err := m.send(context.Background(), to, subject, content, key, name, attachment)
	return err
}

func (m *EmailMailer) send(ctx context.Context, to, subject, content, key, name, attachment string, replyTo ...string) (string, int, error) {
	if m == nil {
		return "", 0, core.Fail(503, "ChÆ°a cáº¥u hÃ¬nh gá»­i email")
	}
	if m.devSink {
		slog.Info("dev email sink", "to", to, "subject", subject, "idempotency", key)
		for _, line := range stringsLines(content) {
			slog.Info("dev email sink: body", "line", line)
		}
		return "dev-" + key, 200, nil
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
		return "", 0, core.Fail(502, "KhÃ´ng káº¿t ná»‘i Ä‘Æ°á»£c dá»‹ch vá»¥ email")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", response.StatusCode, core.Fail(502, "Dá»‹ch vá»¥ email tá»« chá»‘i yÃªu cáº§u")
	}
	var receipt struct {
		ID string `json:"id"`
	}
	// Existing mail callers only need the HTTP outcome. Replies also reconcile by receipt ID.
	_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&receipt)
	return receipt.ID, response.StatusCode, nil
}

// stringsLines yields one entry per physical line of a mail body.
func stringsLines(content string) []string {
	return strings.Split(content, "\n")
}
