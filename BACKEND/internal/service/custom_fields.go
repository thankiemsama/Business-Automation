package service

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"businessautomation/internal/core"
)

type CustomField struct {
	ID      string `json:"id,omitempty"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

func validAdminNumber(value string) bool {
	n, err := strconv.ParseFloat(value, 64)
	return err == nil && !math.IsNaN(n) && !math.IsInf(n, 0)
}

func validCustomValue(field CustomField, input string) bool {
	return len([]rune(input)) <= 1000 &&
		(field.Type != "checkbox" || input == "true" || input == "false") &&
		(field.Type != "number" || validAdminNumber(input))
}
func customKind(kind string) bool {
	return kind == core.KindCustomers || kind == core.KindTickets || kind == core.KindConversations
}
func (a *Admin) Fields(tenant string) ([]CustomField, error) {
	items := []CustomField{}
	err := a.repo.View(func(store core.Reader) error {
		items = workflowConfig[CustomField](store, tenant, "custom_fields")
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, err
}
func (a *Admin) SaveField(user core.User, field CustomField) (CustomField, error) {
	field.Name = strings.TrimSpace(field.Name)
	if !customKind(field.Kind) || field.Name == "" || len([]rune(field.Name)) > 80 || !contains([]string{"text", "number", "checkbox"}, field.Type) {
		return field, core.Fail(400, "Định nghĩa trường không hợp lệ")
	}
	err := a.repo.updateAs(user.ID, func(store core.Writer) error {
		if field.ID == "" {
			if len(store.Records(user.TenantID, "custom_fields")) >= 40 {
				return core.Fail(409, "Tối đa 40 trường tùy chỉnh")
			}
			field.ID = a.repo.IDs.NewID()
		} else {
			existing, ok := store.Record(user.TenantID, field.ID, "custom_fields")
			var old CustomField
			if !ok || json.Unmarshal([]byte(existing.Content), &old) != nil {
				return core.Fail(404, "Không tìm thấy trường")
			}
			if old.Kind != field.Kind || old.Type != field.Type {
				return core.Fail(409, "Không thể đổi tài nguyên hoặc kiểu của trường đã lưu")
			}
		}
		payload, err := json.Marshal(field)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{ID: field.ID, TenantID: user.TenantID, Kind: "custom_fields", Name: field.Name, Content: string(payload), Enabled: field.Enabled, UpdatedAt: a.clock.Now()})
		return nil
	})
	return field, err
}
func (a *Admin) Custom(user core.User, kind, id string, patch map[string]string) (map[string]string, error) {
	if !customKind(kind) {
		return nil, core.Fail(400, "Tài nguyên không hỗ trợ")
	}
	current := map[string]string{}
	load := func(store core.Reader) (core.Record, error) {
		record, err := findOwnedRecord(store, user, id, kind)
		if err != nil {
			return record, err
		}
		if record.CustomValues != "" {
			err = json.Unmarshal([]byte(record.CustomValues), &current)
		}
		return record, err
	}
	if patch == nil {
		err := a.repo.View(func(store core.Reader) error { _, err := load(store); return err })
		return current, err
	}
	err := a.repo.updateAs(user.ID, func(store core.Writer) error {
		record, err := load(store)
		if err != nil {
			return err
		}
		if len(patch) > 40 {
			return core.Fail(400, "Quá nhiều trường")
		}
		for id, input := range patch {
			config, ok := store.Record(user.TenantID, id, "custom_fields")
			var field CustomField
			if !ok || !config.Enabled || json.Unmarshal([]byte(config.Content), &field) != nil || field.Kind != kind {
				return core.Fail(400, "Trường tùy chỉnh không hợp lệ")
			}
			input = strings.TrimSpace(input)
			if input == "" {
				delete(current, id)
				continue
			}
			if !validCustomValue(field, input) {
				return core.Fail(400, "Giá trị trường không hợp lệ")
			}
			current[id] = input
		}
		data, err := json.Marshal(current)
		if err != nil {
			return err
		}
		record.CustomValues = string(data)
		store.PutRecord(record)
		return nil
	})
	return current, err
}
func OpenTenantBackup(tenant, encrypted string) ([]byte, error) {
	value, err := readSecret(strings.TrimSpace(encrypted), "backup:"+tenant)
	return []byte(value), err
}

func (a *Admin) Backup(user core.User, value map[string]any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return protectSecret(string(payload), "backup:"+user.TenantID)
}
