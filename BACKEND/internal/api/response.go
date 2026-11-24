package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"businessautomation/internal/core"
)

const maximumRequestBodyBytes = 64 << 10

func respond(w http.ResponseWriter, status int, data any, err error) {
	message := "Thành công"
	code := "SUCCESS"
	if err != nil {
		var e *core.Error
		if errors.As(err, &e) {
			status = e.Status
			message = e.Message
		} else {
			status = http.StatusInternalServerError
			message = "Lỗi hệ thống"
			slog.Error("request failed", "error", err)
		}
		code = http.StatusText(status)
		data = nil
	}
	var reason string
	if err != nil {
		var e *core.Error
		if errors.As(err, &e) {
			reason = e.Reason
		}
	}
	envelope := map[string]any{"data": data, "message": message, "code": code}
	if reason != "" {
		envelope["reason"] = reason
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) error {
	if strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]) != "application/json" {
		return core.Fail(http.StatusUnsupportedMediaType, "Content-Type phải là application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximumRequestBodyBytes)

	dec := json.NewDecoder(r.Body)
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return core.Fail(http.StatusBadRequest, "JSON không hợp lệ hoặc vượt quá 64 KB")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return core.Fail(http.StatusBadRequest, "Chỉ chấp nhận một JSON object")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return core.Fail(http.StatusBadRequest, "Body phải là JSON object")
	}
	fields := json.NewDecoder(bytes.NewReader(trimmed))
	fields.DisallowUnknownFields()
	if err := fields.Decode(dst); err != nil {
		return core.Fail(http.StatusBadRequest, "JSON không hợp lệ hoặc chứa trường không được hỗ trợ")
	}
	return nil
}
