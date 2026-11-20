package service

import (
	"strings"
	"time"

	"businessautomation/internal/core"
)

// findCustomerByEmail locates a customer record by lower-cased email.
func findCustomerByEmail(store core.Reader, tenant, email string) (core.Record, bool) {
	email = strings.TrimSpace(email)
	if email == "" {
		return core.Record{}, false
	}
	var match core.Record
	for _, record := range store.Records(tenant, core.KindCustomers) {
		if record.Status == core.CustomerMerged || !strings.EqualFold(strings.TrimSpace(record.Email), email) {
			continue
		}
		if match.ID == "" || record.CreatedAt.Before(match.CreatedAt) ||
			(record.CreatedAt.Equal(match.CreatedAt) && record.ID < match.ID) {
			match = record
		}
	}
	return match, match.ID != ""
}

// CustomerEmail resolves a customer's email address for a ticket (best effort).
func (s *PlatformService) CustomerEmail(tenant, customerID string) string {
	if customerID == "" {
		return ""
	}
	var email string
	_ = s.repo.View(func(store core.Reader) error {
		customer, ok := store.Record(tenant, customerID, core.KindCustomers)
		if ok {
			email = customer.Email
		}
		return nil
	})
	return email
}

// CreatePortalTicket lets a portal customer open a new ticket through their
// own web conversation channel, then runs enabled automations.
func (s *PlatformService) CreatePortalTicket(tenant, token, subject, content, priority, fileName, fileData string) (core.Record, error) {
	if err := validateAttachment(fileName, fileData); err != nil {
		return core.Record{}, err
	}
	subject = strings.TrimSpace(subject)
	content = strings.TrimSpace(content)
	if subject == "" || len([]rune(subject)) > 400 {
		return core.Record{}, core.Fail(400, "Tiêu đề phiếu không hợp lệ")
	}
	if content == "" || len(content) > maximumMessageBytes {
		return core.Record{}, core.Fail(400, "Mô tả phiếu không hợp lệ")
	}
	if priority == "" {
		priority = core.PriorityNormal
	}
	if priority != core.PriorityLow && priority != core.PriorityNormal && priority != core.PriorityHigh && priority != core.PriorityUrgent {
		return core.Record{}, core.Fail(400, "Mức ưu tiên không hợp lệ")
	}
	var result core.Record
	err := s.repo.Update(func(store core.Writer) error {
		customer, err := s.sessionCustomerIn(store, tenant, token)
		if err != nil {
			return err
		}
		now := s.clock.Now().UTC()
		// Separate ticket threads so replies and files remain associated with the right request.
		conversation := core.Record{
			ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindConversations,
			CustomerID: customer.ID, Channel: core.ChannelWeb, Status: core.StatusOpen,
			Subject: subject, Priority: priority, CreatedAt: now, UpdatedAt: now,
		}
		store.PutRecord(conversation)
		routed, _ := store.Record(tenant, conversation.ID, core.KindConversations)
		store.PutRecord(core.Record{ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindMessages, ConversationID: conversation.ID, Content: content, Sender: core.SenderCustomer, Sequence: 1, AttachmentName: fileName, AttachmentData: fileData, CreatedAt: now, UpdatedAt: now})
		ticket := core.Record{
			ID: s.ids.NewID(), TenantID: tenant, Kind: core.KindTickets,
			CustomerID: customer.ID, ConversationID: conversation.ID,
			Subject: subject, Content: content, Status: core.StatusOpen,
			Priority: priority, Tags: append([]string{}, customer.Tags...),
			AssigneeID: routed.AssigneeID,
			CreatedAt: now, UpdatedAt: now,
		}
		response, resolution := ticketDeadlines(ticket)
		ticket.ResponseDueAt, ticket.ResolutionDueAt = &response, &resolution
		store.PutRecord(ticket)
		result = portalTicketRecord(ticket)
		return nil
	})
	if err != nil {
		return core.Record{}, err
	}
	return result, nil
}

func portalTicketRecord(ticket core.Record) core.Record {
	ticket.Summary, ticket.SummarySource, ticket.RiskReason, ticket.CustomValues = "", "", "", ""
	return ticket
}

// ---- Multi-channel configuration ---------------------------------------

// ChannelProvider is a pluggable adapter for an external channel (Zalo OA,
// Messenger, WhatsApp, SMS, Instagram DM). Implementations translate in/out.
type ChannelProvider interface {
	Name() string
	Validate(store core.Reader, config *core.ChannelConfig) error
}

