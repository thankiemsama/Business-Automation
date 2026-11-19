package service

import (
	"context"
	"encoding/json"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"businessautomation/internal/core"
)

type TenantSettings struct {
	Allowlist       []string        `json:"allowlist"`
	Flags           map[string]bool `json:"flags"`
	DailyAIRequests int             `json:"daily_ai_requests,omitempty"`
	DisplayName     string          `json:"display_name"`
	LogoURL         string          `json:"logo_url"`
	Accent          string          `json:"accent"`
	Host            string          `json:"host"`
}
type Admin struct {
	repo     *Workspace
	clock    Clock
	mu       sync.Mutex
	crmMu    sync.Mutex
	flushMu  sync.Mutex
	requests map[string]uint64
}

func NewAdmin(repo *Workspace, clock Clock) *Admin {
	return &Admin{repo: repo, clock: clock, requests: map[string]uint64{}}
}

const tenantSettingsKind = "tenant_settings"

func settingsID(tenant string) string { return tokenHash("tenant-settings:" + tenant) }
func tenantSettings(store core.Reader, tenant string) (TenantSettings, error) {
	result := TenantSettings{Allowlist: []string{}, Flags: map[string]bool{}}
	if item, ok := store.Record(tenant, settingsID(tenant), tenantSettingsKind); ok {
		if err := json.Unmarshal([]byte(item.Content), &result); err != nil {
			return TenantSettings{}, err
		}
	}
	return result, nil
}
func (a *Admin) Settings(tenant string) (TenantSettings, error) {
	var result TenantSettings
	err := a.repo.View(func(store core.Reader) error { var err error; result, err = tenantSettings(store, tenant); return err })
	return result, err
}
func networkAllowed(allowlist []string, ip netip.Addr) bool {
	if len(allowlist) == 0 {
		return true
	}
	if !ip.IsValid() {
		return false
	}
	for _, raw := range allowlist {
		if prefix, err := netip.ParsePrefix(raw); err == nil && prefix.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func (a *Admin) Allowed(tenant string, ip netip.Addr) bool {
	settings, err := a.Settings(tenant)
	return err == nil && networkAllowed(settings.Allowlist, ip)
}

var hexAccent = regexp.MustCompile("^#[a-fA-F0-9]{6}$")
var tenantHostname = regexp.MustCompile("^[a-z0-9](?:[a-z0-9-]{0,62})(?:\\.[a-z0-9](?:[a-z0-9-]{0,62}))+\\z")
var tenantFlags = map[string]bool{
	"ai": true, "ai_quality_review": true, "ai_knowledge_drafts": true, "ai_voice_import": true,
	"ai_ticket_agent": true, "ai_ticket_agent_auto_resolve": true,
	"ai_teammate": true, "ai_teammate_sla": true, "ai_teammate_inbox": true,
	"workflows": true, "public_api": true,
}

func (a *Admin) SaveSettings(user core.User, value TenantSettings, ip netip.Addr) (TenantSettings, error) {
	if user.Role != core.RoleOwner {
		return value, core.Fail(403, "Chỉ owner được đổi cấu hình")
	}
	if value.DailyAIRequests < 0 || value.DailyAIRequests > 100000 {
		return value, core.Fail(400, "Quota AI theo ngày phải từ 0 đến 100.000 lượt")
	}
	if len(value.Allowlist) > 30 || len([]rune(value.DisplayName)) > 80 || len(value.LogoURL) > 1000 || value.Accent != "" && !hexAccent.MatchString(value.Accent) {
		return value, core.Fail(400, "Cấu hình không hợp lệ")
	}
	for i, raw := range value.Allowlist {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || prefix.Addr().Is4In6() {
			return value, core.Fail(400, "Mỗi mục mạng phải ở dạng CIDR")
		}
		value.Allowlist[i] = prefix.Masked().String()
	}
	if !networkAllowed(value.Allowlist, ip) {
		return value, core.Fail(400, "Allowlist phải bao gồm địa chỉ hiện tại")
	}
	for key := range value.Flags {
		if !tenantFlags[key] {
			return value, core.Fail(400, "Feature flag không hợp lệ")
		}
	}
	if value.LogoURL != "" {
		parsed, err := url.Parse(value.LogoURL)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
			return value, core.Fail(400, "Logo phải là URL HTTPS")
		}
	}
	value.Host = strings.ToLower(strings.TrimSpace(value.Host))
	if value.Host != "" && (len(value.Host) > 253 || !tenantHostname.MatchString(value.Host)) {
		return value, core.Fail(400, "Tên miền không hợp lệ")
	}
	err := a.repo.updateAs(user.ID, func(store core.Writer) error {
		for _, tenant := range store.Tenants() {
			if value.Host == "" || tenant == user.TenantID {
				continue
			}
			settings, err := tenantSettings(store, tenant)
			if err != nil {
				return err
			}
			if settings.Host == value.Host {
				return core.Fail(409, "Tên miền đã được dùng")
			}
		}
		payload, err := json.Marshal(value)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{ID: settingsID(user.TenantID), TenantID: user.TenantID, Kind: tenantSettingsKind, Content: string(payload), UpdatedAt: a.clock.Now()})
		return nil
	})
	return value, err
}
func featureEnabled(store core.Reader, tenant, flag string) bool {
	settings, err := tenantSettings(store, tenant)
	if err != nil {
		return false
	}
	enabled, set := settings.Flags[flag]
	return !set || enabled
}
func (a *Admin) Feature(tenant, flag string) bool {
	enabled := false
	_ = a.repo.View(func(store core.Reader) error { enabled = featureEnabled(store, tenant, flag); return nil })
	return enabled
}
func (a *Admin) Branding(host string) (map[string]string, error) {
	result := map[string]string{}
	err := a.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			settings, err := tenantSettings(store, tenant)
			if err != nil {
				return err
			}
			if settings.Host != "" && settings.Host == host {
				result = map[string]string{"name": settings.DisplayName, "logo_url": settings.LogoURL, "accent": settings.Accent}
				return nil
			}
		}
		return nil
	})
	return result, err
}
func (a *Admin) Meter(tenant string) {
	key := tenant + ":" + a.clock.Now().In(ReportZone).Format("2006-01")
	a.mu.Lock()
	a.requests[key]++
	a.mu.Unlock()
}
func (a *Admin) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.flush()
			return
		case <-ticker.C:
			a.flush()
		}
	}
}
func (a *Admin) flush() {
	a.flushMu.Lock()
	defer a.flushMu.Unlock()
	a.mu.Lock()
	current := a.requests
	a.requests = map[string]uint64{}
	a.mu.Unlock()
	if len(current) == 0 {
		return
	}
	if err := a.repo.Update(func(store core.Writer) error {
		for key, count := range current {
			tenant, month, _ := strings.Cut(key, ":")
			id := tokenHash("usage:" + key)
			r, _ := store.Record(tenant, id, "meter")
			r.ID, r.TenantID, r.Kind, r.Name = id, tenant, "meter", month
			r.Sequence += count
			store.PutRecord(r)
		}
		return nil
	}); err != nil {
		a.mu.Lock()
		for key, count := range current {
			a.requests[key] += count
		}
		a.mu.Unlock()
	}
}
func (a *Admin) Usage(tenant string) (map[string]any, error) {
	a.flushMu.Lock()
	defer a.flushMu.Unlock()
	now := a.clock.Now().In(ReportZone)
	month := now.Format("2006-01")
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, ReportZone)
	end := start.AddDate(0, 1, 0)
	counts := map[string]any{"month": month, "messages": 0, "tickets": 0, "requests": uint64(0), "plan": "free"}
	a.mu.Lock()
	pending := a.requests[tenant+":"+month]
	a.mu.Unlock()
	err := a.repo.View(func(store core.Reader) error {
		for _, kind := range []string{core.KindTickets, core.KindMessages} {
			count := 0
			for _, r := range store.Records(tenant, kind) {
				if !r.CreatedAt.Before(start) && r.CreatedAt.Before(end) {
					count++
				}
			}
			counts[kind] = count
		}
		persisted, _ := store.Record(tenant, tokenHash("usage:"+tenant+":"+month), "meter")
		counts["requests"] = pending + persisted.Sequence
		if plan, ok := store.Record(tenant, tokenHash("plan:"+tenant), "billing_plan"); ok && plan.Enabled {
			counts["plan"] = plan.Status
		}
		return nil
	})
	return counts, err
}

