package api

import (
	"net/http"

	"businessautomation/internal/core"
)

func (h *Handler) resource(kind string, decoder resourceDecoder) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, u core.User, _ string) {
		key := r.PathValue("id")
		switch r.Method {
		case http.MethodGet:
			items, err := h.resources.List(u, kind, key)
			if key == "" {
				h.page(w, r, filterAssignments(items, u.ID, r.URL.Query().Get("assignment")), err)
				return
			}
			if err != nil {
				respond(w, http.StatusNotFound, nil, err)
				return
			}
			respond(w, http.StatusOK, items[0], nil)
		case http.MethodPost, http.MethodPut:
			if (r.Method == http.MethodPost && key != "") || (r.Method == http.MethodPut && key == "") {
				method(w, http.MethodGet)
				return
			}
			input, err := decoder(w, r)
			if err != nil {
				respond(w, http.StatusBadRequest, nil, err)
				return
			}
			result, err := h.resources.Save(u, kind, key, input)
			status := http.StatusOK
			if r.Method == http.MethodPost {
				status = http.StatusCreated
			}
			respond(w, status, result, err)
		default:
			allowed := "GET, POST"
			if key != "" {
				allowed = "GET, PUT"
			}
			method(w, allowed)
		}
	}
}

func (h *Handler) messages(w http.ResponseWriter, r *http.Request, u core.User, _ string) {
	key := r.PathValue("id")
	if r.Method == http.MethodGet {
		items, err := h.resources.List(u, core.KindMessages, key)
		h.page(w, r, items, err)
		return
	}
	if r.Method != http.MethodPost {
		method(w, "GET, POST")
		return
	}
	var input struct {
		Content        string `json:"content"`
		Sender         string `json:"sender"`
		AttachmentName string `json:"attachment_name"`
		AttachmentData string `json:"attachment_data"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	result, err := h.conversations.MessageWithAttachment(u, key, input.Content, input.Sender, input.AttachmentName, input.AttachmentData)
	respond(w, http.StatusCreated, result, err)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request, u core.User, _ string) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	result, err := h.conversations.Reply(u, r.PathValue("id"))
	respond(w, http.StatusCreated, result, err)
}