// ChannelManager stores and validates per-tenant channel configurations.
type ChannelManager struct {
	repo           core.Repository
	providers      map[string]ChannelProvider
	conversations  *ConversationService
	campaignOrigin string
}

func NewChannels(repo core.Repository) *ChannelManager {
	m := &ChannelManager{repo: repo, providers: map[string]ChannelProvider{}}
	m.registerLiveChannels()
	return m
}

// Register adds a provider implementation for a channel name.
func (m *ChannelManager) ConfigureConversations(conversations *ConversationService) {
	m.conversations = conversations
	conversations.channels = m
}

func (m *ChannelManager) Register(provider ChannelProvider) { m.providers[provider.Name()] = provider }

// Provider returns a registered provider, or a generic built-in adapter.
func (m *ChannelManager) Provider(name string) ChannelProvider {
	if provider, ok := m.providers[name]; ok {
		return provider
	}
	return &GenericChannel{name}
}

// GenericChannel is a no-op adapter that validates shape only, so tenants can
// configure credentials without a live connector being wired in.
type GenericChannel struct{ name string }

func (g *GenericChannel) Name() string { return g.name }

func (g *GenericChannel) Validate(store core.Reader, config *core.ChannelConfig) error {
	if config.Name == "" && config.Content == "" {
		return core.Fail(400, "Thiếu thông tin cấu hình kênh")
	}
	return nil
}

// Configure stores channel settings for a tenant (owner only).
func (m *ChannelManager) Configure(user core.User, channel string, input core.Record) (core.Record, error) {
	if user.Role != core.RoleOwner {
		return core.Record{}, core.Fail(403, "Chỉ owner cấu hình kênh")
	}
	if !contains(core.PublicChannels, channel) {
		return core.Record{}, core.Fail(400, "Kênh không được hỗ trợ")
	}
	provider := m.Provider(channel)
	var result core.Record
	err := m.repo.Update(func(store core.Writer) error {
		id := tokenHash("channel:" + user.TenantID + ":" + channel)
		record, ok := store.Record(user.TenantID, id, core.KindChannelConfig)
		if !ok {
			record = core.Record{ID: id, TenantID: user.TenantID, Kind: core.KindChannelConfig}
		}
		config := core.ChannelConfigFromRecord(record)
		config.Channel, config.Name, config.Content = channel, input.Name, input.Content
		config.Enabled = input.Enabled
		config.UpdatedAt = time.Now().UTC()
		if live, ok := provider.(*liveChannel); ok && !strings.HasPrefix(strings.TrimSpace(config.Content), "{") && live.credentials[user.TenantID].AccessToken == "" {
			if err := (&GenericChannel{name: channel}).Validate(store, &config); err != nil {
				return err
			}
		} else if err := provider.Validate(store, &config); err != nil {
			return err
		}
		result = config.ApplyTo(record)
		store.PutRecord(result)
		return nil
	})
	return result, err
}

// Status reports each public channel's configuration state for a tenant.
func (m *ChannelManager) Status(user core.User) (map[string]core.Record, error) {
	status := map[string]core.Record{}
	err := m.repo.View(func(store core.Reader) error {
		for _, channel := range core.PublicChannels {
			record, ok := store.Record(user.TenantID, tokenHash("channel:"+user.TenantID+":"+channel), core.KindChannelConfig)
			if !ok {
				record = core.Record{ID: tokenHash("channel:" + user.TenantID + ":" + channel), TenantID: user.TenantID, Kind: core.KindChannelConfig, Channel: channel}
			}
			if isLiveChannel(channel) && record.Enabled {
				record.Status = "configured"
				if _, _, _, err := m.providerAccount(store, user.TenantID, channel); err != nil {
					record.Status = "credentials_required"
				}
			}
			status[channel] = record
		}
		return nil
	})
	return status, err
}

// ---- Public API keys ----------------------------------------------------

// APIKey is a developer key for a tenant's public REST API.
type APIKey struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Secret     string   `json:"secret,omitempty"`
	Scopes     []string `json:"scopes"`
	RatePerMin int      `json:"rate_per_min"`
	Enabled    bool     `json:"enabled"`
	LastUsedAt string   `json:"last_used_at,omitempty"`
}

const (
	maximumAPIKeysPerTenant = 20
	maximumAPIKeyRate       = 600
)

