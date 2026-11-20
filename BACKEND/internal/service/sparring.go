package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const sparringSessionsKind = "sparring_conversations"
const sparringCertificatesKind = "sparring_certificates"
const evalSetsKind = "eval_sets"
const sparringPolicyVersion = "ticket-resolve-v1"
const sparringPassScore = 80
const sparringMaxTurns = 8

type SparringAssessment struct {
	Baseline   *int   `json:"baseline,omitempty"`
	Regression bool   `json:"regression,omitempty"`
	Score      int    `json:"score"`
	Coaching   string `json:"coaching"`
	Action     string `json:"action"`
	Expected   string `json:"expected"`
	Passed     bool   `json:"passed"`
}

type SparringSession struct {
	ID          string               `json:"id"`
	Mode        string               `json:"mode"`
	Title       string               `json:"title"`
	Status      string               `json:"status"`
	Lines       []SparringLine       `json:"lines,omitempty"`
	Assessments []SparringAssessment `json:"assessments,omitempty"`
	ScenarioIDs []string             `json:"-"`
	Action      string               `json:"-"`
}

type SparringOverview struct {
	EvalSources []AgentEvalSource  `json:"eval_sources,omitempty"`
	Enabled     bool               `json:"enabled"`
	Scenarios   []SparringScenario `json:"scenarios"`
	Sessions    []SparringSession  `json:"sessions"`
	CertifiedTo *time.Time         `json:"certified_until,omitempty"`
}

type storedSparringSession struct {
	SparringSession
	ScenarioIDs []string `json:"scenario_ids"`
	Action      string   `json:"action,omitempty"`
}

func readSparringSession(record core.Record) (SparringSession, error) {
	var data storedSparringSession
	err := json.Unmarshal([]byte(record.Content), &data)
	result := data.SparringSession
	result.ScenarioIDs, result.Action = data.ScenarioIDs, data.Action
	result.Status, result.ID = record.Status, record.ID
	return result, err
}

func saveSparringSession(store core.Writer, record core.Record, session SparringSession, now time.Time) error {
	payload, err := json.Marshal(storedSparringSession{session, session.ScenarioIDs, session.Action})
	if err != nil {
		return err
	}
	record.Status, record.Content, record.UpdatedAt = session.Status, string(payload), now
	store.PutRecord(record)
	return nil
}

func sparringCertificateID(model string) string {
	return tokenHash("sparring-cert:" + model + "\x00" + sparringPolicyVersion + "\x00" + agentPlannerPolicyID())
}

func sparringCertificate(store core.Reader, tenant, model string, now time.Time) *time.Time {
	if model == "" {
		return nil
	}
	cert, ok := store.Record(tenant, sparringCertificateID(model), sparringCertificatesKind)
	if !ok || !cert.Enabled || cert.EmailRetryAt == nil || !now.Before(*cert.EmailRetryAt) || len(cert.Tags) != 2 {
		return nil
	}
	for _, id := range cert.Tags {
		if scenario, found := store.Record(tenant, id, trainingScenariosKind); !found || !scenario.Enabled {
			return nil
		}
	}
	return cert.EmailRetryAt
}

func sparringPermitsResolution(store core.Reader, tenant, model string, now time.Time) bool {
	return sparringCertificate(store, tenant, model, now) != nil
}

func (s *AIService) SparringOverview(user core.User) (SparringOverview, error) {
	result := SparringOverview{Scenarios: []SparringScenario{}, Sessions: []SparringSession{}}
	err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		result.Enabled = s.Enabled() && aiOptIn(store, user.TenantID, "ai_sparring")
		if !result.Enabled {
			return nil
		}
		result.CertifiedTo = sparringCertificate(store, user.TenantID, s.model.model, s.clock.Now())
		if user.Role == core.RoleOwner {
			var err error
			result.EvalSources, err = agentEvalSources(store, user.TenantID, s.clock.Now())
			if err != nil {
				return err
			}
		}
		for _, record := range store.Records(user.TenantID, trainingScenariosKind) {
			if record.Enabled {
				result.Scenarios = append(result.Scenarios, SparringScenario{record.ID, record.Name})
			}
		}
		sort.Slice(result.Scenarios, func(i, j int) bool { return result.Scenarios[i].Title < result.Scenarios[j].Title })
		records := store.Records(user.TenantID, sparringSessionsKind)
		sort.Slice(records, func(i, j int) bool {
			a, b := records[i], records[j]
			pendingA := a.Status == "active" || a.Status == "queued"
			pendingB := b.Status == "active" || b.Status == "queued"
			if pendingA != pendingB {
				return pendingA
			}
			if a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.ID < b.ID
			}
			return a.UpdatedAt.After(b.UpdatedAt)
		})
		for _, record := range records {
			if record.AuthorID != user.ID && !(user.Role == core.RoleOwner && record.Name == "certification") {
				continue
			}
			session, err := readSparringSession(record)
			if err != nil {
				return err
			}
			result.Sessions = append(result.Sessions, session)
			if len(result.Sessions) == 15 {
				break
			}
		}
		return nil
	})
	return result, err
}

