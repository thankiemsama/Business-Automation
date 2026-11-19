package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSignedResendInboundRetrievesEmail(t *testing.T) {
	const id = "56761188-7520-42d8-8898-ff6fc54ce618"
	raw := `{"type":"email.received","data":{"email_id":"` + id + `","to":["support@example.com"]}}`
	secret := []byte("test-webhook-secret")
	now := time.Now().UTC()
	stamp := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("event-1." + stamp + "." + raw))
	header := http.Header{}
	header.Set("Svix-Id", "event-1")
	header.Set("Svix-Timestamp", stamp)
	header.Set("Svix-Signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	calls := 0
	receiver := &ResendInbound{secret: secret, address: "support@example.com", key: "test",
		client: &http.Client{Transport: webhookTransport(func(req *http.Request) (*http.Response, error) {
			calls++
			if req.URL.Path != "/emails/receiving/"+id || req.Header.Get("Authorization") != "Bearer test" {
				t.Fatal(req.URL)
			}
			body := `{"id":"` + id + `","to":["support@example.com"],"from":"user@example.com","text":"Xin chào","message_id":"<new@example.com>","headers":{"REFERENCES":"<root@example.com> <prior@example.com>"}}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
	if _, err := receiver.Receive(context.Background(), header, []byte(raw+" "), now); err == nil || calls != 0 {
		t.Fatal("tampered webhook was authenticated")
	}
	if _, err := receiver.Receive(context.Background(), header, []byte(raw), now.Add(time.Hour)); err == nil || calls != 0 {
		t.Fatal("stale webhook was authenticated")
	}
	in, err := receiver.Receive(context.Background(), header, []byte(raw), now)
	if err != nil || in == nil || in.MessageID != "<new@example.com>" || in.ThreadID != "<root@example.com>" || in.DeliveryID != "event-1" || calls != 1 {
		t.Fatal(in, err, calls)
	}
}
