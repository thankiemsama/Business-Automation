package api

import (
	"businessautomation/internal/core"
	"net/http"
	"time"
)

func (h *Handler) report(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	query := r.URL.Query()
	tz := time.FixedZone("ICT", 7*60*60)
	from, fromErr := time.ParseInLocation(time.DateOnly, query.Get("from"), tz)
	to, toErr := time.ParseInLocation(time.DateOnly, query.Get("to"), tz)
	if fromErr != nil || toErr != nil || to.Before(from) || to.Sub(from) > 366*24*time.Hour {
		respond(w, http.StatusBadRequest, nil, core.Fail(400, "Khoảng ngày không hợp lệ"))
		return
	}
	data, err := h.dashboard.Report(user, from, to.AddDate(0, 0, 1))
	respond(w, http.StatusOK, data, err)
}

func (h *Handler) recordResponse(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	ticket, err := h.resources.RecordResponse(user, r.PathValue("id"))
	respond(w, http.StatusOK, ticket, err)
}

func (h *Handler) escalate(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	ticket, err := h.resources.Escalate(user, r.PathValue("id"))
	respond(w, http.StatusOK, ticket, err)
}