// CreateKey mints a developer key. The secret is shown exactly once.
func (s *PlatformService) CreateKey(user core.User, name, scope string, rate int) (APIKey, error) {
	if user.Role != core.RoleOwner {
		return APIKey{}, core.Fail(403, "Chỉ owner tạo khóa API")
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return APIKey{}, core.Fail(400, "Tên khóa không hợp lệ")
	}
	if rate <= 0 || rate > maximumAPIKeyRate {
		rate = 60
	}
	if scope != "" && scope != "read" {
		return APIKey{}, core.Fail(400, "API công khai chỉ hỗ trợ quyền đọc")
	}
	allowed := []string{"read"}
	secret := s.ids.NewID() + "." + s.ids.NewID()
	result := APIKey{Name: name, Prefix: "nexa_" + secret[:12], Scopes: allowed, RatePerMin: rate, Enabled: true}
	err := s.repo.Update(func(store core.Writer) error {
		count := 0
		for _, record := range store.Records(user.TenantID, core.KindAPIKeys) {
			if record.Enabled {
				count++
			}
		}
		if count >= maximumAPIKeysPerTenant {
			return core.Fail(409, "Tối đa 20 khóa API")
		}
		now := s.clock.Now().UTC()
		id := tokenHash("apikey:" + secret)
		result.ID = id
		result.Secret = secret
		store.PutRecord(core.Record{
			ID: id, TenantID: user.TenantID, Kind: core.KindAPIKeys,
			AuthorID: user.ID, Name: name, Content: result.Prefix,
			Keywords: allowed, Sequence: uint64(rate), Enabled: true, CreatedAt: now, UpdatedAt: now,
		})
		return nil
	})
	if err != nil {
		return APIKey{}, err
	}
	return result, nil
}

// ListKeys lists the tenant's keys without exposing full secrets.
func (s *PlatformService) ListKeys(user core.User) ([]APIKey, error) {
	items := []APIKey{}
	_ = s.repo.View(func(store core.Reader) error {
		for _, record := range store.Records(user.TenantID, core.KindAPIKeys) {
			if !record.Enabled {
				continue
			}
			prefix := record.Content
			if !strings.HasPrefix(prefix, "nexa_") {
				prefix = "nexa_" + prefix[:min(12, len(prefix))]
			}
			items = append(items, APIKey{
				ID: record.ID, Name: record.Name, Prefix: prefix,
				Scopes: record.Keywords, Enabled: true, RatePerMin: int(record.Sequence),
			})
		}
		return nil
	})
	return items, nil
}

// RevokeKey disables a key.
func (s *PlatformService) RevokeKey(user core.User, id string) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner thu hồi khóa API")
	}
	return s.repo.Update(func(store core.Writer) error {
		record, ok := store.Record(user.TenantID, id, core.KindAPIKeys)
		if !ok || record.AuthorID != user.ID || !record.Enabled {
			return core.Fail(404, "Không tìm thấy khóa API")
		}
		record.Enabled = false
		record.UpdatedAt = s.clock.Now().UTC()
		store.PutRecord(record)
		return nil
	})
}

// ResolveAPIKey authenticates a public request. The token format is
// "nexa_<secret>" where <secret> is "<random>.<random>". Returns the key and
// the tenant that owns it.
func (s *PlatformService) ResolveAPIKey(raw string) (APIKey, core.User, error) {
	if !strings.HasPrefix(raw, "nexa_") {
		return APIKey{}, core.User{}, core.Fail(401, "Khóa API không hợp lệ")
	}
	secret := strings.TrimPrefix(raw, "nexa_")
	if secret == "" || !strings.Contains(secret, ".") {
		return APIKey{}, core.User{}, core.Fail(401, "Khóa API không hợp lệ")
	}
	id := tokenHash("apikey:" + secret)
	var key APIKey
	var owner core.User
	invalid := core.Fail(401, "Khóa API không hợp lệ")
	err := s.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			record, exists := store.Record(tenant, id, core.KindAPIKeys)
			if !exists || !record.Enabled {
				continue
			}
			owner, exists = store.UserByID(record.AuthorID)
			if !exists || owner.Disabled || owner.TenantID != tenant || owner.Role != core.RoleOwner {
				return invalid
			}
			prefix := secret
			if len(prefix) > 12 {
				prefix = prefix[:12]
			}
			rateLimit := int(record.Sequence)
			if rateLimit == 0 {
				rateLimit = 60
			}
			key = APIKey{
				ID: record.ID, Name: record.Name, Prefix: "nexa_" + prefix,
				Scopes: record.Keywords, Enabled: true, RatePerMin: rateLimit,
			}
			return nil
		}
		return invalid
	})
	if err != nil {
		return APIKey{}, core.User{}, err
	}
	return key, owner, nil
}
