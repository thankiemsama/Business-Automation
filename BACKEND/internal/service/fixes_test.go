package service

import (
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

// concurrentDraftProvider injects a newer customer message while the model is
// producing the draft, standing in for the real window in which a customer can
// reply while the model is generating either an answer or a handoff.
type concurrentDraftProvider struct {
	workspace  *Workspace
	needsHuman bool
}

func (p concurrentDraftProvider) Answer(question string, articles []core.Record) (Answer, error) {
	err := p.workspace.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{
			ID: p.workspace.IDs.NewID(), TenantID: "tenant", Kind: core.KindMessages,
			ConversationID: "conv", Content: "nang cap them", Sender: core.SenderCustomer, Sequence: nextSequence(store, core.User{TenantID: "tenant"}, "conv"),
		})
		return nil
	})
	return Answer{Content: "Chờ nhân viên.", Provider: "concurrent-test", NeedsHuman: p.needsHuman}, err
}

// TestDraftAnchorsSourceAndDetectsContextChange verifies two guarantees:
//  1. A draft carries the ID of the source customer message it was built from.
//  2. If a newer customer message arrives before the draft is applied, the
//     stale draft is rejected with 409 instead of silently drifting out of
//     context.
func TestDraftAnchorsSourceAndDetectsContextChange(t *testing.T) {
	repo, err := repository.Open(t.TempDir() + "/draft.json")
	if err != nil {
		t.Fatal(err)
	}
	workspace := NewWorkspace(repo, &workspaceClock{now: time.Now().UTC()}, RandomIDs{})
	user := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(user)
		store.PutRecord(core.Record{ID: "customer", TenantID: user.TenantID, Kind: core.KindCustomers, Name: "K1", Email: "k1@example.test"})
		store.PutRecord(core.Record{ID: "conv", TenantID: user.TenantID, Kind: core.KindConversations, CustomerID: "customer", Channel: core.ChannelWeb, Subject: "H1", Status: core.StatusOpen})
		store.PutRecord(core.Record{ID: "m1", TenantID: user.TenantID, Kind: core.KindMessages, ConversationID: "conv", Content: "ho tro tai khoan", Sender: core.SenderCustomer, Sequence: 1})
		// ASCII content so the local knowledge matcher scores a hit.
		store.PutRecord(core.Record{ID: "kb", TenantID: user.TenantID, Kind: core.KindKnowledge, Subject: "Tai khoan", Content: "Huong dan ho tro tai khoan tai day", Enabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Knowledge-backed answer: anchored to the source message, no handoff.
	svc := NewConversations(workspace, workspace.Clock, workspace.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(workspace.IDs, workspace.Clock))
	reply, err := svc.Draft(user, "conv")
	if err != nil {
		t.Fatalf("draft failed: %v", err)
	}
	if reply.SourceMessageID != "m1" {
		t.Fatalf("draft not anchored to source message: %+v", reply)
	}
	if reply.NeedsHuman || reply.SourceID != "kb" {
		t.Fatalf("expected a knowledge answer, got %+v", reply)
	}

	// Internal notes are outside the customer-visible context.
	if _, err := svc.Message(user, "conv", "Ghi chú riêng", "note"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendMessage(user, "conv", "Reply", core.SenderAgent, "", "", reply.SourceMessageID); err != nil {
		t.Fatal(err)
	}

	for _, needsHuman := range []bool{false, true} {
		if _, err := svc.Message(user, "conv", "New question", core.SenderCustomer); err != nil {
			t.Fatal(err)
		}
		concurrent := NewConversations(workspace, workspace.Clock, workspace.IDs,
			concurrentDraftProvider{workspace: workspace, needsHuman: needsHuman}, svc.automations)
		if _, err := concurrent.Draft(user, "conv"); err == nil {
			t.Fatalf("model result should be stale: needsHuman=%t", needsHuman)
		}
	}
	if _, err := svc.SendMessage(user, "conv", "Old reply", core.SenderAgent, "", "", reply.SourceMessageID); err == nil {
		t.Fatal("stale draft was accepted by send")
	}
}

// TestIngestEmailReusesCustomerProfile verifies that two email threads from the
// same sender address resolve to a single customer profile instead of creating
// duplicates.
func TestIngestEmailReusesCustomerProfile(t *testing.T) {
	repo, err := repository.Open(t.TempDir() + "/email.json")
	if err != nil {
		t.Fatal(err)
	}
	workspace := NewWorkspace(repo, &workspaceClock{now: time.Now().UTC()}, RandomIDs{})
	user := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(user)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewConversations(workspace, workspace.Clock, workspace.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(workspace.IDs, workspace.Clock))

	first, err := svc.IngestEmail(user.TenantID, InboundEmail{ID: "evt-1", ThreadID: "thread-a", From: "Nguyen Van A <a@example.test>", Subject: "Lần 1", Text: "Lần đầu"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.IngestEmail(user.TenantID, InboundEmail{ID: "evt-2", ThreadID: "thread-b", From: "a@example.test", Subject: "Lần 2", Text: "Lần hai"})
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.View(func(store core.Reader) error {
		profiles := store.Records(user.TenantID, core.KindCustomers)
		if len(profiles) != 1 {
			t.Fatalf("expected one customer profile, got %d: %+v", len(profiles), profiles)
		}
		// Both threads must attach to the same profile.
		for _, conv := range store.Records(user.TenantID, core.KindConversations) {
			if conv.CustomerID != profiles[0].ID {
				t.Fatalf("thread attached to a different customer: conv=%q customer=%q want=%q", conv.ID, conv.CustomerID, profiles[0].ID)
			}
		}
		_ = first
		_ = second
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestMergeCustomersRepointsRecords verifies that merging two profiles
// re-points referencing records and flags the source as merged.
func TestMergeCustomersRepointsRecords(t *testing.T) {
	repo, err := repository.Open(t.TempDir() + "/merge.json")
	if err != nil {
		t.Fatal(err)
	}
	workspace := NewWorkspace(repo, &workspaceClock{now: time.Now().UTC()}, RandomIDs{})
	user := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	platform := NewPlatform(workspace, workspace.Clock, workspace.IDs)
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(user)
		store.PutRecord(core.Record{ID: "src", TenantID: user.TenantID, Kind: core.KindCustomers, Name: "Nguồn", Email: "src@example.test", Tags: []string{"vip"}})
		store.PutRecord(core.Record{ID: "dst", TenantID: user.TenantID, Kind: core.KindCustomers, Name: "Đích", Email: "src@example.test"})
		store.PutRecord(core.Record{ID: "conv", TenantID: user.TenantID, Kind: core.KindConversations, CustomerID: "src", Subject: "C1", Status: core.StatusOpen, Channel: core.ChannelWeb})
		store.PutRecord(core.Record{ID: "session", TenantID: user.TenantID, Kind: "portal_sessions", CustomerID: "src", Email: "src@example.test", UpdatedAt: workspace.Clock.Now().Add(time.Hour)})
		store.PutRecord(core.Record{ID: "different", TenantID: user.TenantID, Kind: core.KindCustomers, Email: "other@example.test"})
		store.PutRecord(core.Record{ID: "note", TenantID: user.TenantID, Kind: core.KindCustomerNotes, CustomerID: "src", Content: "g1", AuthorID: user.ID, Enabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := platform.MergeCustomers(user, "src", "different"); err == nil {
		t.Fatal("merged profiles with different email identities")
	}
	if _, err := platform.MergeCustomers(user, "src", "dst"); err != nil {
		t.Fatal(err)
	}
	if err := workspace.View(func(store core.Reader) error {
		session, _ := store.Record(user.TenantID, "session", "portal_sessions")
		if session.CustomerID != "dst" || !session.UpdatedAt.Equal(workspace.Clock.Now().Add(time.Hour)) {
			t.Fatal("merge changed session expiry or lost session ownership")
		}
		match, ok := findCustomerByEmail(store, user.TenantID, "SRC@example.test")
		if !ok || match.ID != "dst" || match.Name != "Đích" {
			t.Fatalf("email match or retained profile changed unexpectedly: %+v", match)
		}
		conv, _ := store.Record(user.TenantID, "conv", core.KindConversations)
		if conv.CustomerID != "dst" {
			t.Fatalf("conversation not re-pointed: %+v", conv)
		}
		note, _ := store.Record(user.TenantID, "note", core.KindCustomerNotes)
		if note.CustomerID != "dst" {
			t.Fatalf("note not re-pointed: %+v", note)
		}
		source, _ := store.Record(user.TenantID, "src", core.KindCustomers)
		if source.Status != "merged" {
			t.Fatalf("source not flagged merged: %+v", source)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	resources := NewResources(workspace, workspace.Clock, workspace.IDs, DefaultPolicies())
	if _, err := resources.List(user, core.KindCustomers, ""); err != nil {
		t.Fatal(err)
	}
	// An agent may not merge; only the owner can.
	agent := core.User{ID: "agent", TenantID: user.TenantID, Role: core.RoleAgent}
	if _, err := platform.MergeCustomers(agent, "src", "dst"); err == nil {
		t.Fatal("agent merged a customer")
	}
	// Re-merging a source that is already merged is rejected.
	if _, err := platform.MergeCustomers(user, "src", "dst"); err == nil {
		t.Fatal("re-merge of a merged source succeeded")
	}
}
