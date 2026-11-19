package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func (h *Handler) workspaceRoutes(mux *http.ServeMux) {
	if h.workspace == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/events", h.protected(h.events))
	mux.HandleFunc("GET /api/v1/chat/{tenant}/events", h.guestEvents)
	mux.HandleFunc("GET /api/v1/notifications", h.protected(h.notifications))
	mux.HandleFunc("POST /api/v1/notifications/read", h.protected(h.readNotifications))
	mux.HandleFunc("GET /api/v1/views", h.protected(h.views))
	mux.HandleFunc("POST /api/v1/views", h.protected(h.views))
	mux.HandleFunc("POST /api/v1/views/remove", h.protected(h.removeView))
	mux.HandleFunc("GET /api/v1/search", h.protected(h.search))
	mux.HandleFunc("POST /api/v1/tickets/bulk", h.protected(h.bulk(core.KindTickets)))
	mux.HandleFunc("POST /api/v1/conversations/bulk", h.protected(h.bulk(core.KindConversations)))
	mux.HandleFunc("GET /api/v1/reports/export", h.protected(h.export))
	mux.HandleFunc("GET /api/v1/push", h.protected(h.push))
	mux.HandleFunc("POST /api/v1/push", h.protected(h.push))
	mux.HandleFunc("POST /api/v1/templates/{id}/export", h.protected(h.exportReplyTemplate))
	mux.HandleFunc("POST /api/v1/templates/import", h.protected(h.importReplyTemplate))
}

func (h *Handler) exportReplyTemplate(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	code, err := h.workspace.ExportReplyTemplate(user, r.PathValue("id"), input.Password)
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]string{"code": code}, err)
}

func (h *Handler) importReplyTemplate(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	record, err := h.workspace.ImportReplyTemplate(user, input.Code, input.Password)
	respond(w, 200, record, err)
}
func (h *Handler) events(w http.ResponseWriter, r *http.Request, user core.User, token string) {
	h.streamEvents(w, r, user.TenantID, func() bool {
		_, err := h.auth.Authenticate(token)
		return err == nil && (h.admin == nil || h.admin.Allowed(user.TenantID, requestIP(r)))
	})
}
func (h *Handler) guestEvents(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	scope, err := h.workspace.GuestScope(r.PathValue("tenant"), token)
	if err != nil {
		respond(w, 401, nil, err)
		return
	}
	h.streamEvents(w, r, scope, func() bool { return true })
}
func (h *Handler) streamEvents(w http.ResponseWriter, r *http.Request, scope string, valid func() bool) {
	cursor, events, stop := h.workspace.Watch(scope)
	defer stop()
	control := http.NewResponseController(w)
	if err := control.SetWriteDeadline(time.Now().Add(20 * time.Second)); err != nil && err != http.ErrNotSupported {
		respond(w, http.StatusInternalServerError, nil, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	since := r.Header.Get("Last-Event-ID")
	if since == "" {
		since = r.URL.Query().Get("since")
	}
	if since != cursor {
		if _, err := fmt.Fprintf(w, "id: %s\nevent: refresh\ndata: {}\n\n", cursor); err != nil {
			return
		}
	} else {
		if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
			return
		}
	}
	if control.Flush() != nil {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case cursor = <-events:
			if !valid() {
				return
			}
			_ = control.SetWriteDeadline(time.Now().Add(20 * time.Second))
			if _, err := fmt.Fprintf(w, "id: %s\nevent: refresh\ndata: {}\n\n", cursor); err != nil {
				return
			}
		case <-heartbeat.C:
			if !valid() {
				return
			}
			_ = control.SetWriteDeadline(time.Now().Add(20 * time.Second))
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
		}
		if control.Flush() != nil {
			return
		}
	}
}
func (h *Handler) notifications(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	items, err := h.workspace.Notifications(user)
	count := 0
	for _, item := range items {
		if item.ReadAt == nil {
			count++
		}
	}
	respond(w, 200, map[string]any{"unread": count, "items": items[:min(50, len(items))]}, err)
}
func (h *Handler) readNotifications(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		ID string `json:"id"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	respond(w, 200, nil, h.workspace.ReadNotifications(user, input.ID))
}
func (h *Handler) views(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method == http.MethodGet {
		items, err := h.workspace.Views(user)
		respond(w, 200, items, err)
		return
	}
	var input service.View
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	result, err := h.workspace.SaveView(user, input)
	respond(w, 200, result, err)
}
func (h *Handler) removeView(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		ID string `json:"id"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	respond(w, 200, nil, h.workspace.DeleteView(user, input.ID))
}
func (h *Handler) search(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	items, err := h.workspace.Search(user, r.URL.Query().Get("q"))
	respond(w, 200, items, err)
}
func (h *Handler) bulk(kind string) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		var input service.Bulk
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		count, err := h.workspace.Bulk(user, kind, input)
		respond(w, 200, map[string]int{"updated": count}, err)
	}
}
func hasTags(actual, required []string) bool {
	for _, tag := range required {
		found := false
		for _, value := range actual {
			if value == strings.ToLower(strings.Join(strings.Fields(tag), " ")) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
