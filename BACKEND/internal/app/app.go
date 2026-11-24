// Package app is the composition root: concrete implementations are wired only here.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"businessautomation/internal/api"
	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func NewHandler(repo core.Repository) http.Handler {
	return NewHandlerWithContext(context.Background(), repo)
}

func NewHandlerWithContext(ctx context.Context, repo core.Repository) http.Handler {
	storage := repo
	clock := service.SystemClock{}
	workspace := service.NewWorkspace(repo, clock, service.RandomIDs{})
	fanout, fanoutErr := service.NATSFanoutFromEnvironment()
	if fanoutErr != nil {
		slog.Error("KhA'ng nA­i vA­i hA­' NATS, dA­ng sA­i fanout nA­i bA­n", "error", fanoutErr)
	} else if fanout != nil {
		workspace.ConfigureFanout(fanout)
		slog.Info("SSE fanout A­i NATS (multi-replica)")
	}
	attachments, err := service.AttachmentStoreFromEnvironment()
	if err != nil {
		slog.Error("Không thể cấu hình kho tệp đính kèm", "error", err)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Kho tệp đính kèm cấu hình sai", http.StatusServiceUnavailable)
		})
	}
	workspace.ConfigureAttachments(attachments)
	repo = workspace
	admin := service.NewAdmin(workspace, clock)
	ids := service.RandomIDs{}
	auth := service.NewAuth(repo, clock, ids, service.PBKDF2Hasher{})
	mailer := service.EmailFromEnvironment()
	authOrigin := os.Getenv("APP_ORIGIN")
	if authOrigin == "" && os.Getenv("NEXA_DEV_EMAIL_SINK") == "true" {
		// Dev convenience: with the mail sink enabled but no public origin set,
		// build reset/invite links against the local dev server so the whole
		// password-recovery flow can be exercised without a mail provider.
		authOrigin = "http://localhost:3000"
	}
	auth.ConfigureEmail(mailer, authOrigin)
	resources := service.NewResources(repo, clock, ids, service.DefaultPolicies())
	conversations := service.NewConversations(repo, clock, ids, service.LocalKnowledgeProvider{}, service.NewKeywordAutomation(ids, clock))
	var model *service.OpenAIKnowledge
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	switch provider {
	case "", "openai":
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			model = service.NewOpenAIKnowledge(key, os.Getenv("OPENAI_CHAT_MODEL"))
		}
	default:
		key := os.Getenv("LLM_API_KEY")
		if provider == "deepseek" {
			key = os.Getenv("DEEPSEEK_API_KEY")
		}
		var err error
		model, err = service.NewCompatibleKnowledge(provider, os.Getenv("LLM_BASE_URL"), key, os.Getenv("LLM_CHAT_MODEL"), os.Getenv("LLM_EMBEDDING_MODEL"))
		if err != nil {
			slog.Error("Cấu hình nhà cung cấp AI không hợp lệ", "error", err)
		}
	}
	if model != nil {
		if db, ok := storage.(interface {
			core.KnowledgeVectors
			EnableKnowledgeVectors(context.Context) error
		}); ok && (provider == "" || provider == "openai") {
			setup, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := db.EnableKnowledgeVectors(setup)
			cancel()
			if err != nil {
				slog.Warn("Không bật được pgvector, dùng cosine trong bộ nhớ", "error", err)
			} else {
				model.ConfigureVectorIndex(db)
			}
		}
		model.ConfigureUsage(repo, clock, ids)
		conversations.ConfigureDraftProvider(model)
	}
	ai := service.NewAI(workspace, clock, model, os.Getenv("AI_SLA_AUTO_ESCALATE") == "true", os.Getenv("AI_CHATBOT_AUTO_REPLY") == "true")

	conversations.ConfigureEmail(mailer)
	platform := service.NewPlatform(repo, clock, ids)
	platform.ConfigureEmail(mailer, os.Getenv("APP_ORIGIN"))
	knowledgePublic := service.NewKnowledge(repo, ids)
	csat := service.NewCSAT(repo, clock, ids)
	csat.Configure(os.Getenv("APP_ORIGIN"))
	csat.ConfigureEmail(mailer)
	workspace.SetOnResolved(func(ticket core.Record) {
		if _, err := csat.IssueLink(ticket); err != nil {
			slog.Warn("Không tạo được khảo sát CSAT", "ticket", ticket.ID, "error", err)
		}
	})
	webhooks := service.NewWebhooks(repo, clock, ids)
	channels := service.NewChannels(repo)
	channels.ConfigureConversations(conversations)
	channels.ConfigureCampaignOrigin(os.Getenv("APP_ORIGIN"))
	ai.ConfigureChannels(channels)
	dashboard := service.NewDashboard(repo)
	operations := service.NewOperations(workspace, dashboard, mailer, clock)
	operations.ConfigureAIJobs(model)
	governance := service.NewGovernance(workspace, clock)
	actions := service.NewAIActions(workspace, clock, model)
	skills := service.NewAISkills(workspace, clock)
	voicebotCreds := service.NewVoicebotCredStore(os.Getenv("APP_ORIGIN"))
	voicebot := service.NewVoicebot(workspace, clock, model, voicebotCreds)
	if ctx.Done() != nil {
		go workspace.Run(ctx)
		go admin.Run(ctx)
		go ai.Run(ctx)
		go channels.Run(ctx)
		go webhooks.Run(ctx)
		go operations.Run(ctx)
		go conversations.RunEmailReplies(ctx)
		if mailer != nil {
			go csat.RunEmail(ctx)
		}
		if fanout != nil {
			go func() {
				<-ctx.Done()
				fanout.Close()
			}()
		}
	}
	return api.New(api.Dependencies{
		Workspace:          workspace,
		Admin:              admin,
		AI:                 ai,
		ResendInbound:      service.ResendInboundFromEnvironment(),
		InboundEmailTenant: os.Getenv("INBOUND_EMAIL_TENANT"),
		InboundEmailSecret: os.Getenv("INBOUND_EMAIL_SECRET"),
		Auth:               auth,
		Captcha:            service.TurnstileFromEnvironment(),
		Resources:          resources,
		Conversations:      conversations,
		Dashboard:          dashboard,
		Operations:         operations,
		Platform:           platform,
		KnowledgePublic:    knowledgePublic,
		CSAT:               csat,
		Webhooks:           webhooks,
		Channels:           channels,
		Governance:         governance,
		Actions:            actions,
		Skills:             skills,
		Voicebot:           voicebot,
	})
}
