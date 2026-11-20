package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"businessautomation/internal/core"
)

func stripePrice(plan string) string {
	switch plan {
	case "pro":
		return os.Getenv("STRIPE_PRO_PRICE")
	case "enterprise":
		return os.Getenv("STRIPE_ENTERPRISE_PRICE")
	}
	return ""
}
func (a *Admin) BillingOptions() map[string]bool {
	enabled := os.Getenv("STRIPE_SECRET_KEY") != "" && os.Getenv("STRIPE_WEBHOOK_SECRET") != "" && os.Getenv("APP_ORIGIN") != ""
	return map[string]bool{"pro": enabled && stripePrice("pro") != "", "enterprise": enabled && stripePrice("enterprise") != ""}
}

var providerClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func (a *Admin) BillingLink(ctx context.Context, user core.User, plan string) (string, error) {
	origin := strings.TrimRight(os.Getenv("APP_ORIGIN"), "/")
	if os.Getenv("STRIPE_SECRET_KEY") == "" || origin == "" {
		return "", core.Fail(503, "Chưa cấu hình Stripe")
	}
	params := url.Values{}
	var current core.Record
	_ = a.repo.View(func(store core.Reader) error {
		current, _ = store.Record(user.TenantID, tokenHash("plan:"+user.TenantID), "billing_plan")
		return nil
	})
	api := "https://api.stripe.com/v1/checkout/sessions"
	if plan == "portal" {
		if current.CustomerID == "" {
			return "", core.Fail(404, "Chưa có tài khoản thanh toán")
		}
		api = "https://api.stripe.com/v1/billing_portal/sessions"
		params.Set("customer", current.CustomerID)
		params.Set("return_url", origin+"/admin")
	} else {
		if !a.BillingOptions()[plan] {
			return "", core.Fail(503, "Gói này chưa cấu hình thanh toán")
		}
		if current.Enabled {
			return "", core.Fail(409, "Dùng cổng thanh toán để đổi gói")
		}
		params.Set("mode", "subscription")
		params.Set("line_items[0][price]", stripePrice(plan))
		params.Set("line_items[0][quantity]", "1")
		params.Set("subscription_data[metadata][nexa_tenant]", user.TenantID)
		params.Set("success_url", origin+"/admin?billing=return")
		params.Set("cancel_url", origin+"/admin")
		if current.CustomerID != "" {
			params.Set("customer", current.CustomerID)
		} else {
			params.Set("customer_email", user.Email)
		}
	}
	request, err := http.NewRequestWithContext(ctx, "POST", api, strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+os.Getenv("STRIPE_SECRET_KEY"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Idempotency-Key", tokenHash(user.TenantID+":"+plan+":"+strconv.FormatInt(a.clock.Now().Unix()/600, 10)))
	response, err := providerClient.Do(request)
	if err != nil {
		return "", core.Fail(502, "Không kết nối được Stripe")
	}
	defer response.Body.Close()
	var result struct{ URL string }
	if response.StatusCode/100 != 2 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) != nil {
		return "", core.Fail(502, "Stripe từ chối yêu cầu")
	}
	target, err := url.Parse(result.URL)
	if err != nil || target.Scheme != "https" || (target.Hostname() != "checkout.stripe.com" && target.Hostname() != "billing.stripe.com") {
		return "", core.Fail(502, "Stripe không trả liên kết hợp lệ")
	}
	return result.URL, nil
}

func (a *Admin) StripeEvent(signature string, body []byte) error {
	secret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if secret == "" {
		return core.Fail(503, "Webhook Stripe chưa cấu hình")
	}
	var timestamp int64
	signatures := []string{}
	for _, part := range strings.Split(signature, ",") {
		key, value, _ := strings.Cut(part, "=")
		if key == "t" {
			timestamp, _ = strconv.ParseInt(value, 10, 64)
		}
		if key == "v1" {
			signatures = append(signatures, value)
		}
	}
	now := a.clock.Now().Unix()
	if timestamp < now-300 || timestamp > now+300 {
		return core.Fail(401, "Webhook Stripe đã hết hạn")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
	mac.Write(body)
	valid := false
	for _, value := range signatures {
		if raw, err := hex.DecodeString(value); err == nil && hmac.Equal(raw, mac.Sum(nil)) {
			valid = true
		}
	}
	if !valid {
		return core.Fail(401, "Chữ ký Stripe không hợp lệ")
	}
	var event struct {
		ID, Type string
		Created  int64
		Data     struct {
			Object struct {
				ID, Status, Customer string
				Metadata             map[string]string
				Items                struct {
					Data []struct{ Price struct{ ID string } }
				}
			}
		}
	}
	if json.Unmarshal(body, &event) != nil || event.ID == "" || event.Created <= 0 || event.Created > now+300 {
		return core.Fail(400, "Sự kiện Stripe không hợp lệ")
	}
	if event.Type == "invoice.paid" {
		return a.applyPaidInvoice(body, time.Unix(event.Created, 0))
	}
	if !contains([]string{"customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted"}, event.Type) {
		return nil
	}
	object := event.Data.Object
	tenant := object.Metadata["nexa_tenant"]
	if tenant == "" || object.ID == "" || object.Customer == "" {
		return nil
	}
	plan := ""
	for _, item := range object.Items.Data {
		if item.Price.ID != "" && item.Price.ID == stripePrice("enterprise") {
			plan = "enterprise"
			break
		}
		if item.Price.ID != "" && item.Price.ID == stripePrice("pro") {
			plan = "pro"
		}
	}
	active := (object.Status == "active" || object.Status == "trialing") && event.Type != "customer.subscription.deleted"
	if active && plan == "" {
		return nil
	}
	return a.repo.Update(func(store core.Writer) error {
		if len(store.Users(tenant)) == 0 {
			return nil
		}
		marker := tokenHash("stripe-event:" + event.ID)
		if _, ok := store.Record(tenant, marker, "billing_events"); ok {
			return nil
		}
		id := tokenHash("plan:" + tenant)
		current, _ := store.Record(tenant, id, "billing_plan")
		if current.Sequence > uint64(event.Created) || current.Enabled && current.Name != object.ID {
			return nil
		}
		if current.CustomerID != "" && current.CustomerID != object.Customer {
			return nil
		}
		if !active {
			plan = "free"
		}
		store.PutRecord(core.Record{ID: id, TenantID: tenant, Kind: "billing_plan", CustomerID: object.Customer, Name: object.ID, Status: plan, Enabled: active, Sequence: uint64(event.Created), UpdatedAt: a.clock.Now()})
		store.PutRecord(core.Record{ID: marker, TenantID: tenant, Kind: "billing_events", UpdatedAt: a.clock.Now()})
		return nil
	})
}
