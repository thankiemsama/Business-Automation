package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

type AuthUseCases interface {
	ChangePassword(core.User, string, string) error
	SendReset(string) error
	Invite(core.User, string, string) error
	Redeem(string, string) error
	Register(service.Credentials) (core.User, error)
	Login(service.Credentials) (string, error)
	Authenticate(string) (core.User, error)
	Logout(string) error
	AddAgent(core.User, service.Credentials) (core.User, error)
	Members(core.User) ([]core.User, error)
	UpdateMember(core.User, string, string, string, bool) (core.User, error)
}

type ResourceUseCases interface {
	RecordResponse(core.User, string) (core.Record, error)
	Escalate(core.User, string) (core.Record, error)
	Save(core.User, string, string, core.Record) (core.Record, error)
	Assign(core.User, string, string, string, bool) (core.Record, error)
	ReadConversation(core.User, string, uint64) error
	List(core.User, string, string) ([]core.Record, error)
}

type ConversationUseCases interface {
	IngestEmail(string, service.InboundEmail) (core.Record, error)
	DeliverEmail(context.Context, core.User, string, string) (core.Record, error)
	Draft(core.User, string) (service.Reply, error)
	StartChat(string, string, string, string, string) (string, error)
	PublicChat(string, string, string, string, int) ([]service.ChatMessage, error)
	ReadPublicChat(string) (service.PublicChatState, error)
	SendMessage(core.User, string, string, string, string, string, string) (core.Record, error)
	Reply(core.User, string) (service.Reply, error)
}

type DashboardUseCases interface {
	Report(core.User, time.Time, time.Time) (service.OperationsReport, error)
	Dashboard(core.User) (map[string]int, error)
}

type PlatformUseCases interface {
	Profile(core.User, string) (service.CustomerProfile, error)
	AddNote(core.User, string, string) (core.Record, error)
	MergeCustomers(core.User, string, string) (core.Record, error)
	RequestCode(tenant, email string) error
	Login(tenant, email, code string) (service.PortalCustomer, error)
	PortalTickets(tenant, token string) ([]core.Record, error)
	PortalTicket(tenant, token, id string) (core.Record, []core.Record, error)
	CreatePortalTicket(tenant, token, subject, content, priority, fileName, fileData string) (core.Record, error)
	CreateKey(core.User, string, string, int) (service.APIKey, error)
	ListKeys(core.User) ([]service.APIKey, error)
	RevokeKey(core.User, string) error
	ResolveAPIKey(string) (service.APIKey, core.User, error)
}

type KnowledgePublicUseCases interface {
	PublicList(tenant, slug string) ([]service.PublicArticle, error)
}

type CSATUseCases interface {
	IssueLink(core.Record) (service.CSATLink, error)
	ResolveLink(tenant, token string) (service.PublicResult, error)
	Submit(tenant, token string, rating int, comment string) (core.Record, error)
}

type WebhookUseCases interface {
	DeadLetters(core.User) ([]service.WebhookDeadLetter, error)
	Replay(core.User, string) error
	List(core.User) ([]service.Webhook, error)
	Save(core.User, string, service.Webhook) (service.Webhook, error)
	Remove(core.User, string) error
}

type ChannelUseCases interface {
	Configure(core.User, string, core.Record) (core.Record, error)
	Status(core.User) (map[string]core.Record, error)
}

type Dependencies struct {
	Admin                                  *service.Admin
	Operations                             *service.Operations
	AI                                     *service.AIService
	Workspace                              *service.Workspace
	InboundEmailTenant, InboundEmailSecret string
	ResendInbound                          *service.ResendInbound
	Auth                                   AuthUseCases
	Resources                              ResourceUseCases
	Conversations                          ConversationUseCases
	Dashboard                              DashboardUseCases
	Platform                               PlatformUseCases
	KnowledgePublic                        KnowledgePublicUseCases
	CSAT                                   CSATUseCases
	Webhooks                               WebhookUseCases
	Channels                               ChannelUseCases
	Governance                             *service.Governance
	Actions                                *service.AIActionService
	Skills                                 *service.AISkills
	Voicebot                               *service.Voicebot
}

type window struct {
	count int
	until time.Time
}

type Handler struct {
	admin                                  *service.Admin
	operations                             *service.Operations
	ai                                     *service.AIService
	workspace                              *service.Workspace
	inboundEmailTenant, inboundEmailSecret string
	resendInbound                          *service.ResendInbound
	auth                                   AuthUseCases
	resources                              ResourceUseCases
	conversations                          ConversationUseCases
	dashboard                              DashboardUseCases
	platform                               PlatformUseCases
	knowledgePublic                        KnowledgePublicUseCases
	csat                                   CSATUseCases
	webhooks                               WebhookUseCases
	channels                               ChannelUseCases
	governance                             *service.Governance
	actions                                *service.AIActionService
	skills                                 *service.AISkills
	voicebot                               *service.Voicebot
	mu                                     sync.Mutex
	attempts                               map[string]window
	aiRate                                 map[string]window
	keyRate                                map[string]window
	authSlots                              chan struct{}
}

