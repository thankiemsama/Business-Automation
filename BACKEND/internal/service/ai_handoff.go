package service

import "strings"

type AIHandoffPolicy struct {
	Enabled           bool    `json:"enabled"`
	MinimumConfidence float64 `json:"minimum_confidence"`
}

func requestsHuman(content string) bool {
	text := normalizeDiacritics(strings.ToLower(strings.Join(strings.Fields(content), " ")))
	for _, phrase := range []string{
		"gặp nhân viên", "gap nhan vien", "nói chuyện với nhân viên", "noi chuyen voi nhan vien",
		"chuyển cho nhân viên", "chuyen cho nhan vien", "cần nhân viên hỗ trợ", "can nhan vien ho tro",
		"talk to a human", "speak to a human", "human agent",
	} {
		if strings.Contains(text, normalizeDiacritics(phrase)) {
			return true
		}
	}
	return false
}
