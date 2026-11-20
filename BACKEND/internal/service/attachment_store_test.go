package service

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestAttachmentsStoreReferencesAndMigrateLegacyCopies(t *testing.T) {
	operations, _, owner := operationsFixture(t, time.Now())
	workspace := operations.repo
	objects := map[string][]byte{}
	var mutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		defer mutex.Unlock()
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=test-key/") {
			t.Error("missing object signing credentials")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPut {
			data, _ := io.ReadAll(r.Body)
			if r.Header.Get("X-Amz-Content-Sha256") != sha256Hex(data) {
				t.Error("incorrect upload payload hash")
			}
			objects[r.URL.Path] = data
			return
		}
		if data, ok := objects[r.URL.Path]; ok {
			_, _ = w.Write(data)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	t.Setenv("ATTACHMENT_S3_ENDPOINT", server.URL)
	t.Setenv("ATTACHMENT_S3_BUCKET", "test-bucket")
	t.Setenv("ATTACHMENT_S3_ACCESS_KEY", "test-key")
	t.Setenv("ATTACHMENT_S3_SECRET_KEY", "test-secret")
	t.Setenv("ATTACHMENT_MIGRATE_ON_READ", "true")
	storage, err := AttachmentStoreFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7 fixture"))
	write := func(id string) {
		t.Helper()
		if err := workspace.UpdateTenant(owner.TenantID, func(store core.Writer) error {
			store.PutRecord(core.Record{ID: id, Kind: core.KindMessages, TenantID: owner.TenantID,
				Sender: "note", AttachmentName: "demo.pdf", AttachmentData: payload})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	read := func(id string) (record core.Record) {
		t.Helper()
		if err := workspace.ViewTenant(owner.TenantID, func(store core.Reader) error {
			record, _ = store.Record(owner.TenantID, id, core.KindMessages)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return
	}
	write("legacy")
	workspace.ConfigureAttachments(storage)
	write("external")
	current := read("external")
	if current.AttachmentData != "" || current.AttachmentKey == "" {
		t.Fatalf("new attachment stored inline: %+v", current)
	}
	foreign := current
	foreign.TenantID = "unrelated-tenant"
	if err := workspace.HydrateAttachment(context.Background(), &foreign); err == nil {
		t.Fatal("cross-tenant attachment reference accepted")
	}
	for _, id := range []string{"legacy", "external"} {
		record := read(id)
		if err := workspace.HydrateAttachment(context.Background(), &record); err != nil || record.AttachmentData != payload {
			t.Fatalf("%s attachment was not readable: %v", id, err)
		}
	}
	if migrated := read("legacy"); migrated.AttachmentKey != current.AttachmentKey || migrated.AttachmentData != "" {
		t.Fatalf("legacy attachment not migrated: %+v", migrated)
	}
}
