package service

import (
	"sort"
	"strings"

	"businessautomation/internal/core"
)

// KnowledgeService backs the public knowledge base: SEO-friendly slugs plus
// public, read-only article access that guests can use before opening a ticket.
type KnowledgeService struct {
	repo core.Repository
	ids  IDGenerator
}

func NewKnowledge(repo core.Repository, ids IDGenerator) *KnowledgeService {
	return &KnowledgeService{repo: repo, ids: ids}
}

// PublicArticle is the sanitized shape exposed on /kb routes.
type PublicArticle struct {
	Slug    string `json:"slug"`
	Subject string `json:"subject"`
	Content string `json:"content"`
	Updated bool   `json:"-"`
}

// EnsureSlug gives a knowledge record a stable, unique, SEO-friendly slug.
func (s *KnowledgeService) EnsureSlug(store core.Reader, record *core.Record) error {
	if record.Slug != "" {
		record.Slug = slugify(record.Slug)
	} else {
		record.Slug = slugify(record.Subject)
	}
	if record.Slug == "" {
		record.Slug = "bai-viet-" + record.ID[:8]
	}
	used := map[string]bool{}
	for _, article := range store.Records(record.TenantID, core.KindKnowledge) {
		if article.ID != record.ID && article.Slug != "" {
			used[article.Slug] = true
		}
	}
	base, n := record.Slug, 1
	for used[base] {
		n++
		base = record.Slug + "-" + itoa(n)
	}
	record.Slug = base
	return nil
}

// PublicList returns the enabled articles for a tenant, optionally narrowed to one slug.
func (s *KnowledgeService) PublicList(tenant, slug string) ([]PublicArticle, error) {
	articles := []PublicArticle{}
	err := s.repo.View(func(store core.Reader) error {
		if len(store.Users(tenant)) == 0 {
			return core.Fail(404, "Không tìm thấy không gian hỗ trợ")
		}
		for _, record := range store.Records(tenant, core.KindKnowledge) {
			if !record.Public || record.Slug == "" {
				continue
			}
			if slug != "" && record.Slug != slug {
				continue
			}
			articles = append(articles, PublicArticle{Slug: record.Slug, Subject: record.Subject, Content: record.Content})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if slug != "" && len(articles) == 0 {
		return nil, core.Fail(404, "Không tìm thấy bài viết")
	}
	sort.Slice(articles, func(i, j int) bool { return articles[i].Subject < articles[j].Subject })
	return articles, nil
}

// assignKnowledgeSlug is used by KnowledgePolicy to give records stable slugs
// inside the same write transaction.
func assignKnowledgeSlug(store core.Reader, record *core.Record) {
	ks := &KnowledgeService{repo: nil, ids: nil}
	_ = ks.EnsureSlug(store, record)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// slugify folds diacritics (incl. Vietnamese) into ASCII and builds a URL slug.
func slugify(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range normalizeDiacritics(value) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	slug := strings.ToLower(b.String())
	slug = strings.Trim(slug, "-")
	if len([]rune(slug)) > 80 {
		slug = string([]rune(slug)[:80])
	}
	return strings.Trim(slug, "-")
}

// normalizeDiacritics lower-cases and folds combining/Unicode letters to ASCII.
func normalizeDiacritics(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		"à", "a", "á", "a", "ả", "a", "ã", "a", "ạ", "a", "â", "a", "ă", "a",
		"ê", "e", "è", "e", "é", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e", "ê", "e", "ế", "e", "ể", "e", "ễ", "e", "ệ", "e",
		"ì", "i", "í", "i", "ỉ", "i", "ĩ", "i", "ị", "i", "ô", "o", "ò", "o", "ó", "o", "ỏ", "o", "õ", "o", "ợ", "o", "ô", "o", "ố", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
		"ù", "u", "ú", "u", "ủ", "u", "ũ", "u", "ụ", "u", "ư", "u", "ừ", "u", "ứ", "u", "ử", "u", "ữ", "u", "ự", "u",
		"ỳ", "y", "ý", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y", "đ", "d",
		"ç", "c", "è", "e", "ñ", "n", "ü", "u",
	)
	return replacer.Replace(value)
}