func New(d Dependencies) http.Handler {
	h := &Handler{
		admin:              d.Admin,
		operations:         d.Operations,
		aiRate:             make(map[string]window),
		ai:                 d.AI,
		workspace:          d.Workspace,
		inboundEmailTenant: d.InboundEmailTenant, inboundEmailSecret: d.InboundEmailSecret,
		resendInbound:   d.ResendInbound,
		auth:            d.Auth,
		resources:       d.Resources,
		conversations:   d.Conversations,
		dashboard:       d.Dashboard,
		platform:        d.Platform,
		knowledgePublic: d.KnowledgePublic,
		csat:            d.CSAT,
		webhooks:        d.Webhooks,
		channels:        d.Channels,
		governance:      d.Governance,
		actions:         d.Actions,
		skills:          d.Skills,
		voicebot:        d.Voicebot,
		attempts:        make(map[string]window),
		keyRate:         make(map[string]window),
		authSlots:       make(chan struct{}, maximumConcurrentAuthTasks),
	}
	mux := http.NewServeMux()
	h.workspaceRoutes(mux)
	h.adminRoutes(mux)
	h.operationsRoutes(mux)
	h.aiRoutes(mux)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			method(w, http.MethodGet)
			return
		}
		respond(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
	})
	mux.HandleFunc("/api/v1/chat/{tenant}", h.chat)
	mux.HandleFunc("POST /api/v1/email/inbound", h.inboundEmail)
	mux.HandleFunc("POST /api/v1/email/resend", h.inboundResend)
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages/{message}/deliver", h.protected(h.deliverEmail))
	mux.HandleFunc("/api/v1/auth/register", h.authEndpoint(h.register))
	mux.HandleFunc("/api/v1/auth/login", h.authEndpoint(h.login))
	mux.HandleFunc("/api/v1/auth/me", h.protected(h.me))
	mux.HandleFunc("/api/v1/auth/logout", h.protected(h.logout))
	mux.HandleFunc("POST /api/v1/auth/password", h.protected(h.accountAction("password")))
	mux.HandleFunc("POST /api/v1/team/invite", h.protected(h.accountAction("invite")))
	mux.HandleFunc("/api/v1/auth/forgot", h.authEndpoint(func(c service.Credentials) (any, int, error) { return nil, http.StatusOK, h.auth.SendReset(c.Email) }))
	mux.HandleFunc("/api/v1/auth/redeem", h.authEndpoint(func(c service.Credentials) (any, int, error) {
		return nil, http.StatusOK, h.auth.Redeem(c.Token, c.Password)
	}))
	mux.HandleFunc("/api/v1/team", h.protected(h.team))
	mux.HandleFunc("PUT /api/v1/team/{id}", h.protected(h.updateMember))
	mux.HandleFunc("POST /api/v1/conversations/{id}/assignment", h.protected(h.assignment(core.KindConversations)))
	mux.HandleFunc("POST /api/v1/tickets/{id}/assignment", h.protected(h.assignment(core.KindTickets)))
	mux.HandleFunc("POST /api/v1/conversations/{id}/read", h.protected(h.readConversation))
	mux.HandleFunc("/api/v1/dashboard", h.protected(h.stats))
	mux.HandleFunc("GET /api/v1/reports", h.protected(h.report))
	mux.HandleFunc("POST /api/v1/tickets/{id}/escalate", h.protected(h.escalate))
	mux.HandleFunc("POST /api/v1/tickets/{id}/response", h.protected(h.recordResponse))
	for kind, decoder := range resourceDecoders() {
		mux.HandleFunc("/api/v1/"+kind, h.protected(h.resource(kind, decoder)))
		mux.HandleFunc("/api/v1/"+kind+"/{id}", h.protected(h.resource(kind, decoder)))
	}
	mux.HandleFunc("/api/v1/conversations/{id}/messages", h.protected(h.messages))
	mux.HandleFunc("POST /api/v1/conversations/{id}/draft", h.protected(h.draft))
	mux.HandleFunc("/api/v1/conversations/{id}/reply", h.protected(h.reply))
	h.platformRoutes(mux)
	h.liveChannelRoutes(mux)
	h.governanceRoutes(mux)
	h.aiActionRoutes(mux)
	h.aiSkillRoutes(mux)
	h.voicebotRoutes(mux)
	h.publicRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusNotFound, nil, core.Fail(http.StatusNotFound, "Endpoint không tồn tại"))
	})
	return securityHeaders(recoverPanics(mux))
}