func (a *Admin) Export(user core.User, customerID string) (map[string]any, error) {
	result := map[string]any{"version": 1, "generated_at": a.clock.Now(), "tenant_id": user.TenantID}
	err := a.repo.View(func(store core.Reader) error {
		if customerID != "" {
			if _, err := findOwnedRecord(store, user, customerID, core.KindCustomers); err != nil {
				return err
			}
		}
		rows := []core.Record{}
		conversations := map[string]bool{}
		for _, r := range store.Records(user.TenantID, core.KindConversations) {
			if r.CustomerID == customerID {
				conversations[r.ID] = true
			}
		}
		for _, r := range store.Records(user.TenantID, "") {
			switch r.Kind {
			case core.KindCustomers, core.KindConversations, core.KindTickets, core.KindMessages, core.KindKnowledge, core.KindAutomations, "audit_log", "custom_fields", "workflow_rules", "templates", core.KindCustomerNotes:
			default:
				continue
			}
			if customerID != "" {
				if !(r.Kind == core.KindCustomers && r.ID == customerID || r.CustomerID == customerID || r.Kind == core.KindMessages && conversations[r.ConversationID] && r.Sender != "note") {
					continue
				}
				if r.Kind == core.KindCustomerNotes {
					continue
				}
			}
			rows = append(rows, r)
		}
		result["records"] = rows
		return nil
	})
	return result, err
}
