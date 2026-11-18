package api

import (
	"businessautomation/internal/core"
	"net/http"
	"strings"
)

func (h *Handler) chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		method(w, "GET, POST")
		return
	}
	tenant := r.PathValue("tenant")
	secret := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if r.Method == http.MethodGet {
		messages, err := h.conversations.PublicChat(tenant+"."+secret, "", "", "", 0)
		respond(w, http.StatusOK, messages, err)
		return
	}
	if !h.allowAuth(r) {
		respond(w, http.StatusTooManyRequests, nil, core.Fail(429, "Vui lòng thử lại sau"))
		return
	}
	var input struct {
		AttachmentName string `json:"attachment_name"`
		AttachmentData string `json:"attachment_data"`
		Name           string `json:"name"`
		Content        string `json:"content"`
		Rating         int    `json:"rating"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	if secret == "" {
		token, err := h.conversations.StartChat(tenant, input.Name, input.Content, input.AttachmentName, input.AttachmentData)
		respond(w, http.StatusCreated, map[string]string{"token": token}, err)
		return
	}
	if strings.TrimSpace(input.Content) == "" && input.AttachmentData == "" && input.Rating == 0 {
		respond(w, http.StatusBadRequest, nil, core.Fail(400, "Nhập tin nhắn hoặc đánh giá"))
		return
	}
	messages, err := h.conversations.PublicChat(tenant+"."+secret, input.Content, input.AttachmentName, input.AttachmentData, input.Rating)
	respond(w, http.StatusOK, messages, err)
}
