package service

import (
	"encoding/json"
	"sort"
	"strings"

	"businessautomation/internal/core"
)

type View struct {
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Status     string   `json:"status"`
	Priority   string   `json:"priority"`
	Tags       []string `json:"tags"`
	Assignment string   `json:"assignment"`
	Assignee   string   `json:"assignee"`
	Search     string   `json:"search"`
}

func (s *Workspace) Views(user core.User) ([]View, error) {
	result := []View{}
	err := s.View(func(store core.Reader) error {
		for _, record := range store.Records(user.TenantID, "views") {
			if record.AuthorID != user.ID || !record.Enabled {
				continue
			}
			var view View
			if err := json.Unmarshal([]byte(record.Content), &view); err != nil {
				return err
			}
			result = append(result, view)
		}
		return nil
	})
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, err
}
func (s *Workspace) SaveView(user core.User, view View) (View, error) {
	view.Name = strings.TrimSpace(view.Name)
	if !workKind(view.Kind) || view.Name == "" || len([]rune(view.Name)) > 80 || len(view.Search) > 400 {
		return view, core.Fail(400, "Tên hoặc bộ lọc không hợp lệ")
	}
	var err error
	if view.Tags, err = normalizeTags(view.Tags); err != nil {
		return view, err
	}
	err = s.Update(func(store core.Writer) error {
		if view.ID == "" {
			view.ID = s.IDs.NewID()
		} else {
			existing, ok := store.Record(user.TenantID, view.ID, "views")
			if !ok || existing.AuthorID != user.ID || !existing.Enabled {
				return core.Fail(404, "Không tìm thấy bộ lọc")
			}
		}
		data, err := json.Marshal(view)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{ID: view.ID, TenantID: user.TenantID, Kind: "views", AuthorID: user.ID, Enabled: true, Content: string(data)})
		return nil
	})
	return view, err
}
func (s *Workspace) DeleteView(user core.User, id string) error {
	return s.Update(func(store core.Writer) error {
		record, ok := store.Record(user.TenantID, id, "views")
		if !ok || record.AuthorID != user.ID {
			return core.Fail(404, "Không tìm thấy bộ lọc")
		}
		record.Enabled = false
		store.PutRecord(record)
		return nil
	})
}

type Bulk struct {
	IDs      []string `json:"ids"`
	Action   string   `json:"action"`
	Assignee string   `json:"assignee_id"`
}

func workKind(kind string) bool { return kind == core.KindTickets || kind == core.KindConversations }

func (s *Workspace) Bulk(user core.User, kind string, cmd Bulk) (int, error) {
	if !workKind(kind) || len(cmd.IDs) == 0 || len(cmd.IDs) > 100 {
		return 0, core.Fail(400, "Chọn từ 1 đến 100 mục")
	}
	if cmd.Action != "assign" && !(cmd.Action == "resolve" && kind == core.KindTickets) && !(cmd.Action == "close" && kind == core.KindConversations) {
		return 0, core.Fail(400, "Thao tác không hợp lệ")
	}
	count := 0
	err := s.Update(func(store core.Writer) error {
		if cmd.Action == "assign" && cmd.Assignee != "" {
			target, ok := store.UserByID(cmd.Assignee)
			if !ok || target.TenantID != user.TenantID || target.Disabled {
				return core.Fail(400, "Người phụ trách không hợp lệ")
			}
		}
		now := s.Clock.Now().UTC()
		seen := map[string]bool{}
		for _, id := range cmd.IDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			record, err := findOwnedRecord(store, user, id, kind)
			if err != nil {
				return err
			}
			switch cmd.Action {
			case "assign":
				if record.AssigneeID == cmd.Assignee {
					continue
				}
				record.AssigneeID = cmd.Assignee
			case "resolve":
				if record.Status == core.StatusResolved {
					continue
				}
				record.Status, record.ResolvedAt, record.ResolvedBy = core.StatusResolved, &now, user.ID
			case "close":
				if record.Status == core.StatusClosed {
					continue
				}
				record.Status = core.StatusClosed
			}
			record.UpdatedAt = now
			store.PutRecord(record)
			count++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

type SearchResult struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	score int
}

func (s *Workspace) Search(user core.User, query string) ([]SearchResult, error) {
	result := []SearchResult{}
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return result, nil
	}
	if len(query) > 160 {
		return result, core.Fail(400, "Từ khóa quá dài")
	}
	err := s.View(func(store core.Reader) error {
		for _, kind := range []string{core.KindTickets, core.KindCustomers, core.KindConversations} {
			for _, record := range store.Records(user.TenantID, kind) {
				title := record.Subject
				if kind == core.KindCustomers {
					if record.Status == core.CustomerMerged {
						continue
					}
					title = record.Name
				}
				if title == "" {
					title = record.ID
				}
				score := fuzzyScore(strings.ToLower(title+" "+record.Email+" "+record.ID), query)
				if score >= 0 {
					result = append(result, SearchResult{ID: record.ID, Kind: kind, Title: title, score: score})
				}
			}
		}
		return nil
	})
	sort.Slice(result, func(i, j int) bool {
		if result[i].score == result[j].score {
			return result[i].Title < result[j].Title
		}
		return result[i].score < result[j].score
	})
	return result[:min(16, len(result))], err
}

func fuzzyScore(text, query string) int {
	if i := strings.Index(text, query); i >= 0 {
		return min(99, i)
	}
	score := 100
	for _, needle := range query {
		i := strings.IndexRune(text, needle)
		if i < 0 {
			return -1
		}
		score += i
		text = text[i+len(string(needle)):]
	}
	return score
}
