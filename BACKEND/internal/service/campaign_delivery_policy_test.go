package service

import (
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestQRServiceWindowDiffersByProvider(t *testing.T) {
	start := time.Date(2026, 11, 20, 12, 0, 0, 0, ReportZone)
	for channel, duration := range map[string]time.Duration{
		core.ChannelZalo: 48 * time.Hour, core.ChannelWhatsApp: 24 * time.Hour, core.ChannelSMS: 0,
	} {
		if deadline := campaignServiceDeadline(channel, start); !deadline.Equal(start.Add(duration)) {
			t.Errorf("%s deadline = %v", channel, deadline)
		}
	}
}

func TestQRReminderDefersQuietHoursAndRespectsRecipientFrequency(t *testing.T) {
	now := time.Date(2026, 11, 20, 21, 0, 0, 0, ReportZone)
	ops, _, owner := operationsFixture(t, now)
	manager := NewChannels(ops.repo)
	const leadID = "lead"
	thread := core.Record{ID: "thread", TenantID: owner.TenantID, Kind: core.KindConversations, Status: core.StatusOpen, CustomerID: "customer", Channel: core.ChannelZalo}
	job := core.Record{ID: "job", TenantID: owner.TenantID, Kind: channelOutbox, Subject: "oa",
		Name: "recipient", ConversationID: thread.ID, Channel: core.ChannelZalo, Status: "pending",
		Sequence: 1, AutomationID: leadDeliveryPrefix + leadID}
	err := ops.repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
		store.PutRecord(thread)
		store.PutRecord(core.Record{ID: "campaign", TenantID: owner.TenantID, Kind: leadCampaignKind, Subject: "oa", Enabled: true})
		store.PutRecord(core.Record{ID: leadID, TenantID: owner.TenantID, Kind: campaignLeadKind,
			AutomationID: "campaign", Enabled: true, CreatedAt: now.Add(-time.Hour)})
		if manager.prepareCampaignDelivery(store, &job, leadID, now) {
			t.Fatal("quiet hours permitted a reminder")
		}
		morning := time.Date(2026, 11, 21, 8, 0, 0, 0, ReportZone)
		if job.EmailRetryAt == nil || !job.EmailRetryAt.Equal(morning) {
			t.Fatalf("expected 08:00 retry: %+v", job.EmailRetryAt)
		}
		for _, id := range []string{"sent1", "sent2"} {
			previous := job
			previous.ID, previous.Status, previous.UpdatedAt = id, "sent", morning.Add(-time.Hour)
			store.PutRecord(previous)
		}
		if manager.prepareCampaignDelivery(store, &job, leadID, morning) || job.EmailLastError != "campaign_frequency_cap" {
			t.Fatalf("frequency cap did not cancel: %+v", job)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
