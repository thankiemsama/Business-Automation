package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

func TestChatbotDoesNotAutoSendAnUnfoundedPolicyAnswer(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "bot.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	workspace := NewWorkspace(repo, clock, RandomIDs{})
	model := NewOpenAIKnowledge("test-key", "test-model")
	calls := 0
	model.client.Transport = webhookTransport(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Path != "/v1/chat/completions" {
			t.Fatalf("private knowledge was indexed: %s", request.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"content\":\"Bạn có thể liên hệ hỗ trợ.\",\"mode\":\"general\",\"confidence\":0.99}"}}]}`))}, nil
	})
	user := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	err = workspace.Update(func(store core.Writer) error {
		store.PutUser(user)
		store.PutRecord(core.Record{ID: "private", TenantID: user.TenantID, Kind: core.KindKnowledge, Enabled: true, Content: "Nội bộ"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	conversations := NewConversations(workspace, clock, workspace.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(workspace.IDs, clock))
	secret, err := conversations.StartChat(user.TenantID, "Khách", "Chính sách hoàn tiền của công ty là gì?", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ai := NewAI(workspace, clock, model, false, true)
	ai.runChatbot()
	ai.runChatbot()
	messages, err := conversations.PublicChat(user.TenantID+"."+secret, "", "", "", 0)
	if err != nil || len(messages) != 1 || calls != 1 {
		t.Fatal(messages, calls, err)
	}
	if err = workspace.View(func(store core.Reader) error {
		if tickets := store.Records(user.TenantID, core.KindTickets); len(tickets) != 1 {
			t.Fatal(tickets)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAudioRequiresSupportedContainerAndUsesMultipart(t *testing.T) {
	model := NewOpenAIKnowledge("test-key", "test-model")
	model.client.Transport = webhookTransport(func(request *http.Request) (*http.Response, error) {
		if err := request.ParseMultipartForm(MaxAudioBytes); err != nil {
			t.Fatal(err)
		}
		file, header, err := request.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
		if header.Filename != "voice.wav" || request.FormValue("model") != "gpt-transcribe" {
			t.Fatal(header, request.MultipartForm.Value)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"text":"Xin chào."}`))}, nil
	})
	t.Setenv("OPENAI_TRANSCRIPTION_MODEL", "")
	ai := &AIService{model: model}
	if _, err := ai.Transcribe([]byte("invalid container")); err == nil {
		t.Fatal("unsupported container was accepted")
	}
	data := []byte("RIFF1234WAVEfmt 12345678")
	if text, err := ai.Transcribe(data); err != nil || text != "Xin chào." {
		t.Fatal(text, err)
	}
}

