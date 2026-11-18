package service

import (
	"net/url"
	"strings"

	"businessautomation/internal/core"
)

// CSATService issues one-time, shareable survey links when a ticket is resolved
// and captures the customer's rating plus optional comment.
type CSATService struct {
	repo   core.Repository
	clock  Clock
	ids    IDGenerator
	origin string
	mailer *EmailMailer
}

func NewCSAT(repo core.Repository, clock Clock, ids IDGenerator) *CSATService {
	return &CSATService{repo: repo, clock: clock, ids: ids}
}

func (s *CSATService) ConfigureEmail(mailer *EmailMailer) { s.mailer = mailer }

func (s *CSATService) Configure(origin string) { s.origin = strings.TrimRight(origin, "/") }

// CSATLink is the public survey target. The token embeds tenant + secret so the
// link stays shareable without a customer session.
type CSATLink struct {
	Token   string `json:"token"`
	URL     string `json:"url"`
	Subject string `json:"subject"`
	Expires int64  `json:"expires_at"`
}

// IssueLink mints a survey link for a resolved ticket and returns the public URL.
func (s *CSATService) IssueLink(ticket core.Record) (CSATLink, error) {
	tenant := ticket.TenantID
	token := s.ids.NewID()
	expires := s.clock.Now().UTC().Add(core.CSATLinkTTL)
	now := s.clock.Now().UTC()
	err := s.repo.Update(func(store core.Writer) error {
		current, ok := store.Record(tenant, ticket.ID, core.KindTickets)
		if !ok || current.Status != core.StatusResolved {
			return core.Fail(409, "Phiếu chưa được giải quyết")
		}
		recordID := tokenHash("csat:" + token)
		if _, exists := store.Record(tenant, recordID, core.KindCSATLinks); exists {
			return nil
		}
		store.PutRecord(core.Record{
			ID:             recordID,
			TenantID:       tenant,
			Kind:           core.KindCSATLinks,
			ConversationID: ticket.ID,
			Subject:        ticket.Subject,
			Email:          ticket.Email,
			Enabled:        true,
			CreatedAt:      now,
			UpdatedAt:      expires,
		})
		if s.mailer != nil && s.origin != "" {
			customer, ok := store.Record(tenant, current.CustomerID, core.KindCustomers)
			if ok && customer.Email != "" && current.Rating == 0 {
				link := s.publicLink(tenant, token, current.Subject, expires.Unix())
				store.PutRecord(core.Record{ID: tokenHash("csat-email:" + token), TenantID: tenant, Kind: "csat_mail", Email: customer.Email, CustomerID: customer.ID, ConversationID: current.ID, Content: link.URL, Enabled: true, CreatedAt: now, UpdatedAt: now})
			}
		}
		return nil
	})
	if err != nil {
		return CSATLink{}, err
	}
	return s.publicLink(tenant, token, ticket.Subject, expires.Unix()), nil
}

func (s *CSATService) publicLink(tenant, token, subject string, expires int64) CSATLink {
	base := s.origin
	if base == "" {
		base = "http://127.0.0.1:3000"
	}
	target, _ := url.JoinPath(base, "sat", tenant+"."+token)
	return CSATLink{Token: token, URL: target, Subject: subject, Expires: expires}
}

// PublicResult is what the /sat page can read without authentication.
type PublicResult struct {
	Conversation string `json:"conversation"`
	Subject      string `json:"subject"`
	Rated        bool   `json:"rated"`
}

// ResolveLink validates a token and returns the public survey context.
func (s *CSATService) ResolveLink(tenant, token string) (PublicResult, error) {
	result := PublicResult{}
	invalid := core.Fail(404, "Liên kết khảo sát hết hạn")
	err := s.repo.View(func(store core.Reader) error {
		link, ok := store.Record(tenant, tokenHash("csat:"+token), core.KindCSATLinks)
		if !ok || !link.Enabled || s.clock.Now().After(link.UpdatedAt) {
			return invalid
		}
		ticket, ok := store.Record(tenant, link.ConversationID, core.KindTickets)
		if !ok || ticket.Status != core.StatusResolved {
			return invalid
		}
		result.Conversation = ticket.ConversationID
		result.Subject = ticket.Subject
		result.Rated = ticket.Rating > 0
		return nil
	})
	if err != nil {
		return PublicResult{}, err
	}
	return result, nil
}

// Submit records the rating on the underlying conversation exactly once.
func (s *CSATService) Submit(tenant, secret string, rating int, comment string) (core.Record, error) {
	if rating < 1 || rating > 5 {
		return core.Record{}, core.Fail(400, "Đánh giá phải từ 1 đến 5 sao")
	}
	comment = strings.TrimSpace(comment)
	if len([]rune(comment)) > 1000 {
		return core.Record{}, core.Fail(400, "Nhận xét tối đa 1000 ký tự")
	}
	var result core.Record
	err := s.repo.Update(func(store core.Writer) error {
		link, ok := store.Record(tenant, tokenHash("csat:"+secret), core.KindCSATLinks)
		if !ok || !link.Enabled || !s.clock.Now().Before(link.UpdatedAt) {
			return core.Fail(404, "Liên khảo sát không hợp lệ")
		}
		ticket, ok := store.Record(tenant, link.ConversationID, core.KindTickets)
		if !ok || ticket.Status != core.StatusResolved {
			return core.Fail(404, "Không tìm thấy phiếu đã giải quyết")
		}
		if ticket.Rating > 0 {
			return core.Fail(409, "Phiếu đã được đánh giá")
		}
		now := s.clock.Now().UTC()
		ticket.Rating, ticket.RatedAt, ticket.RatingComment = rating, &now, comment
		store.PutRecord(ticket)
		result = ticket
		enqueueWebhooks(store, tenant, core.EventCSATSubmitted, map[string]any{
			"ticket": ticket.ID, "conversation": ticket.ConversationID, "rating": rating,
		})
		return nil
	})
	return result, err
}
