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
	var result struct{ Text string }
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) != nil || result.Text == "" {
		return "", core.Fail(502, "Không chuyển được giọng nói thành văn bản")
	}
	return clipped(result.Text, 4000), nil
}
