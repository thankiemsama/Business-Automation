package service

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"businessautomation/internal/core"
)

type OpenAIKnowledge struct {
	key, model  string
	client      *http.Client
	cacheMu     sync.Mutex
	vectors     map[string][]float64
	vectorIndex core.KnowledgeVectors
	meter       func(string, string, providerUsage)
	reserve     func(string) error
}

func NewOpenAIKnowledge(key, model string) *OpenAIKnowledge {
	if model == "" {
		model = "gpt-4.1-mini"
	}
	return &OpenAIKnowledge{key: key, model: model, client: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, vectors: make(map[string][]float64)}
}

func (p *OpenAIKnowledge) ConfigureVectorIndex(index core.KnowledgeVectors) { p.vectorIndex = index }

func (p *OpenAIKnowledge) request(path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		return core.Fail(502, "Không kết nối được OpenAI")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return core.Fail(502, "OpenAI trả về lỗi")
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(output); err != nil {
		return core.Fail(502, "Phản hồi OpenAI không hợp lệ")
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
