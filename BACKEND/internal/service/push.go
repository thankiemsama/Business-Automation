package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"businessautomation/internal/core"
	webpush "github.com/SherClockHolmes/webpush-go"
)

type PushInput struct {
	webpush.Subscription
	Disable bool `json:"disable"`
}

func (s *Workspace) PushKey() string {
	if os.Getenv("VAPID_PRIVATE_KEY") == "" || os.Getenv("VAPID_SUBJECT") == "" {
		return ""
	}
	return os.Getenv("VAPID_PUBLIC_KEY")
}

func (s *Workspace) SavePush(user core.User, input PushInput) error {
	if s.PushKey() == "" {
		return core.Fail(503, "Web Push chưa được cấu hình")
	}
	endpoint, err := url.Parse(input.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.User != nil || endpoint.Fragment != "" || endpoint.Port() != "" || len(input.Endpoint) > 2048 || !validPushHost(endpoint.Hostname()) {
		return core.Fail(400, "Địa chỉ Web Push không hợp lệ")
	}
	if !input.Disable {
		auth, aErr := base64.RawURLEncoding.DecodeString(strings.TrimRight(input.Keys.Auth, "="))
		key, kErr := base64.RawURLEncoding.DecodeString(strings.TrimRight(input.Keys.P256dh, "="))
		if aErr != nil || kErr != nil || len(auth) != 16 || len(key) != 65 || key[0] != 4 {
			return core.Fail(400, "Khóa Web Push không hợp lệ")
		}
	}
	return s.Update(func(store core.Writer) error {
		id := tokenHash("push:" + input.Endpoint)
		record, ok := store.Record(user.TenantID, id, "push_subscriptions")
		if input.Disable {
			if !ok || record.AuthorID != user.ID {
				return nil
			}
			record.Enabled = false
		} else {
			count := 0
			for _, item := range store.Records(user.TenantID, "push_subscriptions") {
				if item.Enabled && item.AuthorID == user.ID && item.ID != id {
					count++
				}
			}
			if count >= 10 {
				return core.Fail(409, "Tối đa 10 thiết bị nhận Web Push")
			}
			data, err := json.Marshal(input.Subscription)
			if err != nil {
				return err
			}
			record = core.Record{ID: id, TenantID: user.TenantID, Kind: "push_subscriptions", AuthorID: user.ID, Content: string(data), Enabled: true}
		}
		store.PutRecord(record)
		return nil
	})
}

func validPushHost(host string) bool {
	host = strings.ToLower(host)
	return host == "fcm.googleapis.com" || host == "updates.push.services.mozilla.com" || host == "web.push.apple.com" || host == "wns.windows.com" || strings.HasSuffix(host, ".notify.windows.com") || strings.HasSuffix(host, ".push.apple.com")
}

var pushClient = &http.Client{
	Timeout:       4 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (s *Workspace) sendPush(ctx context.Context) {
	key := s.PushKey()
	if key == "" {
		return
	}
	type delivery struct{ notice, subscription core.Record }
	var queue []delivery
	now := s.Clock.Now()
	_ = s.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			subscriptions := store.Records(tenant, "push_subscriptions")
			for _, item := range store.Records(tenant, notificationKind) {
				if item.ReadAt != nil || now.Sub(item.CreatedAt) > 24*time.Hour {
					continue
				}
				user, exists := store.UserByID(item.AuthorID)
				if !exists || user.Disabled {
					continue
				}
				for _, sub := range subscriptions {
					if !sub.Enabled || sub.AuthorID != item.AuthorID {
						continue
					}
					if _, ok := store.Record(tenant, item.ID+":"+sub.ID, "push_deliveries"); ok {
						continue
					}
					queue = append(queue, delivery{item, sub})
					if len(queue) >= 8 {
						return nil
					}
				}
			}
		}
		return nil
	})
	for _, job := range queue {
		if ctx.Err() != nil {
			return
		}
		var subscription webpush.Subscription
		if json.Unmarshal([]byte(job.subscription.Content), &subscription) != nil {
			continue
		}
		endpoint, err := url.Parse(subscription.Endpoint)
		if err != nil || endpoint.Scheme != "https" || !validPushHost(endpoint.Hostname()) {
			continue
		}
		response, err := webpush.SendNotificationWithContext(ctx, []byte("Bạn có thông báo mới trong Nexa."), &subscription, &webpush.Options{
			HTTPClient: pushClient, Subscriber: strings.TrimPrefix(os.Getenv("VAPID_SUBJECT"), "mailto:"), VAPIDPublicKey: key, VAPIDPrivateKey: os.Getenv("VAPID_PRIVATE_KEY"), TTL: 300,
		})
		if err != nil {
			continue
		}
		response.Body.Close()
		expired := response.StatusCode == 404 || response.StatusCode == 410
		if response.StatusCode >= 500 || response.StatusCode == 429 {
			continue
		}
		_ = s.Update(func(store core.Writer) error {
			if expired {
				if sub, ok := store.Record(job.subscription.TenantID, job.subscription.ID, "push_subscriptions"); ok && sub.Content == job.subscription.Content {
					sub.Enabled = false
					store.PutRecord(sub)
				}
			}
			store.PutRecord(core.Record{ID: job.notice.ID + ":" + job.subscription.ID, TenantID: job.notice.TenantID, Kind: "push_deliveries"})
			return nil
		})
	}
}
