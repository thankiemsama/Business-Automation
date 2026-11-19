package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"businessautomation/internal/core"
)

type AIService struct {
	repo                               *Workspace
	clock                              Clock
	model                              *OpenAIKnowledge
	channels                           *ChannelManager
	autoEscalate                       bool
	autoChat                           bool
	chatCalibration, triageCalibration confidenceCalibration
	chatThreshold, triageThreshold     float64
	summarySlots                       chan struct{}
	mu                                 sync.Mutex
	summaries                          map[string]bool
	retry                              map[string]time.Time
}

func NewAI(repo *Workspace, clock Clock, model *OpenAIKnowledge, autoEscalate bool, autoChat bool) *AIService {
	return &AIService{
		repo: repo, clock: clock, model: model, autoEscalate: autoEscalate, autoChat: autoChat,
		chatCalibration: calibrationFromEnvironment("AI_CHATBOT_CALIBRATION"), chatThreshold: confidenceThreshold("AI_CHATBOT_CONFIDENCE_THRESHOLD", .7),
		triageCalibration: calibrationFromEnvironment("AI_TRIAGE_CALIBRATION"), triageThreshold: confidenceThreshold("AI_TRIAGE_CONFIDENCE_THRESHOLD", .85),
		summarySlots: make(chan struct{}, 3), summaries: map[string]bool{}, retry: map[string]time.Time{},
	}
}

func (s *AIService) ConfigureChannels(channels *ChannelManager) { s.channels = channels }

func (s *AIService) Enabled() bool { return s.model != nil }

func (s *AIService) Search(user core.User, q string) ([]KnowledgeMatch, error) {
	if !s.Enabled() {
		return nil, core.Fail(503, "Chưa cấu hình OpenAI")
	}
	var articles []core.Record
	if err := s.repo.View(func(store core.Reader) error { articles = store.Records(user.TenantID, core.KindKnowledge); return nil }); err != nil {
		return nil, err
	}
	return s.model.Search(q, articles, 5)
}

func transcript(store core.Reader, tenant, id string) []core.Record {
	rows := []core.Record{}
	if id == "" {
		return rows
	}
	for _, message := range store.Records(tenant, core.KindMessages) {
		if message.ConversationID == id && message.Sender != "note" && strings.TrimSpace(message.Content) != "" {
			rows = append(rows, message)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return laterMessage(rows[j], rows[i]) })
	return rows
}

func (s *AIService) Summary(user core.User, kind, id string) (string, error) {
	if kind != core.KindConversations && kind != core.KindTickets {
		return "", core.Fail(400, "Loại tài nguyên không hợp lệ")
	}
	var entity core.Record
	var thread []core.Record
	err := s.repo.View(func(store core.Reader) error {
		var err error
		entity, err = findOwnedRecord(store, user, id, kind)
		if err != nil {
			return err
		}
		target := entity.ID
		if kind == core.KindTickets {
			target = entity.ConversationID
		}
		thread = transcript(store, user.TenantID, target)
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(thread) < 8 {
		return entity.Summary, nil
	}
	source := thread[len(thread)-1].ID
	if source == entity.SummarySource {
		return entity.Summary, nil
	}
	if !s.Enabled() {
		return entity.Summary, nil
	}
	key := user.TenantID + ":" + kind + ":" + id
	s.mu.Lock()
	if s.summaries[key] {
		s.mu.Unlock()
		return entity.Summary, nil
	}
	s.summaries[key] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.summaries, key); s.mu.Unlock() }()
	select {
	case s.summarySlots <- struct{}{}:
		defer func() { <-s.summarySlots }()
	default:
		return "", core.Fail(429, "Hệ thống đang tóm tắt hội thoại khác")
	}
	type line struct{ Role, Text string }
	if len(thread) > 50 {
		thread = append(append([]core.Record{}, thread[:10]...), thread[len(thread)-40:]...)
	}
	lines := []line{}
	for _, msg := range thread {
		lines = append(lines, line{msg.Sender, clipped(msg.Content, 500)})
	}
	var output struct{ Sentences []string }
	err = s.model.complete(user.TenantID, "Tóm tắt tiếng Việt từ 5 đến 10 câu. JSON gồm sentences là mảng câu. Nêu vấn đề, việc đã làm và việc còn chờ; không bịa dữ kiện hoặc chép thông tin định danh không cần thiết.", lines, 900, &output)
	if err != nil {
		return "", err
	}
	if len(output.Sentences) < 5 || len(output.Sentences) > 10 {
		return "", core.Fail(502, "Bản tóm tắt AI không đúng định dạng")
	}
	sentences := []string{}
	for _, sentence := range output.Sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" || len([]rune(sentence)) > 450 {
			return "", core.Fail(502, "Bản tóm tắt AI không hợp lệ")
		}
		sentences = append(sentences, sentence)
	}
	summary := strings.Join(sentences, "\n")
	err = s.repo.Update(func(store core.Writer) error {
		target := entity.ID
		if kind == core.KindTickets {
			target = entity.ConversationID
		}
		current := transcript(store, user.TenantID, target)
		if len(current) == 0 || current[len(current)-1].ID != source {
			return core.Fail(409, "Hội thoại đã thay đổi")
		}
		fresh, err := findOwnedRecord(store, user, id, kind)
		if err != nil {
			return err
		}
		if fresh.ConversationID != entity.ConversationID {
			return core.Fail(409, "Hội thoại đã thay đổi")
		}
		fresh.Summary, fresh.SummarySource = summary, source
		store.PutRecord(fresh)
		if kind == core.KindConversations {
			for _, ticket := range store.Records(user.TenantID, core.KindTickets) {
				if ticket.ConversationID == id {
					ticket.Summary, ticket.SummarySource = summary, source
					store.PutRecord(ticket)
				}
			}
		}
		return nil
	})
	return summary, err
}

