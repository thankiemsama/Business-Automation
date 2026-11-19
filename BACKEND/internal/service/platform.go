package service

import (
	"crypto/subtle"
	"net/mail"
	"sort"
	"strings"
	"time"

	"businessautomation/internal/core"
)

// PlatformService aggregates the differentiation features: Customer 360° profile,
// the customer self-serve portal (email-code auth), multi-channel config, and
// public API keys.
type PlatformService struct {
	repo   core.Repository
	clock  Clock
	ids    IDGenerator
	mailer *EmailMailer
	origin string
}

func NewPlatform(repo core.Repository, clock Clock, ids IDGenerator) *PlatformService {
	return &PlatformService{repo: repo, clock: clock, ids: ids}
}

func (s *PlatformService) ConfigureEmail(mailer *EmailMailer, origin string) {
	s.mailer, s.origin = mailer, strings.TrimRight(origin, "/")
}

// ---- Customer 360° Profile ---------------------------------------------

// CustomerProfile is the cross-channel, cross-record view of one customer.
type CustomerProfile struct {
	Customer          core.Record     `json:"customer"`
	Tickets           int             `json:"total_tickets"`
	Open              int             `json:"open_tickets"`
	Rating            float64         `json:"average_rating"`
	ConversationCount int             `json:"conversations"`
	Channels          []string        `json:"channels"`
	Tags              []string        `json:"tags"`
	Notes             []Note          `json:"notes"`
	LTV               LTV             `json:"ltv"`
	Timeline          []TimelineEvent `json:"timeline"`
}

// Note is a customer annotation (agent or system).
type Note struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
}

// LTV holds simple lifetime-value signals derived from tickets/ratings.
type LTV struct {
	Tickets    int       `json:"tickets"`
	Resolved   int       `json:"resolved"`
	AvgRating  float64   `json:"avg_rating"`
	LastActive time.Time `json:"last_active"`
}

// TimelineEvent is one activity across every channel.
type TimelineEvent struct {
	Kind      string `json:"kind"`
	Channel   string `json:"channel,omitempty"`
	Subject   string `json:"subject"`
	Content   string `json:"content,omitempty"`
	Sender    string `json:"sender,omitempty"`
	CreatedAt string `json:"created_at"`
}

