package service

import (
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestVideoKnowledgeValidatesAttachmentAndPublishesTranscriptOnlyOnDetail(t *testing.T) {
	ops, clock, owner := operationsFixture(t, time.Now())
	resources := NewResources(ops.repo, clock, RandomIDs{}, DefaultPolicies())
	item := core.Record{Subject: "Cài đặt", Content: "Hướng dẫn cài đặt", Enabled: true,
		Public: true, VideoURL: "javascript:alert(1)", VideoTranscript: "Chọn xác minh eSIM."}
	if _, err := resources.Save(owner, core.KindKnowledge, "", item); err == nil {
		t.Fatal("accepted unsafe video URL")
	}
	item.VideoURL = "https://media.example.test/help"
	saved, err := resources.Save(owner, core.KindKnowledge, "", item)
	if err != nil || scoreArticle(saved, map[string]struct{}{"esim": {}}) != 1 {
		t.Fatalf("transcript missing from search: %+v, %v", saved, err)
	}
	kb := NewKnowledge(ops.repo, RandomIDs{})
	list, err := kb.PublicList(owner.TenantID, "")
	if err != nil || len(list) != 1 || list[0].VideoURL != item.VideoURL || list[0].VideoTranscript != "" {
		t.Fatalf("unexpected public listing: %+v %v", list, err)
	}
	detail, err := kb.PublicList(owner.TenantID, saved.Slug)
	if err != nil || len(detail) != 1 || detail[0].VideoTranscript != item.VideoTranscript {
		t.Fatalf("unexpected public detail: %+v %v", detail, err)
	}
}
