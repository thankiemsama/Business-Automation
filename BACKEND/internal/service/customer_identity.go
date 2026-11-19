package service

import (
	"strings"

	"businessautomation/internal/core"
)

// Customer identity resolution and profile merging.
//
// The email channel identifies a customer by their sender address, so a new
// thread from the same person never creates a second profile. When legacy or
// manually created duplicates exist, an owner can merge them: every record
// that references the source profile is re-pointed to the target, and the
// source is flagged so listings and dashboards can exclude it.
//
// billing_plan records are intentionally skipped: their CustomerID holds the
// Stripe customer id, not a Nexa customer id.

// customerMergeKinds are the record kinds whose CustomerID points at a Nexa
// customer profile and must be re-pointed during a merge.
var customerMergeKinds = []string{
	core.KindConversations,
	core.KindTickets,
	core.KindCustomerNotes,
	"crm_sync",
	core.KindSurveyEmailJobs,
	"portal_codes",
	"portal_sessions",
}

// MergeCustomers consolidates the source profile into the target profile.
// The operation is atomic: one Writer transaction re-points references,
// merges display fields, and flags the source as merged.
func (s *PlatformService) MergeCustomers(user core.User, sourceID, targetID string) (core.Record, error) {
	if user.Role != core.RoleOwner {
		return core.Record{}, core.Fail(403, "Chỉ owner gộp hồ sơ khách hàng")
	}
	if sourceID == targetID {
		return core.Record{}, core.Fail(400, "Không thể gộp khách hàng với chính nó")
	}
	var result core.Record
	err := updateAs(s.repo, user.ID, func(store core.Writer) error {
		source, err := findOwnedRecord(store, user, sourceID, core.KindCustomers)
		if err != nil {
			return err
		}
		target, err := findOwnedRecord(store, user, targetID, core.KindCustomers)
		if err != nil {
			return err
		}
		sourceEmail, targetEmail := strings.TrimSpace(source.Email), strings.TrimSpace(target.Email)
		if sourceEmail != "" && targetEmail != "" && !strings.EqualFold(sourceEmail, targetEmail) {
			return core.Fail(409, "Chỉ gộp hồ sơ cùng email hoặc có một hồ sơ chưa có email")
		}
		now := s.clock.Now().UTC()

		// 1. Re-point every record that references the source profile.
		for _, kind := range customerMergeKinds {
			records := store.Records(user.TenantID, kind)
			for i := range records {
				if records[i].CustomerID != sourceID {
					continue
				}
				records[i].CustomerID = targetID
				store.PutRecord(records[i])
			}
		}

		// 2. Best-first field merge: richer source data fills target gaps.
		if (target.Name == "" || strings.EqualFold(target.Name, targetEmail)) && source.Name != "" && !strings.EqualFold(source.Name, sourceEmail) {
			target.Name = source.Name
		}
		if targetEmail == "" {
			target.Email = sourceEmail
		}
		if source.Phone != "" && target.Phone == "" {
			target.Phone = source.Phone
		}
		seen := map[string]bool{}
		for _, tag := range target.Tags {
			seen[strings.ToLower(tag)] = true
		}
		for _, tag := range source.Tags {
			if !seen[strings.ToLower(tag)] {
				seen[strings.ToLower(tag)] = true
				target.Tags = append(target.Tags, tag)
			}
		}
		target.UpdatedAt = now
		store.PutRecord(target)

		// 3. Flag the source so it stays auditable but out of active lists.
		source.Status = core.CustomerMerged
		source.UpdatedAt = now
		store.PutRecord(source)

		result, _ = store.Record(user.TenantID, target.ID, core.KindCustomers)
		return nil
	})
	return result, err
}
