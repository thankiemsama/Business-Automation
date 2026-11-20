package api

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"businessautomation/internal/core"
)

const (
	defaultPageSize   = 20
	maximumPageSize   = 100
	maximumPageNumber = 1000000
)

type listQuery struct {
	page     int
	limit    int
	search   string
	status   string
	priority string
	tags     []string
	assignee string
}

type pageResponse struct {
	Items []core.Record `json:"items"`
	Total int           `json:"total"`
	Page  int           `json:"page"`
	Limit int           `json:"limit"`
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request, records []core.Record, listError error) {
	if listError != nil {
		respond(w, http.StatusBadRequest, nil, listError)
		return
	}
	query, err := parseListQuery(r.URL.Query())
	if err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	filtered := filterRecords(records, query)
	start := min((query.page-1)*query.limit, len(filtered))
	end := min(start+query.limit, len(filtered))
	items := filtered[start:end]
	if h.workspace != nil {
		for i := range items {
			if err := h.workspace.HydrateAttachment(r.Context(), &items[i]); err != nil {
				respond(w, http.StatusServiceUnavailable, nil, err)
				return
			}
		}
	}
	respond(w, http.StatusOK, pageResponse{
		Items: items,
		Total: len(filtered),
		Page:  query.page,
		Limit: query.limit,
	}, nil)
}

func parseListQuery(values url.Values) (listQuery, error) {
	query := listQuery{
		search:   strings.ToLower(values.Get("q")),
		status:   values.Get("status"),
		priority: values.Get("priority"),
		tags:     values["tag"],
		assignee: values.Get("assignee"),
	}
	var err error
	query.page, err = parseBoundedInteger(values.Get("page"), 1, maximumPageNumber, "Page phải từ 1 đến 1000000")
	if err != nil {
		return query, err
	}
	query.limit, err = parseBoundedInteger(values.Get("limit"), defaultPageSize, maximumPageSize, "Limit phải từ 1 đến 100")
	return query, err
}

func parseBoundedInteger(raw string, defaultValue, maximum int, message string) (int, error) {
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximum {
		return 0, core.Fail(http.StatusBadRequest, message)
	}
	return value, nil
}

func filterRecords(records []core.Record, query listQuery) []core.Record {
	filtered := make([]core.Record, 0, len(records))
	for _, record := range records {
		if query.priority != "" && query.priority != record.Priority || query.assignee != "" && query.assignee != record.AssigneeID || !hasTags(record.Tags, query.tags) {
			continue
		}
		if query.status != "" && record.Status != query.status {
			continue
		}
		searchableText := strings.ToLower(record.Name + " " + record.Email + " " + record.Subject + " " + record.Content)
		if query.search != "" && !strings.Contains(searchableText, query.search) {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}
