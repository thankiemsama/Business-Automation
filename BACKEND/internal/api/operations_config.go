package api

import (
	"net/http"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func (h *Handler) operationsRoutes(mux *http.ServeMux) {
	if h.operations == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/reports/analytics", h.protected(h.analytics))
	mux.HandleFunc("POST /api/v1/operations/workflows/migrate", h.protected(h.migrateWorkflow))
	mux.HandleFunc("GET /api/v1/operations/field-extraction", h.protected(h.fieldExtraction))
	mux.HandleFunc("POST /api/v1/operations/field-extraction", h.protected(h.fieldExtraction))
	for _, endpoint := range []string{"workflows", "schedules"} {
		mux.HandleFunc("GET /api/v1/operations/"+endpoint, h.protected(h.operationsConfig(endpoint)))
		mux.HandleFunc("POST /api/v1/operations/"+endpoint, h.protected(h.operationsConfig(endpoint)))
		mux.HandleFunc("POST /api/v1/operations/"+endpoint+"/remove", h.protected(h.removeConfig(endpoint)))
	}
}

func (h *Handler) fieldExtraction(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method == http.MethodGet {
		batches, err := h.operations.FieldExtractionBatches(user)
		respond(w, 200, batches, err)
		return
	}
	var input service.FieldExtractionRequest
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	respond(w, 202, nil, h.operations.QueueFieldExtraction(user, input))
}

func (h *Handler) migrateWorkflow(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !ownerIntegration(w, user) {
		return
	}
	var input struct {
		ID string `json:"id"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	result, err := h.operations.MigrateKeywordRule(user, input.ID)
	respond(w, 200, result, err)
}

func (h *Handler) analytics(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	query := r.URL.Query()
	from, e1 := time.ParseInLocation(time.DateOnly, query.Get("from"), service.ReportZone)
	to, e2 := time.ParseInLocation(time.DateOnly, query.Get("to"), service.ReportZone)
	if e1 != nil || e2 != nil || to.Before(from) || to.Sub(from) > 366*24*time.Hour {
		respond(w, 400, nil, core.Fail(400, "Khoảng ngày không hợp lệ"))
		return
	}
	data, err := h.operations.Analytics(user, from, to.AddDate(0, 0, 1))
	respond(w, 200, data, err)
}

func (h *Handler) operationsConfig(endpoint string) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		if endpoint == "workflows" {
			if r.Method == http.MethodGet {
				items, err := h.operations.Rules(user)
				respond(w, 200, items, err)
				return
			}
			var rule service.WorkflowRule
			if err := decode(w, r, &rule); err != nil {
				respond(w, 400, nil, err)
				return
			}
			saved, err := h.operations.SaveRule(user, rule)
			respond(w, 200, saved, err)
			return
		}
		if r.Method == http.MethodGet {
			items, err := h.operations.Schedules(user)
			respond(w, 200, map[string]any{"items": items, "email_enabled": h.operations.MailEnabled()}, err)
			return
		}
		var schedule service.ReportSchedule
		if err := decode(w, r, &schedule); err != nil {
			respond(w, 400, nil, err)
			return
		}
		saved, err := h.operations.SaveSchedule(user, schedule)
		respond(w, 200, saved, err)
	}
}

func (h *Handler) removeConfig(endpoint string) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var input struct {
			ID string `json:"id"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		kind := "workflow_rules"
		if endpoint == "schedules" {
			kind = "report_schedules"
		}
		respond(w, 200, nil, h.operations.Remove(user, kind, input.ID))
	}
}
