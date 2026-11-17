package api

import (
	"net/http"
	"sync"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

type AuthUseCases interface {
	Register(service.Credentials) (core.User, error)
	Login(service.Credentials) (string, error)
	Authenticate(string) (core.User, error)
	Logout(string) error
	AddAgent(core.User, service.Credentials) (core.User, error)
}

type ResourceUseCases interface {
	Save(core.User, string, string, core.Record) (core.Record, error)
	List(core.User, string, string) ([]core.Record, error)
}

type ConversationUseCases interface {
	Message(core.User, string, string, string) (core.Record, error)
	Reply(core.User, string) (service.Reply, error)
}

type DashboardUseCases interface {
	Dashboard(core.User) (map[string]int, error)
}

type Dependencies struct {
	Auth          AuthUseCases
	Resources     ResourceUseCases
	Conversations ConversationUseCases
	Dashboard     DashboardUseCases
}

type window struct {
	count int
	until time.Time
}

type Handler struct {
	auth          AuthUseCases
	resources     ResourceUseCases
	conversations ConversationUseCases
	dashboard     DashboardUseCases
	mu            sync.Mutex
	attempts      map[string]window
	authSlots     chan struct{}
}

func New(d Dependencies) http.Handler {
	h := &Handler{
		auth:          d.Auth,
		resources:     d.Resources,
		conversations: d.Conversations,
		dashboard:     d.Dashboard,
		attempts:      make(map[string]window),
		authSlots:     make(chan struct{}, maximumConcurrentAuthTasks),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			method(w, http.MethodGet)
			return
		}
		respond(w, http.StatusOK, map[string]string{"status": "ok"}, nil)
	})
	mux.HandleFunc("/api/v1/auth/register", h.authEndpoint(h.register))
	mux.HandleFunc("/api/v1/auth/login", h.authEndpoint(h.login))
	mux.HandleFunc("/api/v1/auth/me", h.protected(h.me))
	mux.HandleFunc("/api/v1/auth/logout", h.protected(h.logout))
	mux.HandleFunc("/api/v1/team", h.protected(h.team))
	mux.HandleFunc("/api/v1/dashboard", h.protected(h.stats))
	for kind, decoder := range resourceDecoders() {
		mux.HandleFunc("/api/v1/"+kind, h.protected(h.resource(kind, decoder)))
		mux.HandleFunc("/api/v1/"+kind+"/{id}", h.protected(h.resource(kind, decoder)))
	}
	mux.HandleFunc("/api/v1/conversations/{id}/messages", h.protected(h.messages))
	mux.HandleFunc("/api/v1/conversations/{id}/reply", h.protected(h.reply))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusNotFound, nil, core.Fail(http.StatusNotFound, "Endpoint không tồn tại"))
	})
	return securityHeaders(recoverPanics(mux))
}
