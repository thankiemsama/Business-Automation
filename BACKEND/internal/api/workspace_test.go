package api_test

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/app"
	"businessautomation/internal/repository"
)

func TestWorkspaceChangesAndExports(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "workspace.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	owner := root.account("workspace@example.test")
	outsider := root.account("other-workspace@example.test")
	me := owner.call("GET", "/api/v1/auth/me", nil, 200)
	ticket := owner.call("POST", "/api/v1/tickets", map[string]any{"subject": "=HYPERLINK(1)", "tags": []string{"VIP", " vip ", "trả hàng"}}, 201)
	id := ticket["id"].(string)
	if len(ticket["tags"].([]any)) != 2 {
		t.Fatal("tags were not normalized", ticket)
	}
	list := owner.call("GET", "/api/v1/tickets?tag=VIP&tag=tr%E1%BA%A3+h%C3%A0ng", nil, 200)
	if list["total"] != float64(1) {
		t.Fatal("tag filters did not match", list)
	}
	view := owner.call("POST", "/api/v1/views", map[string]any{"name": "VIP", "kind": "tickets", "tags": []string{"vip"}}, 200)
	outsider.call("POST", "/api/v1/views/remove", map[string]any{"id": view["id"]}, 404)
	foreign := outsider.call("POST", "/api/v1/tickets", map[string]any{"subject": "Khác tenant"}, 201)
	owner.call("POST", "/api/v1/tickets/bulk", map[string]any{"action": "resolve", "ids": []any{id, foreign["id"]}}, 404)
	if ticket = owner.call("GET", "/api/v1/tickets/"+id, nil, 200); ticket["status"] != "open" {
		t.Fatal("bulk failed to roll back")
	}
	server := httptest.NewServer(root.h)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/events", nil)
	req.Header.Set("Authorization", "Bearer "+owner.token)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	reader := bufio.NewReader(stream.Body)
	readEvent := func() string {
		t.Helper()
		var result strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if line == "\n" {
				return result.String()
			}
			result.WriteString(line)
		}
	}
	if event := readEvent(); !strings.Contains(event, "event: refresh") {
		t.Fatal(event)
	}
	owner.call("POST", "/api/v1/tickets/bulk", map[string]any{"action": "assign", "ids": []any{id}, "assignee_id": me["id"]}, 200)
	if event := readEvent(); !strings.Contains(event, "event: refresh") {
		t.Fatal(event)
	}
	inbox := owner.call("GET", "/api/v1/notifications", nil, 200)
	if inbox["unread"] != float64(1) {
		t.Fatal(inbox)
	}
	notice := inbox["items"].([]any)[0].(map[string]any)
	outsider.call("POST", "/api/v1/notifications/read", map[string]any{"id": notice["id"]}, 404)
	owner.call("POST", "/api/v1/notifications/read", map[string]any{"id": notice["id"]}, 200)
	inbox = owner.call("GET", "/api/v1/notifications", nil, 200)
	if inbox["unread"] != float64(0) {
		t.Fatal(inbox)
	}
	found := false
	for _, format := range []string{"csv", "xlsx"} {
		req := httptest.NewRequest("GET", "/api/v1/reports/export?kind=tickets&format="+format, nil)
		req.Header.Set("Authorization", "Bearer "+owner.token)
		recorder := httptest.NewRecorder()
		root.h.ServeHTTP(recorder, req)
		if recorder.Code != 200 {
			t.Fatal(recorder.Body.String())
		}
		if format == "csv" {
			if !strings.Contains(recorder.Body.String(), "'=HYPERLINK(1)") || strings.Contains(recorder.Body.String(), "Khác tenant") {
				t.Fatal("unexpected CSV", recorder.Body.String())
			}
			continue
		}
		archive, err := zip.NewReader(bytes.NewReader(recorder.Body.Bytes()), int64(recorder.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range archive.File {
			if file.Name != "xl/worksheets/sheet1.xml" {
				continue
			}
			reader, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(reader)
			reader.Close()
			if err != nil || !bytes.Contains(content, []byte("=HYPERLINK(1)")) || bytes.Contains(content, []byte("<f>")) {
				t.Fatal("unexpected workbook", err, string(content))
			}
			found = true
		}
	}
	if !found {
		t.Fatal("worksheet missing")
	}
}
