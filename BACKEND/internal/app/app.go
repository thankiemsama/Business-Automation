// Package app is the composition root: concrete implementations are wired only here.
package app

import (
	"net/http"

	"businessautomation/internal/api"
	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func NewHandler(repo core.Repository) http.Handler {
	clock := service.SystemClock{}
	ids := service.RandomIDs{}
	auth := service.NewAuth(repo, clock, ids, service.PBKDF2Hasher{})
	resources := service.NewResources(repo, clock, ids, service.DefaultPolicies())
	conversations := service.NewConversations(repo, clock, ids, service.LocalKnowledgeProvider{}, service.NewKeywordAutomation(ids, clock))
	return api.New(api.Dependencies{
		Auth:          auth,
		Resources:     resources,
		Conversations: conversations,
		Dashboard:     service.NewDashboard(repo),
	})
}
