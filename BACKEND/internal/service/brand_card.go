package service

import (
	"net/url"
	"strings"

	"businessautomation/internal/core"
)

type BrandCardSettings struct {
	Enabled    bool   `json:"enabled"`
	Intro      string `json:"intro"`
	ProductURL string `json:"product_url"`
	SupportURL string `json:"support_url"`
}

func validateBrandCard(name string, settings *BrandCardSettings) error {
	settings.Intro = strings.TrimSpace(settings.Intro)
	settings.ProductURL = strings.TrimSpace(settings.ProductURL)
	settings.SupportURL = strings.TrimSpace(settings.SupportURL)
	if (settings.Enabled && strings.TrimSpace(name) == "") || len([]rune(settings.Intro)) > 240 {
		return core.Fail(400, "Danh thiếp cần tên thương hiệu và lời giới thiệu tối đa 240 ký tự")
	}
	for _, value := range []string{settings.ProductURL, settings.SupportURL} {
		if value == "" {
			continue
		}
		link, err := url.Parse(value)
		if err != nil || link.Scheme != "https" || link.Hostname() == "" || link.User != nil || len(value) > 400 {
			return core.Fail(400, "Liên kết trên danh thiếp phải là URL HTTPS tối đa 400 ký tự")
		}
	}
	return nil
}

func firstReplyBrandCard(store core.Reader, conversation core.Record) *core.BrandCard {
	switch conversation.Channel {
	case core.ChannelWeb, core.ChannelZalo, core.ChannelWhatsApp:
	default:
		return nil
	}
	settings, err := tenantSettings(store, conversation.TenantID)
	if err != nil || !settings.BrandCard.Enabled {
		return nil
	}
	for _, message := range store.ConversationMessages(conversation.TenantID, conversation.ID) {
		if message.Sender == core.SenderAgent && message.BrandCard != nil {
			return nil
		}
	}
	return &core.BrandCard{Name: strings.TrimSpace(settings.DisplayName), Intro: settings.BrandCard.Intro,
		ProductURL: settings.BrandCard.ProductURL, SupportURL: settings.BrandCard.SupportURL}
}

func brandCardText(card *core.BrandCard) string {
	lines := []string{card.Name}
	if card.Intro != "" {
		lines = append(lines, card.Intro)
	}
	if card.ProductURL != "" {
		lines = append(lines, "Sản phẩm: "+card.ProductURL)
	}
	if card.SupportURL != "" {
		lines = append(lines, "Hỗ trợ: "+card.SupportURL)
	}
	return strings.Join(lines, "\n")
}
