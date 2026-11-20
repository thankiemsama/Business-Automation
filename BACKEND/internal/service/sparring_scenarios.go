package service

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"businessautomation/internal/core"
)

const trainingScenariosKind = "training_scenarios"

type SparringLine struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type SparringScenario struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type SparringScenarioInput struct {
	ConversationID string         `json:"conversation_id"`
	Title          string         `json:"title"`
	ExpectedAction string         `json:"expected_action"`
	Lines          []SparringLine `json:"lines"`
	Anonymized     bool           `json:"anonymized"`
}

type sparringScenarioData struct {
	Lines  []SparringLine `json:"lines"`
	Action string         `json:"action"`
}

var sparringIdentifiers = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>]+|[\pL\pN._%+\-]+@[\pL\pN.\-]+\.[\pL]{2,}|\+?[0-9][0-9 .()\-]{6,}[0-9]|\b[0-9]{5,}\b|(?:\bid|mã đơn|mã khách|mã vận đơn|đơn hàng|tài khoản|địa chỉ)\s*[:#=]\s*[^\r\n;,]+`)

func sparringRedact(text string) string {
	return strings.TrimSpace(sparringIdentifiers.ReplaceAllString(text, "[ẩn thông tin]"))
}

func sparringRedactor(store core.Reader, conversation core.Record) func(string) string {
	customer, _ := store.Record(conversation.TenantID, conversation.CustomerID, core.KindCustomers)
	values := []string{customer.Name, customer.Email, customer.Phone, conversation.Email, conversation.Phone}
	for _, user := range store.Users(conversation.TenantID) {
		values = append(values, user.Name, user.Email)
	}
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	type replacement struct {
		pattern *regexp.Regexp
		label   string
	}
	var replacements []replacement
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if len([]rune(value)) < 2 {
			continue
		}
		replacements = append(replacements, replacement{
			pattern: regexp.MustCompile("(?i)" + regexp.QuoteMeta(value)),
			label:   "[ẩn-" + tokenHash(conversation.TenantID + "\x00" + conversation.ID + "\x00" + strings.ToLower(value))[:8] + "]",
		})
	}
	return func(text string) string {
		for _, item := range replacements {
			var cleaned strings.Builder
			start := 0
			for _, match := range item.pattern.FindAllStringIndex(text, -1) {
				before, _ := utf8.DecodeLastRuneInString(text[:match[0]])
				after, _ := utf8.DecodeRuneInString(text[match[1]:])
				if unicode.IsLetter(before) || unicode.IsNumber(before) || unicode.IsLetter(after) || unicode.IsNumber(after) {
					continue
				}
				cleaned.WriteString(text[start:match[0]])
				cleaned.WriteString(item.label)
				start = match[1]
			}
			if start != 0 {
				cleaned.WriteString(text[start:])
				text = cleaned.String()
			}
		}
		return sparringRedact(text)
	}
}

func (s *AIService) PreviewSparringScenario(user core.User, id string) ([]SparringLine, error) {
	if user.Role != core.RoleOwner {
		return nil, core.Fail(403, "Chỉ owner được nhập kịch bản")
	}
	lines := []SparringLine{}
	err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		if !aiOptIn(store, user.TenantID, "ai_sparring") {
			return core.Fail(403, "Doanh nghiệp chưa bật AI Sparring")
		}
		conversation, err := findOwnedRecord(store, user, id, core.KindConversations)
		if err != nil {
			return err
		}
		redact := sparringRedactor(store, conversation)
		messages := store.ConversationMessages(user.TenantID, id)
		sort.Slice(messages, func(i, j int) bool { return laterMessage(messages[j], messages[i]) })
		for _, msg := range messages {
			if len(lines) == 24 {
				break
			}
			if (msg.Sender == core.SenderCustomer || msg.Sender == core.SenderAgent) && msg.Status != "pending" && msg.Status != "failed" {
				if output := clipped(redact(msg.Content), 600); output != "" {
					lines = append(lines, SparringLine{Role: msg.Sender, Text: output})
				}
			}
		}
		return nil
	})
	return lines, err
}

func (s *AIService) SaveSparringScenario(user core.User, input SparringScenarioInput) (SparringScenario, error) {
	item := SparringScenario{}
	if user.Role != core.RoleOwner {
		return item, core.Fail(403, "Chỉ owner được lưu kịch bản")
	}
	if !input.Anonymized || len(input.Lines) < 2 || len(input.Lines) > 24 ||
		!contains([]string{"resolve_ticket", "handoff"}, input.ExpectedAction) {
		return item, core.Fail(400, "Chọn hành động chuẩn và duyệt ít nhất hai lượt thoại đã ẩn danh")
	}
	err := updateAsTenant(s.repo, user, func(store core.Writer) error {
		if !aiOptIn(store, user.TenantID, "ai_sparring") {
			return core.Fail(403, "Doanh nghiệp chưa bật AI Sparring")
		}
		conversation, err := findOwnedRecord(store, user, input.ConversationID, core.KindConversations)
		if err != nil {
			return err
		}
		redact := sparringRedactor(store, conversation)
		item.Title = redact(input.Title)
		if item.Title == "" || len([]rune(item.Title)) > 100 || input.Lines[0].Role != core.SenderCustomer {
			return core.Fail(400, "Thêm tiêu đề và bắt đầu kịch bản bằng lời khách hàng")
		}
		hasAgent := false
		for i := range input.Lines {
			line := &input.Lines[i]
			line.Text = redact(line.Text)
			if line.Text == "" || len([]rune(line.Text)) > 600 ||
				!contains([]string{core.SenderCustomer, core.SenderAgent}, line.Role) {
				return core.Fail(400, "Lượt thoại không hợp lệ")
			}
			hasAgent = hasAgent || line.Role == core.SenderAgent
		}
		if !hasAgent {
			return core.Fail(400, "Kịch bản cần ít nhất một phản hồi của nhân viên")
		}
		data, err := json.Marshal(sparringScenarioData{Lines: input.Lines, Action: input.ExpectedAction})
		if err != nil {
			return err
		}
		active := 0
		source := tokenHash("sparring-source:" + user.TenantID + ":" + conversation.ID)
		for _, record := range store.Records(user.TenantID, trainingScenariosKind) {
			if record.Enabled {
				if record.Subject == source && record.Name == item.Title && record.Content == string(data) {
					item.ID = record.ID
					return nil
				}
				active++
			}
		}
		if active >= 30 {
			return core.Fail(409, "Tối đa 30 kịch bản đang hoạt động")
		}
		now := s.clock.Now().UTC()
		item.ID = s.repo.IDs.NewID()
		store.PutRecord(core.Record{
			ID: item.ID, TenantID: user.TenantID, Kind: trainingScenariosKind, Name: item.Title, Content: string(data),
			Subject: source,
			Enabled: true, AuthorID: user.ID, CreatedAt: now, UpdatedAt: now,
		})
		return nil
	})
	return item, err
}

func (s *AIService) ArchiveSparringScenario(user core.User, id string) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner được lưu trữ kịch bản")
	}
	return updateAsTenant(s.repo, user, func(store core.Writer) error {
		record, err := findOwnedRecord(store, user, id, trainingScenariosKind)
		if err != nil {
			return err
		}
		record.Enabled, record.UpdatedAt = false, s.clock.Now().UTC()
		store.PutRecord(record)
		return nil
	})
}
