package service_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
	"businessautomation/internal/service"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

type counterIDs struct{ n atomic.Uint64 }

func (c *counterIDs) NewID() string { return fmt.Sprintf("id-%06d", c.n.Add(1)) }

type testHasher struct{}

func (testHasher) Hash(p, s string) string { return s + ":" + p }
func fixture(t *testing.T) (*repository.File, *fakeClock, *counterIDs, core.User) {
	t.Helper()
	repo, err := repository.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)}
	ids := &counterIDs{}
	u := core.User{ID: "owner", TenantID: "tenant", Role: "owner"}
	err = repo.Update(func(st core.Writer) error {
		st.PutRecord(core.Record{ID: "customer", Kind: "customers", TenantID: u.TenantID, Name: "Customer"})
		st.PutRecord(core.Record{ID: "conversation", Kind: "conversations", TenantID: u.TenantID, CustomerID: "customer", Status: "open"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, clock, ids, u
}

func TestSessionExpiryUsesInjectedClock(t *testing.T) {
	repo, clock, ids, _ := fixture(t)
	auth := service.NewAuth(repo, clock, ids, testHasher{})
	c := service.Credentials{Name: "Owner", Email: "owner@example.com", Password: "a-long-password"}
	if _, err := auth.Register(c); err != nil {
		t.Fatal(err)
	}
	token, err := auth.Login(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auth.Authenticate(token); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(24 * time.Hour)
	if _, err = auth.Authenticate(token); err == nil {
		t.Fatal("expired token accepted")
	}
}

type providerStub struct {
	fail  bool
	calls int
}

func (p *providerStub) Answer(question string, articles []core.Record) (service.Answer, error) {
	p.calls++
	for _, a := range articles {
		if a.TenantID != "tenant" {
			return service.Answer{}, errors.New("cross tenant leak")
		}
	}
	if p.fail {
		return service.Answer{}, errors.New("provider unavailable")
	}
	return service.Answer{Content: "swapped provider", Provider: "stub"}, nil
}

func TestReplyProviderReplacementAndFailureRollback(t *testing.T) {
	repo, clock, ids, u := fixture(t)
	provider := &providerStub{fail: true}
	conv := service.NewConversations(repo, clock, ids, provider, service.NewKeywordAutomation(ids, clock))
	if _, err := conv.Message(u, "conversation", "question", "customer"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(func(st core.Writer) error {
		st.PutRecord(core.Record{ID: "private", TenantID: "other", Kind: "knowledge", Content: "secret", Enabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := conv.Reply(u, "conversation"); err == nil {
		t.Fatal("expected provider failure")
	}
	_ = repo.View(func(st core.Reader) error {
		if len(st.Records(u.TenantID, "messages")) != 1 {
			t.Error("failed reply was persisted")
		}
		return nil
	})
	provider.fail = false
	reply, err := conv.Reply(u, "conversation")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Provider != "stub" || reply.Message.Content != "swapped provider" {
		t.Fatal(reply)
	}
	if _, err = conv.Reply(u, "conversation"); err == nil {
		t.Fatal("reply repeated with equal clock timestamps")
	}
	if provider.calls != 2 {
		t.Fatal(provider.calls)
	}
}

type failingAutomation struct{}

func (failingAutomation) Run(st core.Writer, u core.User, c, m core.Record) error {
	st.PutRecord(core.Record{ID: "partial", TenantID: u.TenantID, Kind: "tickets"})
	return errors.New("automation failed")
}

func TestAutomationFailureRollsBackWholeMessage(t *testing.T) {
	repo, clock, ids, u := fixture(t)
	conv := service.NewConversations(repo, clock, ids, service.LocalKnowledgeProvider{}, failingAutomation{})
	if _, err := conv.Message(u, "conversation", "hello", "customer"); err == nil {
		t.Fatal("expected automation error")
	}
	_ = repo.View(func(st core.Reader) error {
		if len(st.Records(u.TenantID, "messages")) != 0 || len(st.Records(u.TenantID, "tickets")) != 0 {
			t.Error("partial transaction committed")
		}
		return nil
	})
}

func TestConcurrentAutomationCreatesOneOpenTicket(t *testing.T) {
	repo, clock, ids, u := fixture(t)
	_ = repo.Update(func(st core.Writer) error {
		st.PutRecord(core.Record{ID: "rule", TenantID: u.TenantID, Kind: "automations", Name: "Refund", Keywords: []string{"refund"}, Enabled: true})
		return nil
	})
	conv := service.NewConversations(repo, clock, ids, service.LocalKnowledgeProvider{}, service.NewKeywordAutomation(ids, clock))
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := conv.Message(u, "conversation", "refund", "customer"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	_ = repo.View(func(st core.Reader) error {
		tickets := st.Records(u.TenantID, "tickets")
		if len(tickets) != 1 || tickets[0].AutomationID != "rule" {
			t.Error(tickets)
		}
		seen := map[uint64]bool{}
		for _, m := range st.Records(u.TenantID, "messages") {
			if seen[m.Sequence] || m.Sequence == 0 {
				t.Error("duplicate/zero sequence")
			}
			seen[m.Sequence] = true
		}
		if len(seen) != 12 {
			t.Error(len(seen))
		}
		return nil
	})
}

type customPolicy struct{}

func (customPolicy) CanWrite(u core.User) bool { return true }
func (customPolicy) Validate(st core.Reader, u core.User, r *core.Record) error {
	if r.Name != "custom" {
		return errors.New("invalid custom entity")
	}
	return nil
}

func TestResourcePolicyExtendsWithoutChangingService(t *testing.T) {
	repo, clock, ids, u := fixture(t)
	policies := service.DefaultPolicies()
	policies["custom"] = customPolicy{}
	resources := service.NewResources(repo, clock, ids, policies)
	delete(policies, "custom")
	r, err := resources.Save(u, "custom", "", core.Record{Name: "custom"})
	if err != nil || r.Kind != "custom" {
		t.Fatalf("%v %v", r, err)
	}
}

func TestLegacyAutomationTicketRemainsDeduplicated(t *testing.T) {
	repo, clock, ids, u := fixture(t)
	_ = repo.Update(func(st core.Writer) error {
		st.PutRecord(core.Record{ID: "rule", TenantID: u.TenantID, Kind: "automations", Name: "Refund", Keywords: []string{"refund"}, Enabled: true})
		st.PutRecord(core.Record{ID: "legacy", TenantID: u.TenantID, Kind: "tickets", Name: "rule", ConversationID: "conversation", Status: "open"})
		return nil
	})
	conv := service.NewConversations(repo, clock, ids, service.LocalKnowledgeProvider{}, service.NewKeywordAutomation(ids, clock))
	if _, err := conv.Message(u, "conversation", "refund", "customer"); err != nil {
		t.Fatal(err)
	}
	_ = repo.View(func(st core.Reader) error {
		if len(st.Records(u.TenantID, "tickets")) != 1 {
			t.Error("legacy ticket duplicated")
		}
		return nil
	})
}