func TestSLAPredictionCanEscalateBeforeBreach(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "risk.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	workspace := NewWorkspace(repo, clock, RandomIDs{})
	ticket := core.Record{ID: "risk", TenantID: "tenant", Kind: core.KindTickets, Status: core.StatusOpen, Priority: core.PriorityUrgent, CreatedAt: clock.now.Add(-50 * time.Minute)}
	err = workspace.Update(func(store core.Writer) error {
		store.PutUser(core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner})
		store.PutRecord(ticket)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	NewAI(workspace, clock, nil, false, false).predict()
	verify := func(escalated bool) {
		t.Helper()
		if err := workspace.View(func(store core.Reader) error {
			current, _ := store.Record(ticket.TenantID, ticket.ID, ticket.Kind)
			if current.RiskReason == "" || (current.EscalatedAt != nil) != escalated || ticketOverdue(current, clock.now) {
				t.Fatal(current)
			}
			if escalated && current.AssigneeID != "owner" {
				t.Fatal(current)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	verify(false)
	NewAI(workspace, clock, nil, true, false).predict()
	verify(true)
}

func TestHybridKnowledgeSearchCachesArticleVectors(t *testing.T) {
	model := NewOpenAIKnowledge("test-key", "test-model")
	runs := 0
	model.client.Transport = webhookTransport(func(req *http.Request) (*http.Response, error) {
		var payload struct {
			Input []string
			Model string
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		runs++
		if payload.Model != "text-embedding-3-small" || (runs == 1 && len(payload.Input) != 3) || (runs == 2 && len(payload.Input) != 1) {
			t.Fatalf("unexpected input %v", payload)
		}
		data := []map[string]any{}
		for i, text := range payload.Input {
			vector := make([]float64, 256)
			column := 0
			if strings.Contains(text, "Shipping") {
				column = 1
			}
			vector[column] = 1
			data = append(data, map[string]any{"index": i, "embedding": vector})
		}
		body, _ := json.Marshal(map[string]any{"data": data})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	articles := []core.Record{
		{ID: "refund", TenantID: "tenant", Kind: core.KindKnowledge, Subject: "Hoàn tiền", Content: "Xử lý khoản hoàn", Enabled: true},
		{ID: "shipping", TenantID: "tenant", Kind: core.KindKnowledge, Subject: "Shipping", Enabled: true},
		{ID: "disabled", TenantID: "tenant", Kind: core.KindKnowledge, Subject: "Inactive"},
	}
	for i := 0; i < 2; i++ {
		results, err := model.Search("Lấy lại tiền", articles, 5)
		if err != nil || len(results) != 1 || results[0].ID != "refund" {
			t.Fatal(results, err)
		}
	}
}

func TestSummaryAndProposalDoNotOverwriteAgentDecisions(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	clock := &workspaceClock{now: now}
	workspace := NewWorkspace(repo, clock, RandomIDs{})
	model := NewOpenAIKnowledge("test-key", "test-model")
	model.client.Transport = webhookTransport(func(req *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(req.Body)
		if bytes.Contains(data, []byte("PRIVATE_NOTE")) {
			t.Fatal("private note reached provider")
		}
		output := `{"sentences":["Câu 1.","Câu 2.","Câu 3.","Câu 4.","Câu 5."]}`
		if bytes.Contains(data, []byte("priority")) {
			output = `{"tags":["VIP"],"priority":"high"}`
		}
		body, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]string{"content": output}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	ai := NewAI(workspace, clock, model, false, false)
	user := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	ticket := core.Record{ID: "ticket", TenantID: user.TenantID, Kind: core.KindTickets, ConversationID: "conversation", Subject: "Đơn hàng", Priority: "normal", Status: core.StatusOpen, CreatedAt: now}
	err = workspace.Update(func(store core.Writer) error {
		store.PutUser(user)
		store.PutRecord(core.Record{ID: "conversation", TenantID: user.TenantID, Kind: core.KindConversations})
		store.PutRecord(ticket)
		for i := 1; i <= 9; i++ {
			record := core.Record{ID: workspace.IDs.NewID(), TenantID: user.TenantID, Kind: core.KindMessages, ConversationID: "conversation", Sender: "customer", Content: "Khách hỏi đơn hàng", Sequence: uint64(i)}
			if i == 9 {
				record.Sender = "note"
				record.Content = "PRIVATE_NOTE"
			}
			store.PutRecord(record)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := ai.Summary(user, core.KindConversations, "conversation")
	if err != nil || len(strings.Split(summary, "\n")) != 5 {
		t.Fatal(summary, err)
	}
	ai.categorize(ticket)
	if err = workspace.View(func(store core.Reader) error {
		stored, _ := store.Record(user.TenantID, ticket.ID, ticket.Kind)
		if stored.Priority != "normal" || stored.Summary != summary {
			t.Fatal(stored)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	patch, err := ai.Suggest(user, ticket.ID, true)
	if err != nil || patch == nil || patch.Priority != "high" {
		t.Fatal(patch, err)
	}
	if _, err = ai.Suggest(user, ticket.ID, true); err == nil {
		t.Fatal("suggestion applied twice")
	}
}

func TestQuantilesLearnFromTenantHistory(t *testing.T) {
	var samples []slaSample
	for i := range 200 {
		f := slaFeatures{channel: "web", priority: "normal", hour: i % 24, workload: i % 3}
		duration := 20 * time.Minute
		if i%10 >= 7 {
			duration = 55 * time.Minute
		}
		samples = append(samples, slaSample{f, duration})
		f.channel, f.priority = "email", "low"
		samples = append(samples, slaSample{f, duration * 3})
	}
	median, tail := fitSLAQuantile(samples, .5), fitSLAQuantile(samples, .9)
	web := forecastSLA(median, tail, slaFeatures{"web", "normal", 12, 1})
	email := forecastSLA(median, tail, slaFeatures{"email", "low", 12, 1})
	if web.p50 >= web.p90 || email.p50 <= web.p50 || email.p90 <= web.p90 {
		t.Fatalf("unexpected forecasts for web %+v and email %+v", web, email)
	}
}

func TestConfidenceGateRequiresSupportedCalibration(t *testing.T) {
	calibration := confidenceCalibration{}
	calibration[8] = confidenceBin{Correct: 10, Total: 10}
	if _, bound := calibration.probability(.89); bound != 0 {
		t.Fatal("accepted insufficiently sampled bin")
	}
	calibration[9] = confidenceBin{Correct: 98, Total: 100}
	probability, bound := calibration.probability(1)
	if probability != .98 || bound < .85 || bound >= probability {
		t.Fatal(probability, bound)
	}
}

func TestCalibratedTriageAndBotResolution(t *testing.T) {
	base, err := repository.Open(filepath.Join(t.TempDir(), "calibrated.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	workspace := NewWorkspace(base, clock, RandomIDs{})
	tenant := "calibrated-workspace"
	owner := core.User{ID: "owner", TenantID: tenant, Role: core.RoleOwner}
	ticket := core.Record{ID: "triage", TenantID: tenant, Kind: core.KindTickets, Status: core.StatusOpen, Priority: core.PriorityNormal,
		Subject: "Sự cố phiếu phân loại", CreatedAt: clock.now, UpdatedAt: clock.now}
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(owner)
		store.PutRecord(ticket)
		store.PutRecord(core.Record{ID: "public", TenantID: tenant, Kind: core.KindKnowledge, Subject: "Cập nhật địa chỉ",
			Content: "Chọn Địa chỉ trong Hồ sơ.", Public: true, Enabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	model := NewOpenAIKnowledge("test", "test")
	model.client.Transport = webhookTransport(func(request *http.Request) (*http.Response, error) {
		requestBody, _ := io.ReadAll(request.Body)
		var body []byte
		if request.URL.Path == "/v1/embeddings" {
			vector := make([]float64, 256)
			vector[0] = 1
			body, _ = json.Marshal(map[string]any{"data": []map[string]any{
				{"index": 0, "embedding": vector}, {"index": 1, "embedding": vector},
			}})
		} else {
			output := `{"content":"Chọn Địa chỉ trong Hồ sơ.","source":"public","mode":"grounded","confidence":0.98,"sentiment":"neutral"}`
			if bytes.Contains(requestBody, []byte("priority")) {
				output = `{"tags":["Kiểm tra"],"priority":"high","sentiment":"negative","confidence":0.98}`
			}
			body, _ = json.Marshal(map[string]any{"choices": []map[string]any{
				{"finish_reason": "stop", "message": map[string]string{"content": output}},
			}})
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	ai := NewAI(workspace, clock, model, false, true)
	ai.triageCalibration[9] = confidenceBin{Correct: 98, Total: 100}
	ai.chatCalibration[9] = confidenceBin{Correct: 97, Total: 100}
	ai.categorize(ticket)
	conversations := NewConversations(workspace, clock, workspace.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(workspace.IDs, clock))
	secret, err := conversations.StartChat(tenant, "Khách", "Cập nhật địa chỉ như thế nào?", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ai.runChatbot()
	ai.runChatbot()
	token := tenant + "." + secret
	msgs, err := conversations.PublicChat(token, "", "", "", 0)
	if err != nil || len(msgs) != 2 || !strings.Contains(msgs[1].Content, "Đã giải quyết") {
		t.Fatal(msgs, err)
	}
	if _, err := conversations.PublicChat(token, "Đã giải quyết", "", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := conversations.PublicChat(token, "", "", "", 5); err != nil {
		t.Fatal(err)
	}
	err = workspace.View(func(store core.Reader) error {
		current, _ := store.Record(tenant, ticket.ID, ticket.Kind)
		if current.Priority != core.PriorityHigh || current.Sentiment != "negative" || len(current.Tags) != 1 || current.ResponseDueAt == nil {
			t.Fatal(current)
		}
		resolutions := store.Records(tenant, "ai_bot_resolutions")
		if len(resolutions) != 1 || len(store.Records(tenant, core.KindTickets)) != 1 {
			t.Fatal(resolutions)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	from := clock.now.Add(-time.Hour)
	analytics, err := NewDashboard(workspace).Analytics(owner, from, clock.now.Add(time.Hour), clock.now)
	if err != nil || analytics.AI.AutoResolved != 1 || analytics.AI.BotCases != 1 || len(analytics.ChannelCSAT) != 1 || *analytics.ChannelCSAT[0].Value != 5 {
		t.Fatal(analytics, err)
	}
}