func enqueueSparringAssessment(store core.Writer, record core.Record, now time.Time) {
	store.PutRecord(core.Record{
		ID: tokenHash("sparring-job:" + record.ID), TenantID: record.TenantID, Kind: workflowJobsKind,
		Subject: core.WorkflowSparringAssessment, ConversationID: record.ID, Enabled: true,
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	})
}

func (s *AIService) StartSparring(user core.User, scenarioID, requestID string, certification bool) (SparringSession, error) {
	return s.startSparring(user, scenarioID, requestID, certification, nil)
}

func (s *AIService) startSparring(user core.User, scenarioID, requestID string, certification bool, pinnedCases []string) (SparringSession, error) {
	session := SparringSession{}
	if !s.Enabled() {
		return session, core.Fail(503, "Chưa cấu hình OpenAI")
	}
	if certification && user.Role != core.RoleOwner {
		return session, core.Fail(403, "Chỉ owner được kiểm định AI agent")
	}
	if len(requestID) < 16 || len(requestID) > 80 {
		return session, core.Fail(400, "Mã yêu cầu luyện tập không hợp lệ")
	}
	err := updateAsTenant(s.repo, user, func(store core.Writer) error {
		if !aiOptIn(store, user.TenantID, "ai_sparring") {
			return core.Fail(403, "Doanh nghiệp chưa bật AI Sparring")
		}
		scope := user.ID
		if len(pinnedCases) > 0 {
			scope = "system-regression"
		}
		id := tokenHash("sparring-session:" + scope + ":" + requestID)
		if existing, ok := store.Record(user.TenantID, id, sparringSessionsKind); ok {
			var err error
			session, err = readSparringSession(existing)
			return err
		}
		now := s.clock.Now().UTC()
		record := core.Record{ID: id, TenantID: user.TenantID, Kind: sparringSessionsKind, AuthorID: user.ID, CreatedAt: now}
		session = SparringSession{ID: id, Mode: "sparring", Status: "active"}
		if certification {
			var cases = map[string]core.Record{}
			if len(pinnedCases) > 0 {
				for _, caseID := range pinnedCases {
					item, data, err := loadSparringScenario(store, user.TenantID, caseID)
					if err != nil {
						return err
					}
					cases[data.Action] = item
				}
			} else {
				for _, candidate := range store.Records(user.TenantID, trainingScenariosKind) {
					var data sparringScenarioData
					if candidate.Enabled && json.Unmarshal([]byte(candidate.Content), &data) == nil && laterMessage(candidate, cases[data.Action]) {
						cases[data.Action] = candidate
					}
				}
			}
			if cases["resolve_ticket"].ID == "" || cases["handoff"].ID == "" {
				return core.Fail(409, "Cần một kịch bản giải quyết và một kịch bản chuyển người xử lý")
			}
			session.Mode, session.Title, session.Status = "certification", "Kiểm định AI agent", "queued"
			session.ScenarioIDs = []string{cases["resolve_ticket"].ID, cases["handoff"].ID}
			record.Subject = s.model.model
			record.Slug = sparringCertificateID(s.model.model)
		} else {
			item, found := store.Record(user.TenantID, scenarioID, trainingScenariosKind)
			var data sparringScenarioData
			if !found || !item.Enabled || json.Unmarshal([]byte(item.Content), &data) != nil || len(data.Lines) == 0 {
				return core.Fail(404, "Không tìm thấy kịch bản")
			}
			session.ScenarioIDs, session.Title, session.Lines = []string{item.ID}, item.Name, []SparringLine{data.Lines[0]}
		}
		for _, existing := range store.Records(user.TenantID, sparringSessionsKind) {
			if certification && existing.Name == "certification" && existing.Status == "queued" ||
				!certification && existing.AuthorID == user.ID && existing.Name == "sparring" && existing.Status == "active" {
				return core.Fail(409, "Đã có phiên đang hoạt động")
			}
		}
		record.Name = session.Mode
		if session.Status == "queued" {
			store.PutRecord(core.Record{
				ID: tokenHash("agent-eval-set:" + record.ID), TenantID: user.TenantID, Kind: evalSetsKind,
				Name: record.Subject, Subject: record.Slug, ConversationID: record.ID, Tags: session.ScenarioIDs,
				AuthorID: user.ID, Enabled: true, CreatedAt: now, UpdatedAt: now,
			})
			enqueueSparringAssessment(store, record, now)
		}
		return saveSparringSession(store, record, session, now)
	})
	return session, err
}