type Suggestion struct {
	Tags       []string `json:"tags"`
	Priority   string   `json:"priority"`
	Confidence float64  `json:"confidence"`
	Sentiment  string   `json:"sentiment,omitempty"`
}

func (s *AIService) Suggest(user core.User, id string, apply bool) (*Suggestion, error) {
	var result *Suggestion
	read := func(store core.Reader) (core.Record, core.Record, error) {
		ticket, err := findOwnedRecord(store, user, id, core.KindTickets)
		if err != nil {
			return ticket, core.Record{}, err
		}
		suggestion, _ := store.Record(user.TenantID, tokenHash("ai-suggestion:"+id), "ai_suggestions")
		if suggestion.Enabled && suggestion.Subject == suggestionKey(ticket) && ticket.Status != core.StatusResolved {
			var decoded Suggestion
			if err := json.Unmarshal([]byte(suggestion.Content), &decoded); err != nil {
				return ticket, suggestion, err
			}
			result = &decoded
		}
		return ticket, suggestion, nil
	}
	if !apply {
		err := s.repo.View(func(store core.Reader) error { _, _, err := read(store); return err })
		return result, err
	}
	err := s.repo.Update(func(store core.Writer) error {
		ticket, record, err := read(store)
		if err != nil {
			return err
		}
		if result == nil || ticket.Status == core.StatusResolved || suggestionKey(ticket) != record.Subject {
			return core.Fail(409, "Gợi ý AI đã hết hiệu lực")
		}
		if err := s.applyTriage(store, ticket, *result); err != nil {
			return err
		}
		record.Enabled = false
		store.PutRecord(record)
		recordTriageEvent(store, user.TenantID, id, "accepted", s.clock.Now().UTC())
		return nil
	})
	return result, err
}

func (s *AIService) applyTriage(store core.Writer, ticket core.Record, result Suggestion) error {
	tags, err := normalizeTags(append(append([]string{}, ticket.Tags...), result.Tags...))
	if err != nil {
		return err
	}
	if ticket.Priority != result.Priority {
		ticket.Priority = result.Priority
		ticket.ResponseDueAt, ticket.ResolutionDueAt = nil, nil
		response, resolution := ticketDeadlines(ticket)
		ticket.ResponseDueAt, ticket.ResolutionDueAt = &response, &resolution
	}
	if validSentiment(result.Sentiment) {
		ticket.Sentiment = result.Sentiment
	}
	ticket.Tags, ticket.UpdatedAt = tags, s.clock.Now().UTC()
	store.PutRecord(ticket)
	return nil
}

func suggestionKey(ticket core.Record) string {
	return tokenHash(ticket.Subject + "\x00" + ticket.Content + "\x00" + ticket.Priority + "\x00" + strings.Join(ticket.Tags, "\x00"))
}

func (s *AIService) Run(ctx context.Context) {
	if s.Enabled() {
		go runAIWork(ctx, s.analyzeOne)
		go runAIWork(ctx, s.runChatbot)
	}
	runAIWork(ctx, s.predict)
}

