package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"businessautomation/internal/core"
)

type OpenAIKnowledge struct {
	model       string
	provider    LLMProvider
	client      *http.Client
	cacheMu     sync.Mutex
	vectors     map[string][]float64
	vectorIndex core.KnowledgeVectors
	meter       func(string, string, providerUsage)
	reserve     func(string) error
}

// LLMProvider supplies protocol details for an OpenAI-compatible endpoint.
type LLMProvider interface {
	NewRequest(path string, body io.Reader) (*http.Request, error)
	TokenLimitField() string
	EmbeddingModel() string
	SupportsTranscription() bool
}

type compatibleLLM struct {
	name, baseURL, key, embedding string
}

func (p compatibleLLM) NewRequest(path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, p.baseURL+"/"+path, body)
	if err == nil && p.key != "" {
		req.Header.Set("Authorization", "Bearer "+p.key)
	}
	return req, err
}

func (p compatibleLLM) TokenLimitField() string {
	if p.name == "openai" {
		return "max_completion_tokens"
	}
	return "max_tokens"
}

func (p compatibleLLM) EmbeddingModel() string      { return p.embedding }
func (p compatibleLLM) SupportsTranscription() bool { return p.name == "openai" }

func NewOpenAIKnowledge(key, model string) *OpenAIKnowledge {
	if model == "" {
		model = "gpt-4.1-mini"
	}
	return newLLMKnowledge(compatibleLLM{"openai", "https://api.openai.com/v1", key, "text-embedding-3-small"}, model)
}

func NewCompatibleKnowledge(name, baseURL, key, model, embedding string) (*OpenAIKnowledge, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("LLM_CHAT_MODEL is required")
	}
	switch name {
	case "deepseek":
		if key == "" || baseURL != "" || embedding != "" {
			return nil, fmt.Errorf("DeepSeek requires DEEPSEEK_API_KEY and supports chat only")
		}
		baseURL = "https://api.deepseek.com"
	case "ollama":
		if baseURL == "" {
			baseURL = "http://127.0.0.1:11434/v1"
		}
	default:
		return nil, fmt.Errorf("unknown LLM_PROVIDER")
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" ||
		endpoint.ForceQuery || endpoint.Fragment != "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		return nil, fmt.Errorf("invalid LLM_BASE_URL")
	}
	return newLLMKnowledge(compatibleLLM{name, strings.TrimRight(baseURL, "/"), key, strings.TrimSpace(embedding)}, model), nil
}

func newLLMKnowledge(provider LLMProvider, model string) *OpenAIKnowledge {
	return &OpenAIKnowledge{provider: provider, model: model, client: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, vectors: make(map[string][]float64)}
}

func (p *OpenAIKnowledge) ConfigureVectorIndex(index core.KnowledgeVectors) { p.vectorIndex = index }

func (p *OpenAIKnowledge) request(path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := p.provider.NewRequest(path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		return core.Fail(502, "Không kết nối được nhà cung cấp AI")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return core.Fail(502, "Nhà cung cấp AI trả về lỗi")
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(output); err != nil {
		return core.Fail(502, "Phản hồi AI không hợp lệ")
	}
	return nil
}

func cosine(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, aa, bb float64
	for i, v := range a {
		dot += v * b[i]
		aa += v * v
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}
