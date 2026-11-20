package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const maxAttachmentBytes = 32 * 1024

type AttachmentStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
}

type s3Attachments struct {
	endpoint, bucket, region, key, secret string
	client                                *http.Client
}

var s3BucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
var s3RegionName = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

func AttachmentStoreFromEnvironment() (AttachmentStore, error) {
	endpoint := strings.TrimRight(os.Getenv("ATTACHMENT_S3_ENDPOINT"), "/")
	if endpoint == "" {
		return nil, nil
	}
	target, err := url.Parse(endpoint)
	bucket, region := os.Getenv("ATTACHMENT_S3_BUCKET"), os.Getenv("ATTACHMENT_S3_REGION")
	if region == "" {
		region = "us-east-1"
	}
	key, secret := os.Getenv("ATTACHMENT_S3_ACCESS_KEY"), os.Getenv("ATTACHMENT_S3_SECRET_KEY")
	if err != nil || target.Hostname() == "" || target.User != nil || target.Path != "" ||
		target.RawQuery != "" || target.ForceQuery || target.Fragment != "" ||
		(target.Scheme != "https" && target.Scheme != "http") ||
		!s3BucketName.MatchString(bucket) || !s3RegionName.MatchString(region) || key == "" || secret == "" {
		return nil, errors.New("cấu hình kho tệp S3 không hợp lệ")
	}
	return &s3Attachments{endpoint: endpoint, bucket: bucket, region: region, key: key, secret: secret,
		client: &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}}, nil
}

func (s *s3Attachments) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.request(ctx, http.MethodPut, key, data)
	return err
}

func (s *s3Attachments) Get(ctx context.Context, key string) ([]byte, error) {
	return s.request(ctx, http.MethodGet, key, nil)
}

func (s *s3Attachments) request(ctx context.Context, method, key string, data []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, s.endpoint+"/"+s.bucket+"/"+key, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	s.sign(request, data, time.Now())
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("kho tệp S3 từ chối yêu cầu")
	}
	result, err := io.ReadAll(io.LimitReader(response.Body, maxAttachmentBytes+1))
	if err != nil || len(result) > maxAttachmentBytes {
		return nil, errors.New("phản hồi kho tệp S3 không hợp lệ")
	}
	return result, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *s3Attachments) sign(request *http.Request, data []byte, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	day := stamp[:8]
	payload := sha256Hex(data)
	const headers = "host;x-amz-content-sha256;x-amz-date"
	canonical := request.Method + "\n" + request.URL.EscapedPath() + "\n\n" +
		"host:" + request.URL.Host + "\nx-amz-content-sha256:" + payload + "\nx-amz-date:" + stamp + "\n\n" +
		headers + "\n" + payload
	scope := day + "/" + s.region + "/s3/aws4_request"
	signingKey := []byte("AWS4" + s.secret)
	for _, part := range []string{day, s.region, "s3", "aws4_request"} {
		mac := hmac.New(sha256.New, signingKey)
		mac.Write([]byte(part))
		signingKey = mac.Sum(nil)
	}
	mac := hmac.New(sha256.New, signingKey)
	mac.Write([]byte("AWS4-HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + sha256Hex([]byte(canonical))))
	request.Header.Set("X-Amz-Date", stamp)
	request.Header.Set("X-Amz-Content-Sha256", payload)
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.key+"/"+scope+
		",SignedHeaders="+headers+",Signature="+hex.EncodeToString(mac.Sum(nil)))
}

func (s *Workspace) ConfigureAttachments(store AttachmentStore) { s.attachments = store }

func (s *Workspace) offloadAttachment(record *core.Record) error {
	if s.attachments == nil || record.Kind != core.KindMessages || record.AttachmentData == "" {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(record.AttachmentData)
	if err != nil || len(data) == 0 || len(data) > maxAttachmentBytes {
		return core.Fail(400, "Tệp đính kèm không hợp lệ")
	}
	key := tokenHash(record.TenantID) + "/" + sha256Hex(data)
	if err := s.attachments.Put(context.Background(), key, data); err != nil {
		return core.Fail(503, "Kho tệp đính kèm chưa khả dụng")
	}
	record.AttachmentKey, record.AttachmentData = key, ""
	return nil
}

// HydrateAttachment preserves the attachment wire format after access has been checked.
func (s *Workspace) HydrateAttachment(ctx context.Context, record *core.Record) error {
	if record.AttachmentKey == "" {
		if record.Kind == core.KindMessages && record.AttachmentData != "" && s.attachments != nil && os.Getenv("ATTACHMENT_MIGRATE_ON_READ") == "true" {
			s.migrateAttachment(record)
		}
		return nil
	}
	tenant, digest, _ := strings.Cut(record.AttachmentKey, "/")
	digestBytes, err := hex.DecodeString(digest)
	if s.attachments == nil || tenant != tokenHash(record.TenantID) || err != nil || len(digestBytes) != sha256.Size {
		return core.Fail(503, "Tệp đính kèm chưa khả dụng")
	}
	data, err := s.attachments.Get(ctx, record.AttachmentKey)
	if err != nil || len(data) == 0 || len(data) > maxAttachmentBytes || sha256Hex(data) != digest {
		return core.Fail(503, "Tệp đính kèm chưa khả dụng")
	}
	record.AttachmentData, record.AttachmentKey = base64.StdEncoding.EncodeToString(data), ""
	return nil
}

func (s *Workspace) migrateAttachment(record *core.Record) {
	candidate := *record
	if err := s.offloadAttachment(&candidate); err != nil {
		return
	}
	key := candidate.AttachmentKey
	if err := s.HydrateAttachment(context.Background(), &candidate); err != nil || candidate.AttachmentData != record.AttachmentData {
		return
	}
	_ = updateTenant(s.Repository, record.TenantID, func(store core.Writer) error {
		current, ok := store.Record(record.TenantID, record.ID, core.KindMessages)
		if ok && current.AttachmentKey == "" && current.AttachmentData == record.AttachmentData {
			current.AttachmentKey, current.AttachmentData = key, ""
			store.PutRecord(current)
		}
		return nil
	})
}
