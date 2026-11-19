package service

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"businessautomation/internal/core"
)

const MaxAudioBytes = 4 << 20

func (s *AIService) Transcribe(data []byte) (string, error) {
	return s.transcribe("", data)
}

func (s *AIService) transcribe(tenant string, data []byte) (string, error) {
	if !s.Enabled() {
		return "", core.Fail(503, "Chưa cấu hình OpenAI")
	}
	if len(data) < 12 || len(data) > MaxAudioBytes {
		return "", core.Fail(400, "Âm thanh không hợp lệ hoặc vượt 4 MiB")
	}
	kind := http.DetectContentType(data)
	var file string
	switch {
	case bytes.HasPrefix(data, []byte{0x1a, 0x45, 0xdf, 0xa3}):
		file = "voice.webm"
	case bytes.HasPrefix(data, []byte("OggS")):
		file = "voice.ogg"
	case string(data[4:8]) == "ftyp":
		file = "voice.m4a"
	case kind == "audio/wave":
		file = "voice.wav"
	case bytes.HasPrefix(data, []byte("ID3")) || data[0] == 0xff && data[1]&0xe0 == 0xe0:
		file = "voice.mp3"
	default:
		return "", core.Fail(400, "Hỗ trợ WEBM, OGG, M4A, WAV và MP3")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	model := os.Getenv("OPENAI_TRANSCRIPTION_MODEL")
	if model != "whisper-1" {
		model = "gpt-transcribe"
	}
	if err := writer.WriteField("model", model); err != nil {
		return "", err
	}
	if model == "whisper-1" {
		if err := writer.WriteField("response_format", "verbose_json"); err != nil {
			return "", err
		}
	}
	part, err := writer.CreateFormFile("file", file)
	if err != nil {
		return "", err
	}
	if _, err = part.Write(data); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	request, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+s.model.key)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	client := *s.model.client
	client.Timeout = 25 * time.Second
	response, err := client.Do(request)
	if err != nil {
		return "", core.Fail(502, "Không kết nối được dịch vụ chuyển giọng nói")
	}
	defer response.Body.Close()
	var result struct {
		Text     string
		Usage    providerUsage
		Duration float64
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) != nil || result.Text == "" {
		return "", core.Fail(502, "Không chuyển được giọng nói thành văn bản")
	}
	if result.Usage.Seconds == 0 {
		result.Usage.Seconds = result.Duration
	}
	s.model.recordUsage(tenant, model, result.Usage)
	return clipped(result.Text, 4000), nil
}

func validSentiment(sentiment string) bool {
	return sentiment == "neutral" || sentiment == "negative" || sentiment == "angry"
}

type VoiceAnalysis struct {
	Text      string `json:"text"`
	Intent    string `json:"intent,omitempty"`
	Sentiment string `json:"sentiment,omitempty"`
}

func (s *AIService) AnalyzeVoice(tenant string, data []byte) (VoiceAnalysis, error) {
	text, err := s.transcribe(tenant, data)
	if err != nil {
		return VoiceAnalysis{}, err
	}
	result := VoiceAnalysis{Text: text}
	var analysis VoiceAnalysis
	if err := s.model.complete(tenant, "Từ bản ghi giọng nói của khách, trả JSON intent (ý định ngắn bằng tiếng Việt) và sentiment (neutral, negative hoặc angry). Chỉ dùng angry khi lời nói có dấu hiệu tức giận rõ.", text, 120, &analysis); err == nil && validSentiment(analysis.Sentiment) {
		result.Intent = clipped(analysis.Intent, 120)
		result.Sentiment = analysis.Sentiment
	}
	return result, nil
}
