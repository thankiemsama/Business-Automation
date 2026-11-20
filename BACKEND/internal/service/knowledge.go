package service

import (
	"strings"

	"businessautomation/internal/core"
)

const (
	localKnowledgeProviderName = "local-knowledge-search"
	humanHandoffMessage        = "Chúng tôi chưa tìm thấy câu trả lời phù hợp. Vui lòng chờ nhân viên hỗ trợ."
	minimumSearchTokenLength   = 3
)

type Answer struct {
	knowledgeGap bool
	Content      string
	SourceID     string
	Provider     string
	NeedsHuman   bool
	Confidence   float64
	Sentiment    string
}

// ReplyProvider selects the content to persist as an assistant message.
type ReplyProvider interface {
	Answer(question string, articles []core.Record) (Answer, error)
}

type LocalKnowledgeProvider struct{}

func (LocalKnowledgeProvider) Answer(question string, articles []core.Record) (Answer, error) {
	result := Answer{
		Content:    humanHandoffMessage,
		Provider:   localKnowledgeProviderName,
		NeedsHuman: true,
	}
	tokens := searchTokens(question)
	bestScore := 0
	var bestArticle core.Record

	for _, article := range articles {
		if article.Kind != core.KindKnowledge || !article.Enabled {
			continue
		}
		score := scoreArticle(article, tokens)
		winsTie := score == bestScore && score > 0 && article.ID < bestArticle.ID
		if score > bestScore || winsTie {
			bestScore = score
			bestArticle = article
		}
	}
	if bestScore == 0 {
		return result, nil
	}
	result.Content = bestArticle.Content
	result.SourceID = bestArticle.ID
	result.NeedsHuman = false
	return result, nil
}

func searchTokens(question string) map[string]struct{} {
	tokens := make(map[string]struct{})
	for _, word := range strings.Fields(strings.ToLower(question)) {
		word = strings.Trim(word, ".,!?;:")
		if len([]rune(word)) >= minimumSearchTokenLength {
			tokens[word] = struct{}{}
		}
	}
	return tokens
}

func scoreArticle(article core.Record, tokens map[string]struct{}) int {
	searchableText := strings.ToLower(article.Subject + " " + article.Content + " " + article.VideoTranscript)
	score := 0
	for token := range tokens {
		if strings.Contains(searchableText, token) {
			score++
		}
	}
	return score
}

func matchesAnyKeyword(content string, keywords []string) bool {
	content = strings.ToLower(content)
	for _, keyword := range keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

var _ ReplyProvider = LocalKnowledgeProvider{}
