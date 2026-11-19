package service

import (
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestReplyTemplateTransferAcrossTenants(t *testing.T) {
	operations, _, source := operationsFixture(t, time.Now())
	target := core.User{ID: "other-owner", TenantID: "other-tenant", Role: core.RoleOwner}
	if err := operations.repo.Update(func(w core.Writer) error {
		w.PutUser(target)
		w.PutRecord(core.Record{
			ID: "reply", TenantID: source.TenantID, Kind: "templates",
			Subject: "Hướng dẫn đổi hàng", Content: "Kiểm tra mã đơn.", Enabled: true,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	const password = "nexa-test-password"
	code, err := operations.repo.ExportReplyTemplate(source, "reply", password)
	if err != nil || !strings.HasPrefix(code, templateSharePrefix) || strings.Contains(code, "Kiểm tra") {
		t.Fatalf("unexpected export: %v", err)
	}
	if _, err := operations.repo.ExportReplyTemplate(target, "reply", password); err == nil {
		t.Fatal("owner from another tenant could export source template")
	}
	if _, err := operations.repo.ImportReplyTemplate(target, code, "incorrect-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
	first, err := operations.repo.ImportReplyTemplate(target, code, password)
	if err != nil || first.TenantID != target.TenantID || first.Content != "Kiểm tra mã đơn." || first.ID == "reply" {
		t.Fatalf("unexpected import: %+v, %v", first, err)
	}
	repeat, err := operations.repo.ImportReplyTemplate(target, code, password)
	if err != nil || repeat.ID != first.ID || repeat.Version != first.Version {
		t.Fatalf("duplicate import changed record: %+v, %v", repeat, err)
	}
}
