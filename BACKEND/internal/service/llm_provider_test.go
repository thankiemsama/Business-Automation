package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCompatibleLLMChatUsesConfiguredProvider(t *testing.T) {
	for _, tc := range []struct {
		name, base, key, endpoint string
	}{
		{"deepseek", "", "deepseek-key", "https://api.deepseek.com/chat/completions"},
		{"ollama", "http://ollama:11434/v1/", "", "http://ollama:11434/v1/chat/completions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := NewCompatibleKnowledge(tc.name, tc.base, tc.key, "test-model", "")
			if err != nil {
				t.Fatal(err)
			}
			model.client.Transport = webhookTransport(func(req *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				auth := ""
				if tc.key != "" {
					auth = "Bearer " + tc.key
				}
				if req.URL.String() != tc.endpoint || req.Header.Get("Authorization") != auth ||
					body["max_tokens"] != float64(200) || body["max_completion_tokens"] != nil || body["model"] != "test-model" {
					t.Fatalf("incorrect request to %s: %+v", req.URL, body)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
					`{"choices":[{"finish_reason":"stop","message":{"content":"{\"text\":\"ok\"}"}}]}`,
				))}, nil
			})
			var output struct{ Text string }
			if err := model.complete("tenant", "Return JSON", "draft", 200, &output); err != nil || output.Text != "ok" {
				t.Fatalf("chat failed: %+v %v", output, err)
			}
			if model.provider.SupportsTranscription() || model.provider.EmbeddingModel() != "" {
				t.Fatal("unsupported capabilities enabled")
			}
		})
	}
}
