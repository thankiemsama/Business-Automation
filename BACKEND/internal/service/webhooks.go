package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"businessautomation/internal/core"
)

// WebhookService manages outgoing webhooks: CRUD plus an outbox that a background
// dispatcher drains. Payloads are signed with HMAC-SHA256 so consumers
// (n8n/Zapier/custom) can verify authenticity.
type WebhookService struct {
	repo   core.Repository
	clock  Clock
	ids    IDGenerator
	client *http.Client
}

func NewWebhooks(repo core.Repository, clock Clock, ids IDGenerator) *WebhookService {
	return &WebhookService{
		repo:   repo,
		clock:  clock,
		ids:    ids,
		client: &http.Client{Timeout: 6 * time.Second, Transport: &http.Transport{DialContext: dialWebhook}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

// Webhook is the stored shape of a webhook subscription.
type Webhook struct {
	ID     string   `json:"id,omitempty"`
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret,omitempty"`
}

const maximumWebhookURLBytes = 2048
const maximumWebhooksPerTenant = 20
const webhookFailureRetention = 30 * 24 * time.Hour
const webhookDeliveryLease = 30 * time.Second

type WebhookDeadLetter struct {
	ID          string    `json:"id"`
	WebhookName string    `json:"webhook_name"`
	Event       string    `json:"event"`
	Attempts    uint64    `json:"attempts"`
	FailedAt    time.Time `json:"failed_at"`
	Replayable  bool      `json:"replayable"`
}

func (s *WebhookService) DeadLetters(user core.User) ([]WebhookDeadLetter, error) {
	items := []WebhookDeadLetter{}
	if user.Role != core.RoleOwner {
		return nil, core.Fail(403, "Chỉ owner quản lý webhook lỗi")
	}
	cutoff := s.clock.Now().Add(-webhookFailureRetention)
	err := s.repo.View(func(store core.Reader) error {
		for _, record := range store.Records(user.TenantID, core.KindWebhookDeliveries) {
			if record.Status != "failed" || !record.UpdatedAt.After(cutoff) {
				continue
			}
			hook, exists := store.Record(user.TenantID, record.ConversationID, core.KindWebhooks)
			var subscription Webhook
			var body struct{ Event string }
			_ = json.Unmarshal([]byte(hook.Content), &subscription)
			_ = json.Unmarshal([]byte(record.Content), &body)
			items = append(items, WebhookDeadLetter{
				ID: record.ID, WebhookName: subscription.Name, Event: body.Event,
				Attempts: record.Sequence, FailedAt: record.UpdatedAt,
				Replayable: exists && hook.Enabled,
			})
		}
		return nil
	})
	sort.Slice(items, func(i, j int) bool {
		if items[i].FailedAt.Equal(items[j].FailedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].FailedAt.After(items[j].FailedAt)
	})
	return items, err
}

func (s *WebhookService) Replay(user core.User, id string) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner quản lý webhook lỗi")
	}
	return s.repo.Update(func(store core.Writer) error {
		now := s.clock.Now().UTC()
		record, ok := store.Record(user.TenantID, id, core.KindWebhookDeliveries)
		if !ok || (record.Status == "failed" && !record.UpdatedAt.After(now.Add(-webhookFailureRetention))) {
			return core.Fail(404, "Không tìm thấy lượt gửi webhook lỗi")
		}
		if record.Status != "failed" {
			return core.Fail(409, "Lượt gửi webhook không ở trạng thái lỗi")
		}
		hook, ok := store.Record(user.TenantID, record.ConversationID, core.KindWebhooks)
		if !ok || !hook.Enabled {
			return core.Fail(409, "Webhook đã bị tắt")
		}
		record.Status = "retry"
		record.Sequence = 0
		record.Enabled = true
		record.UpdatedAt = now
		store.PutRecord(record)
		return nil
	})
}

var allWebhookEvents = []string{
	core.EventTicketCreated, core.EventMessageNew, core.EventSLABreached, core.EventCSATSubmitted, core.EventBillingQuotaLow,
}

// List returns the tenant's webhooks.
func (s *WebhookService) List(user core.User) ([]Webhook, error) {
	items := []Webhook{}
	err := s.repo.View(func(store core.Reader) error {
		for _, record := range store.Records(user.TenantID, core.KindWebhooks) {
			if !record.Enabled {
				continue
			}
			var item Webhook
			if json.Unmarshal([]byte(record.Content), &item) != nil {
				continue
			}
			item.Secret = ""
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

// Save creates or updates a webhook (owner only).
func (s *WebhookService) Save(user core.User, id string, input Webhook) (Webhook, error) {
	if user.Role != core.RoleOwner {
		return Webhook{}, core.Fail(403, "Chỉ owner cấu hình webhook")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 120 {
		return Webhook{}, core.Fail(400, "Tên webhook không hợp lệ")
	}
	target, err := url.Parse(input.URL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil || target.Fragment != "" || (target.Port() != "" && target.Port() != "443" && target.Port() != "80") || !allowedWebhookHost(target.Hostname()) || len(input.URL) > maximumWebhookURLBytes {
		return Webhook{}, core.Fail(400, "URL webhook phải là http/https hợp lệ")
	}
	if len(input.Events) == 0 {
		input.Events = allWebhookEvents
	}
	seen := map[string]bool{}
	for _, event := range input.Events {
		if !contains(allWebhookEvents, event) || seen[event] {
			return Webhook{}, core.Fail(400, "Sự kiện webhook không hợp lệ")
		}
		seen[event] = true
	}
	input.Secret = ""
	err = s.repo.Update(func(store core.Writer) error {
		now := s.clock.Now().UTC()
		if id == "" {
			count := 0
			for _, record := range store.Records(user.TenantID, core.KindWebhooks) {
				if record.Enabled {
					count++
				}
			}
			if count >= maximumWebhooksPerTenant {
				return core.Fail(409, "Tối đa 20 webhook")
			}
			input.ID = s.ids.NewID()
			input.Secret = s.ids.NewID()
			data, _ := json.Marshal(input)
			store.PutRecord(core.Record{
				ID: input.ID, TenantID: user.TenantID, Kind: core.KindWebhooks,
				AuthorID: user.ID, Enabled: true, Content: string(data),
				CreatedAt: now, UpdatedAt: now,
			})
			return nil
		}
		record, ok := store.Record(user.TenantID, id, core.KindWebhooks)
		if !ok || !record.Enabled {
			return core.Fail(404, "Không tìm thấy webhook")
		}
		var previous Webhook
		if err := json.Unmarshal([]byte(record.Content), &previous); err != nil {
			return err
		}
		input.Secret = previous.Secret
		input.ID = id
		data, _ := json.Marshal(input)
		record.Content, record.UpdatedAt = string(data), now
		store.PutRecord(record)
		return nil
	})
	if err != nil {
		return Webhook{}, err
	}
	if id != "" {
		input.Secret = ""
	}
	return input, nil
}

// Remove disables a webhook.
func (s *WebhookService) Remove(user core.User, id string) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner cấu hình webhook")
	}
	return s.repo.Update(func(store core.Writer) error {
		record, ok := store.Record(user.TenantID, id, core.KindWebhooks)
		if !ok || !record.Enabled {
			return core.Fail(404, "Không tìm thấy webhook")
		}
		record.Enabled = false
		record.UpdatedAt = s.clock.Now().UTC()
		store.PutRecord(record)
		return nil
	})
}

// Enqueue queues deliveries for every matching webhook in a tenant. Call inside
// a writer transaction so jobs commit atomically with the triggering change.
func enqueueWebhooks(store core.Writer, tenant, event string, payload map[string]any) {
	body, _ := json.Marshal(map[string]any{"event": event, "tenant": tenant, "payload": payload})
	for _, record := range store.Records(tenant, core.KindWebhooks) {
		if !record.Enabled {
			continue
		}
		var hook Webhook
		if json.Unmarshal([]byte(record.Content), &hook) != nil || !contains(hook.Events, event) {
			continue
		}
		deliveryID := tokenHash("wh:" + record.ID + ":" + string(body))
		if _, ok := store.Record(tenant, deliveryID, core.KindWebhookDeliveries); ok {
			continue
		}
		store.PutRecord(core.Record{
			ID: deliveryID, TenantID: tenant, Kind: core.KindWebhookDeliveries,
			ConversationID: record.ID, Content: string(body),
			Enabled: true, CreatedAt: sClockNow(store),
		})
	}
}

// sClockNow keeps enqueue dependency-free; the dispatcher stamps status later.
func sClockNow(store core.Writer) time.Time { return time.Now().UTC() }

// Run starts the background dispatcher; called once from Workspace.Run.
func (s *WebhookService) Run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		s.dispatch(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// dispatch delivers a bounded batch of pending webhooks.
func (s *WebhookService) dispatch(ctx context.Context) {
	var jobs []core.Record
	now := s.clock.Now().UTC()
	_ = s.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			hooks := map[string]bool{}
			for _, record := range store.Records(tenant, core.KindWebhooks) {
				if record.Enabled {
					hooks[record.ID] = true
				}
			}
			for _, record := range store.Records(tenant, core.KindWebhookDeliveries) {
				if !record.Enabled || record.Status == "sent" || record.UpdatedAt.After(now) {
					continue
				}
				if !hooks[record.ConversationID] {
					continue
				}
				jobs = append(jobs, record)
				if len(jobs) >= 8 {
					return nil
				}
			}
		}
		return nil
	})
	for _, item := range jobs {
		if ctx.Err() != nil {
			return
		}
		var lease time.Time
		var hook Webhook
		claimErr := s.repo.Update(func(store core.Writer) error {
			current, ok := store.Record(item.TenantID, item.ID, core.KindWebhookDeliveries)
			now := s.clock.Now().UTC()
			if !ok || !current.Enabled || current.Status == "sent" || current.UpdatedAt.After(now) {
				return nil
			}
			subscription, ok := store.Record(current.TenantID, current.ConversationID, core.KindWebhooks)
			if !ok || !subscription.Enabled || json.Unmarshal([]byte(subscription.Content), &hook) != nil {
				return nil
			}
			lease = now.Add(webhookDeliveryLease).Truncate(time.Microsecond)
			current.UpdatedAt = lease
			store.PutRecord(current)
			item = current
			return nil
		})
		if claimErr != nil || lease.IsZero() {
			continue
		}
		err := s.deliver(ctx, hook, item.Content, item.ID)
		if ctx.Err() != nil {
			return
		}
		_ = s.repo.Update(func(store core.Writer) error {
			current, ok := store.Record(item.TenantID, item.ID, core.KindWebhookDeliveries)
			if !ok || !current.UpdatedAt.Equal(lease) {
				return nil
			}
			if err == nil {
				current.Status = "sent"
				current.Enabled = false
				current.UpdatedAt = s.clock.Now().UTC()
			} else {
				current.Sequence++
				current.Status = "retry"
				current.UpdatedAt = s.clock.Now().Add(time.Minute * time.Duration(1<<min(current.Sequence, 6)))
				if current.Sequence >= 8 {
					current.Status = "failed"
					current.Enabled = false
					current.UpdatedAt = s.clock.Now().UTC()
				}
			}
			store.PutRecord(current)
			return nil
		})
	}
}

func (s *WebhookService) deliver(ctx context.Context, hook Webhook, body, deliveryID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nexa-Signature", signWebhook(hook.Secret, body))
	req.Header.Set("X-Nexa-Delivery", deliveryID)
	response, err := s.client.Do(req)
	if err != nil {
		return err
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("webhook HTTP %d", response.StatusCode)
	}
	return nil
}

// signWebhook produces an HMAC-SHA256 hex signature a consumer can verify.
func signWebhook(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
