// Package app is the composition root: concrete implementations are wired only here.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"businessautomation/internal/api"
	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func NewHandler(repo core.Repository) http.Handler {
	return NewHandlerWithContext(context.Background(), repo)
}

func NewHandlerWithContext(ctx context.Context, repo core.Repository) http.Handler {
	clock := service.SystemClock{}
	workspace := service.NewWorkspace(repo, clock, service.RandomIDs{})
	if ctx.Done() != nil {
		go workspace.Run(ctx)
	}
	repo = workspace
	admin := service.NewAdmin(workspace, clock)
	if ctx.Done() != nil {
		go admin.Run(ctx)
	}
	ids := service.RandomIDs{}
	auth := service.NewAuth(repo, clock, ids, service.PBKDF2Hasher{})
	mailer := service.EmailFromEnvironment()
	auth.ConfigureEmail(mailer, os.Getenv("APP_ORIGIN"))
	resources := service.NewResources(repo, clock, ids, service.DefaultPolicies())
	conversations := service.NewConversations(repo, clock, ids, service.LocalKnowledgeProvider{}, service.NewKeywordAutomation(ids, clock))
	var model *service.OpenAIKnowledge
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		model = service.NewOpenAIKnowledge(key, os.Getenv("OPENAI_CHAT_MODEL"))
		conversations.ConfigureDraftProvider(model)
	}
	ai := service.NewAI(workspace, clock, model, os.Getenv("AI_SLA_AUTO_ESCALATE") == "true", os.Getenv("AI_CHATBOT_AUTO_REPLY") == "true")
	if ctx.Done() != nil {
		go ai.Run(ctx)
	}
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
	dashboard := service.NewDashboard(repo)
	operations := service.NewOperations(workspace, dashboard, mailer, clock)
	if ctx.Done() != nil {
		go webhooks.Run(ctx)
		go operations.Run(ctx)
		if mailer != nil {
			go csat.RunEmail(ctx)
		}
	}
	return api.New(api.Dependencies{
		Workspace:          workspace,
		Admin:              admin,
		AI:                 ai,
		InboundEmailTenant: os.Getenv("INBOUND_EMAIL_TENANT"),
		InboundEmailSecret: os.Getenv("INBOUND_EMAIL_SECRET"),
		Auth:               auth,
		Resources:          resources,
		Conversations:      conversations,
		Dashboard:          dashboard,
		Operations:         operations,
		Platform:           platform,
		KnowledgePublic:    knowledgePublic,
		CSAT:               csat,
		Webhooks:           webhooks,
		Channels:           channels,
	})
}