func (s *AIService) SparringTurn(user core.User, id, text, requestID string, turn int) (SparringSession, error) {
	result := SparringSession{}
	if !s.Enabled() {
		return result, core.Fail(503, "Chưa cấu hình OpenAI")
	}
	text = sparringRedact(text)
	if text == "" || len([]rune(text)) > 1000 || len(requestID) < 16 || len(requestID) > 80 {
		return result, core.Fail(400, "Phản hồi luyện tập không hợp lệ")
	}
	var before core.Record
	var scenario sparringScenarioData
	err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		var err error
		before, result, err = loadSparringPractice(store, user, id)
		if err != nil || before.MessageID == requestID {
			return err
		}
		if result.Status != "active" || len(result.Lines) != turn || turn >= 2*sparringMaxTurns+1 {
			return core.Fail(409, "Lượt luyện tập đã thay đổi hoặc đã đạt số lượt tối đa")
		}
		_, scenario, err = loadSparringScenario(store, user.TenantID, result.ScenarioIDs[0])
		return err
	})
	if err != nil || before.MessageID == requestID {
		return result, err
	}
	result.Lines = append(result.Lines, SparringLine{core.SenderAgent, text})
	customerLines := []string{}
	for _, line := range scenario.Lines {
		if line.Role == core.SenderCustomer {
			customerLines = append(customerLines, line.Text)
		}
	}
	var reply struct{ Text string }
	if err := s.model.complete(user.TenantID, `Đóng vai khách hàng khó tính trong buổi luyện tập. Dựa vào tình huống và diễn biến để nêu một phản hồi tự nhiên bằng tiếng Việt, thử sự kiên nhẫn hoặc yêu cầu giải thích nếu phù hợp. Tránh lặp lại lời khách đã xuất hiện. Nếu vấn đề đã xử lý thỏa đáng, hãy xác nhận. Không nhắc dữ liệu định danh, không kể đáp án và không đóng vai nhân viên. Trả JSON text tối đa 600 ký tự.`, map[string]any{"customer_examples": customerLines, "practice": result.Lines}, 400, &reply); err != nil {
		return SparringSession{}, err
	}
	reply.Text = sparringRedact(reply.Text)
	if reply.Text == "" || len([]rune(reply.Text)) > 600 {
		return SparringSession{}, core.Fail(502, "Phản hồi khách mô phỏng không hợp lệ")
	}
	result.Lines = append(result.Lines, SparringLine{core.SenderCustomer, reply.Text})
	err = updateAsTenant(s.repo, user, func(store core.Writer) error {
		current, session, err := loadSparringPractice(store, user, id)
		if err != nil {
			return err
		}
		if current.MessageID == requestID {
			result = session
			return nil
		}
		if current.Content != before.Content {
			return core.Fail(409, "Lượt luyện tập đã thay đổi")
		}
		if _, _, err := loadSparringScenario(store, user.TenantID, session.ScenarioIDs[0]); err != nil {
			return err
		}
		current.MessageID = requestID
		return saveSparringSession(store, current, result, s.clock.Now().UTC())
	})
	return result, err
}

func loadSparringPractice(store core.Reader, user core.User, id string) (core.Record, SparringSession, error) {
	if !aiOptIn(store, user.TenantID, "ai_sparring") {
		return core.Record{}, SparringSession{}, core.Fail(403, "Doanh nghiệp chưa bật AI Sparring")
	}
	record, ok := store.Record(user.TenantID, id, sparringSessionsKind)
	if !ok || record.AuthorID != user.ID || record.Name != "sparring" {
		return record, SparringSession{}, core.Fail(404, "Không tìm thấy phiên luyện tập")
	}
	session, err := readSparringSession(record)
	return record, session, err
}

func loadSparringScenario(store core.Reader, tenant, id string) (core.Record, sparringScenarioData, error) {
	record, found := store.Record(tenant, id, trainingScenariosKind)
	var scenario sparringScenarioData
	if !found || !record.Enabled || json.Unmarshal([]byte(record.Content), &scenario) != nil || len(scenario.Lines) == 0 {
		return record, scenario, core.Fail(409, "Kịch bản không còn khả dụng")
	}
	return record, scenario, nil
}

func (s *AIService) EndSparring(user core.User, id, action string) error {
	if !contains([]string{"resolve_ticket", "handoff", "abandon"}, action) {
		return core.Fail(400, "Chọn cách kết thúc phiên hợp lệ")
	}
	return updateAsTenant(s.repo, user, func(store core.Writer) error {
		record, session, err := loadSparringPractice(store, user, id)
		if err != nil || session.Status != "active" {
			return err
		}
		now := s.clock.Now().UTC()
		session.Status = "abandoned"
		if action != "abandon" {
			if len(session.Lines) < 3 {
				return core.Fail(409, "Cần ít nhất một lượt luyện trước khi chấm điểm")
			}
			session.Status, session.Action = "queued", strings.TrimSpace(action)
			enqueueSparringAssessment(store, record, now)
		}
		return saveSparringSession(store, record, session, now)
	})
}