func (s *PlatformService) Profile(user core.User, customerID string) (CustomerProfile, error) {
	var profile CustomerProfile
	err := s.repo.View(func(store core.Reader) error {
		customer, err := findOwnedRecord(store, user, customerID, core.KindCustomers)
		if err != nil {
			return err
		}
		profile.Customer = customer
		profile.Notes = []Note{}
		profile.Channels = []string{}
		profile.Tags = customer.Tags

		channels := map[string]bool{}
		timeline := []TimelineEvent{}
		ltv := LTV{}

		for _, record := range store.Records(user.TenantID, core.KindConversations) {
			if record.CustomerID != customerID {
				continue
			}
			profile.ConversationCount++
			channels[record.Channel] = true
			ltv.last(record.UpdatedAt, &ltv)
			timeline = append(timeline, TimelineEvent{
				Kind: "conversation", Channel: record.Channel,
				Subject: record.Subject, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		ratingSum, ratingCount, open := 0, 0, 0
		for _, record := range store.Records(user.TenantID, core.KindTickets) {
			if record.CustomerID != customerID {
				continue
			}
			ltv.Tickets++
			ltv.last(record.CreatedAt, &ltv)
			if record.Status == core.StatusResolved {
				ltv.Resolved++
			} else {
				open++
			}
			if record.Rating > 0 {
				ratingSum += record.Rating
				ratingCount++
			}
			timeline = append(timeline, TimelineEvent{
				Kind: "ticket", Subject: record.Subject,
				Content: record.Content, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		for _, record := range store.Records(user.TenantID, core.KindMessages) {
			conversation, _ := store.Record(user.TenantID, record.ConversationID, core.KindConversations)
			if conversation.CustomerID != customerID {
				continue
			}
			ltv.last(record.CreatedAt, &ltv)
			timeline = append(timeline, TimelineEvent{
				Kind: "message", Channel: conversation.Channel,
				Subject: conversation.Subject, Content: record.Content, Sender: record.Sender,
				CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		for _, record := range store.Records(user.TenantID, core.KindCustomerNotes) {
			if record.CustomerID != customerID || !record.Enabled {
				continue
			}
			ltv.last(record.CreatedAt, &ltv)
			{
				profile.Notes = append(profile.Notes, Note{
					ID: record.ID, Content: record.Content, Author: record.AuthorID,
					CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339),
				})
			}
		}
		profile.Open = open
		profile.Tickets = ltv.Tickets
		if ratingCount > 0 {
			profile.Rating, ltv.AvgRating = float64(ratingSum)/float64(ratingCount), float64(ratingSum)/float64(ratingCount)
		}
		for channel := range channels {
			profile.Channels = append(profile.Channels, channel)
		}
		profile.LTV = ltv
		profile.Timeline = timeline
		sort.Slice(profile.Notes, func(i, j int) bool { return profile.Notes[i].CreatedAt > profile.Notes[j].CreatedAt })
		profile.Notes = profile.Notes[:min(20, len(profile.Notes))]
		sort.Slice(profile.Timeline, func(i, j int) bool { return profile.Timeline[i].CreatedAt > profile.Timeline[j].CreatedAt })
		if len(profile.Timeline) > 100 {
			profile.Timeline = profile.Timeline[:100]
		}
		sort.Slice(profile.Channels, func(i, j int) bool { return profile.Channels[i] < profile.Channels[j] })
		return nil
	})
	return profile, err
}

func (l *LTV) last(at time.Time, target *LTV) {
	if at.After(target.LastActive) {
		target.LastActive = at
	}
}

// AddNote records a customer annotation.
func (s *PlatformService) AddNote(user core.User, customerID, content string) (core.Record, error) {
	content = strings.TrimSpace(content)
	if content == "" || len([]rune(content)) > 2000 {
		return core.Record{}, core.Fail(400, "Ghi chú phải có nội dung")
	}
	var result core.Record
	err := s.repo.Update(func(store core.Writer) error {
		if _, err := findOwnedRecord(store, user, customerID, core.KindCustomers); err != nil {
			return err
		}
		now := s.clock.Now().UTC()
		result = core.Record{
			ID: s.ids.NewID(), TenantID: user.TenantID, Kind: core.KindCustomerNotes,
			CustomerID: customerID, Content: content, AuthorID: user.ID,
			Enabled: true, CreatedAt: now, UpdatedAt: now,
		}
		store.PutRecord(result)
		return nil
	})
	return result, err
}

// ---- Customer Self-Serve Portal ----------------------------------------

// PortalCustomer is the public, session-scoped customer view.
type PortalCustomer struct {
	Token    string `json:"token"`
	Customer string `json:"customer"`
	Email    string `json:"email"`
	Expires  int64  `json:"expires_at"`
}

// RequestCode emails a one-time portal code for a customer's email.
func (s *PlatformService) RequestCode(tenant, email string) error {
	if s.mailer == nil || s.origin == "" {
		return core.Fail(503, "Chưa cấu hình email cho cổng khách hàng")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(email); err != nil || len(email) > 254 {
		return core.Fail(400, "Email không hợp lệ")
	}
	token := s.ids.NewID()
	err := s.repo.Update(func(store core.Writer) error {
		customer, ok := findCustomerByEmail(store, tenant, email)
		if !ok {
			token = ""
			return nil
		}
		now := s.clock.Now().UTC()
		store.PutRecord(core.Record{
			ID: portalCodeID(tenant, email), TenantID: tenant, Kind: core.KindPortalCodes,
			CustomerID: customer.ID, Content: tokenHash("portal-code:" + token[:8]), Email: email,
			Enabled: true, CreatedAt: now, UpdatedAt: now.Add(core.PortalCodeTTL),
		})
		return nil
	})
	if err != nil {
		return err
	}
	if token == "" {
		return nil
	}
	code := token[:8]
	return s.mailer.Send(email, "Mã truy cập cổng khách hàng Nexa",
		"Mã truy cập của bạn là "+code+". Mở liên kết để đăng nhập:\r\n"+s.origin+"/portal/"+tenant,
		tokenHash("portal-mail:"+token), "", "")
}

func portalCodeID(tenant, email string) string { return tokenHash("portal:" + tenant + ":" + email) }

// Login validates a portal code and returns a session token.
func (s *PlatformService) Login(tenant, email, code string) (PortalCustomer, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.ToLower(strings.TrimSpace(code))
	if len(code) != 8 {
		return PortalCustomer{}, core.Fail(401, "Mã không hợp lệ hoặc đã hết hạn")
	}
	var result PortalCustomer
	err := s.repo.Update(func(store core.Writer) error {
		record, ok := store.Record(tenant, portalCodeID(tenant, email), core.KindPortalCodes)
		if !ok || !record.Enabled || !s.clock.Now().Before(record.UpdatedAt) {
			return core.Fail(401, "Mã không hợp lệ hoặc đã hết hạn")
		}
		if subtle.ConstantTimeCompare([]byte(record.Content), []byte(tokenHash("portal-code:"+code))) != 1 {
			return core.Fail(401, "Mã không hợp lệ hoặc đã hết hạn")
		}
		customer, exists := store.Record(tenant, record.CustomerID, core.KindCustomers)
		if !exists || customer.Status == core.CustomerMerged || !strings.EqualFold(customer.Email, email) {
			return core.Fail(401, "Mã không hợp lệ hoặc đã hết hạn")
		}
		record.Enabled = false
		store.PutRecord(record)
		session := s.ids.NewID()
		now := s.clock.Now().UTC()
		store.PutRecord(core.Record{
			ID: tokenHash("portal-session:" + session), TenantID: tenant, Kind: "portal_sessions",
			CustomerID: customer.ID, Email: email, Enabled: true,
			CreatedAt: now, UpdatedAt: now.Add(core.SessionLifetime),
		})
		result = PortalCustomer{Token: session, Customer: customer.ID, Email: email, Expires: now.Add(core.SessionLifetime).Unix()}
		return nil
	})
	return result, err
}

// sessionCustomer resolves a portal session to the underlying customer record.
func (s *PlatformService) sessionCustomer(tenant, token string) (core.Record, error) {
	var customer core.Record
	err := s.repo.View(func(store core.Reader) error {
		var err error
		customer, err = s.sessionCustomerIn(store, tenant, token)
		return err
	})
	return customer, err
}

func (s *PlatformService) sessionCustomerIn(store core.Reader, tenant, token string) (core.Record, error) {
	session, ok := store.Record(tenant, tokenHash("portal-session:"+token), "portal_sessions")
	if !ok || !session.Enabled || s.clock.Now().After(session.UpdatedAt) {
		return core.Record{}, core.Fail(401, "Phiên khách hàng không hợp lệ")
	}
	customer, exists := store.Record(tenant, session.CustomerID, core.KindCustomers)
	if !exists || customer.Status == core.CustomerMerged || !strings.EqualFold(customer.Email, session.Email) {
		return core.Record{}, core.Fail(401, "Phiên khách hàng không hợp lệ")
	}
	return customer, nil
}

// PortalTickets lists the customer's tickets newest-first.
func (s *PlatformService) PortalTickets(tenant, token string) ([]core.Record, error) {
	customer, err := s.sessionCustomer(tenant, token)
	if err != nil {
		return nil, err
	}
	items := []core.Record{}
	_ = s.repo.View(func(store core.Reader) error {
		for _, ticket := range store.Records(tenant, core.KindTickets) {
			if ticket.CustomerID == customer.ID {
				items = append(items, portalTicketRecord(ticket))
			}
		}
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

// PortalTicket returns one owned ticket plus its thread messages.
func (s *PlatformService) PortalTicket(tenant, token, id string) (core.Record, []core.Record, error) {
	customer, err := s.sessionCustomer(tenant, token)
	if err != nil {
		return core.Record{}, nil, err
	}
	var ticket core.Record
	list := []core.Record{}
	err = s.repo.View(func(store core.Reader) error {
		owned, ok := store.Record(tenant, id, core.KindTickets)
		if !ok || owned.CustomerID != customer.ID {
			return core.Fail(404, "Không tìm thấy phiếu")
		}
		ticket = portalTicketRecord(owned)
		for _, message := range store.Records(tenant, core.KindMessages) {
			if owned.ConversationID != "" && message.ConversationID == owned.ConversationID && message.Sender != "note" {
				message.Sentiment = ""
				list = append(list, message)
			}
		}
		sort.Slice(list, func(i, j int) bool { return laterMessage(list[j], list[i]) })
		return nil
	})
	return ticket, list, err
}