func runAIWork(ctx context.Context, job func()) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		job()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *AIService) analyzeOne() {
	var job core.Record
	_ = s.repo.View(func(store core.Reader) error {
		now := s.clock.Now()
		earliest := now.Add(-24 * time.Hour)
		for key, until := range s.retry {
			if !until.After(now) {
				delete(s.retry, key)
			}
		}
		for _, tenant := range store.Tenants() {
			if !featureEnabled(store, tenant, "ai") {
				continue
			}
			for _, message := range store.Records(tenant, core.KindMessages) {
				if message.Sender == core.SenderCustomer && message.Sentiment == "" && !s.retry[message.ID].After(now) && strings.TrimSpace(message.Content) != "" && message.CreatedAt.After(earliest) && (job.ID == "" || message.CreatedAt.Before(job.CreatedAt)) {
					job = message
				}
			}
			for _, ticket := range store.Records(tenant, core.KindTickets) {
				if s.retry[ticket.ID].After(now) || ticket.Status == core.StatusResolved || !ticket.CreatedAt.After(earliest) {
					continue
				}
				existing, exists := store.Record(tenant, tokenHash("ai-suggestion:"+ticket.ID), "ai_suggestions")
				if (!exists || existing.Enabled && existing.Subject != suggestionKey(ticket)) && (job.ID == "" || ticket.CreatedAt.Before(job.CreatedAt)) {
					job = ticket
				}
			}
		}
		return nil
	})
	if job.ID != "" {
		s.retry[job.ID] = s.clock.Now().Add(5 * time.Minute)
	}
	if job.Kind == core.KindMessages {
		s.classify(job)
	}
	if job.Kind == core.KindTickets {
		s.categorize(job)
	}
}

func (s *AIService) classify(job core.Record) {
	var result struct{ Sentiment string }
	if err := s.model.complete(job.TenantID, "Phân loại cảm xúc của khách. JSON sentiment chỉ là neutral, negative hoặc angry. Chỉ dùng angry nếu có dấu hiệu tức giận rõ.", clipped(job.Content, 2000), 100, &result); err != nil {
		return
	}
	if result.Sentiment != "neutral" && result.Sentiment != "negative" && result.Sentiment != "angry" {
		return
	}
	_ = s.repo.Update(func(store core.Writer) error {
		fresh, ok := store.Record(job.TenantID, job.ID, core.KindMessages)
		if !ok || fresh.Content != job.Content || fresh.Sentiment != "" {
			return nil
		}
		fresh.Sentiment = result.Sentiment
		store.PutRecord(fresh)
		if result.Sentiment == "angry" {
			conversation, ok := store.Record(job.TenantID, job.ConversationID, core.KindConversations)
			if ok && conversation.Status != core.StatusClosed {
				store.(*workspaceWriter).notify(conversation, "sentiment", "Khách có dấu hiệu tức giận")
			}
		}
		return nil
	})
}

func (s *AIService) categorize(job core.Record) {
	var result Suggestion
	if err := s.model.complete(job.TenantID, "Đề xuất phân loại phiếu hỗ trợ. JSON gồm tags (tối đa 4 nhãn tiếng Việt) priority (low, normal, high hoặc urgent), sentiment (neutral, negative hoặc angry) và confidence (0-1 cho toàn bộ kết quả). Chỉ chọn urgent nếu có gián đoạn nghiêm trọng hoặc nguy cơ tức thời thể hiện rõ.", map[string]string{"subject": clipped(job.Subject, 400), "description": clipped(job.Content, 3000), "priority": job.Priority}, 200, &result); err != nil {
		return
	}
	tags, err := normalizeTags(result.Tags)
	if err != nil || len(tags) > 4 || (result.Priority != "low" && result.Priority != "normal" && result.Priority != "high" && result.Priority != "urgent") {
		return
	}
	result.Tags = tags
	probability, lowerBound := s.triageCalibration.probability(result.Confidence)
	result.Confidence = probability
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	_ = s.repo.Update(func(store core.Writer) error {
		fresh, ok := store.Record(job.TenantID, job.ID, core.KindTickets)
		if !ok || fresh.Status == core.StatusResolved || suggestionKey(job) != suggestionKey(fresh) {
			return nil
		}
		key := tokenHash("ai-suggestion:" + job.ID)
		if existing, exists := store.Record(job.TenantID, key, "ai_suggestions"); exists && (!existing.Enabled || existing.Subject == suggestionKey(fresh)) {
			return nil
		}
		review := true
		if s.triageThreshold >= .7 && lowerBound >= s.triageThreshold && validSentiment(result.Sentiment) {
			review = s.applyTriage(store, fresh, result) != nil
		}
		store.PutRecord(core.Record{ID: key, TenantID: job.TenantID, Kind: "ai_suggestions", Subject: suggestionKey(fresh), Content: string(data), Enabled: review})
		outcome := "offered"
		if !review {
			outcome = "auto_applied"
		}
		recordTriageEvent(store, job.TenantID, job.ID, outcome, s.clock.Now().UTC())
		return nil
	})
}

const aiTriageEventsKind = "ai_triage_events"

func recordTriageEvent(store core.Writer, tenant, ticket, outcome string, now time.Time) {
	store.PutRecord(core.Record{
		ID: (RandomIDs{}).NewID(), TenantID: tenant, Kind: aiTriageEventsKind,
		ConversationID: ticket, Status: outcome, CreatedAt: now, UpdatedAt: now,
	})
}
