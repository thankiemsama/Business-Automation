package service

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const billingAI = "ai"
const billingTickets = "tickets"

var errBillingQuota = core.Fail(402, "Đã hết hạn mức tháng của gói hiện tại")

type BillingQuota struct {
	Used     uint64 `json:"used"`
	Included uint64 `json:"included"`
	Credits  uint64 `json:"credits"`
	Overage  uint64 `json:"overage"`
	Limited  bool   `json:"limited"`
	Low      bool   `json:"low"`
}

func billingPlan(store core.Reader, tenant string) string {
	if plan, ok := store.Record(tenant, tokenHash("plan:"+tenant), "billing_plan"); ok && plan.Enabled &&
		(plan.Status == "pro" || plan.Status == "enterprise") {
		return plan.Status
	}
	return "free"
}

func billingLimit(plan, metric, suffix string) uint64 {
	value, _ := strconv.ParseUint(os.Getenv("NEXA_BILLING_"+strings.ToUpper(plan+"_"+metric+"_"+suffix)), 10, 32)
	return value
}

func billingMonth(now time.Time) string { return now.In(ReportZone).Format("2006-01") }

func billingQuota(store core.Reader, tenant, metric string, now time.Time) (BillingQuota, core.Record) {
	plan := billingPlan(store, tenant)
	month := billingMonth(now)
	record, found := store.Record(tenant, tokenHash("quota:"+tenant+":"+month+":"+metric), "billing_quota")
	if !found {
		record = core.Record{ID: tokenHash("quota:" + tenant + ":" + month + ":" + metric), TenantID: tenant, Kind: "billing_quota", CreatedAt: now}
		kind := core.KindTickets
		if metric == billingAI {
			kind = "ai_usage_samples"
		}
		for _, item := range store.Records(tenant, kind) {
			if billingMonth(item.CreatedAt) == month {
				record.Sequence++
			}
		}
	}
	included := billingLimit(plan, metric, "limit")
	var credits uint64
	if plan != "free" {
		entitlement, _ := store.Record(tenant, tokenHash("invoice-quota:"+tenant+":"+month+":"+plan+":"+metric), "billing_credits")
		credits = entitlement.Sequence
	}
	result := BillingQuota{Used: record.Sequence, Included: included, Credits: credits, Limited: included > 0}
	if result.Limited {
		result.Low = result.Used >= ((included+credits)*8+9)/10
		if result.Used > included {
			result.Overage = result.Used - included
		}
	}
	return result, record
}

type invoiceSubscription struct {
	Subscription string
	Metadata     map[string]string
}

func (a *Admin) applyPaidInvoice(body []byte, paidAt time.Time) error {
	var event struct {
		Data struct {
			Object struct {
				ID, Customer, Subscription, Status string
				AmountPaid                         int64               `json:"amount_paid"`
				BillingReason                      string              `json:"billing_reason"`
				SubscriptionDetails                invoiceSubscription `json:"subscription_details"`
				Parent                             struct {
					SubscriptionDetails invoiceSubscription `json:"subscription_details"`
				}
				Lines struct {
					Data []struct {
						Price   struct{ ID string }
						Pricing struct {
							PriceDetails struct{ Price string } `json:"price_details"`
						}
					}
				}
			}
		}
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return core.Fail(400, "Hóa đơn Stripe không hợp lệ")
	}
	invoice := event.Data.Object
	if invoice.Status != "paid" || invoice.AmountPaid <= 0 || invoice.ID == "" || invoice.Customer == "" ||
		!contains([]string{"subscription_create", "subscription_cycle"}, invoice.BillingReason) {
		return nil
	}
	details := invoice.Parent.SubscriptionDetails
	if details.Subscription == "" {
		details = invoice.SubscriptionDetails
		details.Subscription = invoice.Subscription
	}
	tenant := details.Metadata["nexa_tenant"]
	if tenant == "" || details.Subscription == "" {
		return nil
	}
	plan := ""
	for _, line := range invoice.Lines.Data {
		price := line.Pricing.PriceDetails.Price
		if price == "" {
			price = line.Price.ID
		}
		if price != "" && price == stripePrice("enterprise") {
			plan = "enterprise"
			break
		}
		if price != "" && price == stripePrice("pro") {
			plan = "pro"
		}
	}
	if plan == "" {
		return nil
	}
	return a.repo.UpdateTenant(tenant, func(store core.Writer) error {
		marker := tokenHash("paid-invoice:" + tenant + ":" + invoice.ID)
		if _, exists := store.Record(tenant, marker, "billing_events"); exists {
			return nil
		}
		subscription, exists := store.Record(tenant, tokenHash("plan:"+tenant), "billing_plan")
		if !exists || !subscription.Enabled {
			if len(store.Users(tenant)) == 0 {
				return nil
			}
			return core.Fail(409, "Đang chờ trạng thái subscription từ Stripe")
		}
		if subscription.CustomerID != invoice.Customer || subscription.Name != details.Subscription {
			return nil
		}
		for _, metric := range []string{billingTickets, billingAI} {
			grant := billingLimit(plan, metric, "invoice_credits")
			if grant == 0 || billingLimit(plan, metric, "limit") == 0 {
				continue
			}
			id := tokenHash("invoice-quota:" + tenant + ":" + billingMonth(paidAt) + ":" + plan + ":" + metric)
			credit, _ := store.Record(tenant, id, "billing_credits")
			credit.ID, credit.TenantID, credit.Kind = id, tenant, "billing_credits"
			credit.Sequence += grant
			credit.UpdatedAt = a.clock.Now()
			store.PutRecord(credit)
		}
		store.PutRecord(core.Record{ID: marker, TenantID: tenant, Kind: "billing_events", CreatedAt: paidAt})
		return nil
	})
}

func reserveBillingQuota(store core.Writer, tenant, metric string, now time.Time) error {
	if billingLimit("free", metric, "limit") == 0 &&
		billingLimit("pro", metric, "limit") == 0 &&
		billingLimit("enterprise", metric, "limit") == 0 {
		return nil
	}
	quota, record := billingQuota(store, tenant, metric, now)
	if quota.Limited && quota.Used >= quota.Included+quota.Credits {
		return errBillingQuota
	}
	record.Sequence++
	record.UpdatedAt = now
	store.PutRecord(record)
	threshold := ((quota.Included+quota.Credits)*8 + 9) / 10
	if quota.Limited && record.Sequence == threshold {
		enqueueWebhooks(store, tenant, core.EventBillingQuotaLow, map[string]any{
			"metric": metric, "month": billingMonth(now), "limit": quota.Included + quota.Credits,
			"remaining": quota.Included + quota.Credits - record.Sequence,
		})
	}
	return nil
}
