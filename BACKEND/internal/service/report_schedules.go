package service

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const reportSchedulesKind = "report_schedules"
const operationMailKind = "operation_mail"

type ReportSchedule struct {
	ID        string     `json:"id,omitempty"`
	Name      string     `json:"name"`
	Frequency string     `json:"frequency"`
	Recipient string     `json:"recipient_id"`
	Enabled   bool       `json:"enabled"`
	Hour      int        `json:"hour"`
	Day       int        `json:"day"`
	NextAt    *time.Time `json:"next_at,omitempty"`
}

func (o *Operations) Schedules(user core.User) ([]ReportSchedule, error) {
	var items []ReportSchedule
	err := o.repo.View(func(store core.Reader) error {
		items = workflowConfig[ReportSchedule](store, user.TenantID, reportSchedulesKind)
		return nil
	})
	return items, err
}

func (o *Operations) SaveSchedule(user core.User, input ReportSchedule) (ReportSchedule, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len([]rune(input.Name)) > 100 || input.Hour < 0 || input.Hour > 23 ||
		!(input.Frequency == "weekly" && input.Day >= 1 && input.Day <= 7 || input.Frequency == "monthly" && input.Day >= 1 && input.Day <= 28) {
		return input, core.Fail(400, "Tên, chu kỳ hoặc giờ gửi không hợp lệ")
	}
	err := o.repo.Update(func(store core.Writer) error {
		member, ok := store.UserByID(input.Recipient)
		if !ok || member.TenantID != user.TenantID || member.Disabled || member.Email == "" {
			return core.Fail(400, "Người nhận phải là thành viên đang hoạt động")
		}
		if input.ID == "" {
			input.ID = o.repo.IDs.NewID()
		} else {
			existing, ok := store.Record(user.TenantID, input.ID, reportSchedulesKind)
			if !ok || existing.Status == "removed" {
				return core.Fail(404, "Không tìm thấy lịch gửi")
			}
		}
		input.NextAt = nil
		if input.Enabled {
			next := scheduledNext(input, o.clock.Now())
			input.NextAt = &next
		}
		return o.saveConfig(store, user.TenantID, reportSchedulesKind, input.ID, input.Enabled, input)
	})
	return input, err
}

func scheduledNext(s ReportSchedule, now time.Time) time.Time {
	local := now.In(ReportZone)
	if s.Frequency == "weekly" {
		date := reportWeek(local).AddDate(0, 0, s.Day-1).Add(time.Duration(s.Hour) * time.Hour)
		if !date.After(local) {
			date = date.AddDate(0, 0, 7)
		}
		return date
	}
	date := time.Date(local.Year(), local.Month(), s.Day, s.Hour, 0, 0, 0, ReportZone)
	if !date.After(local) {
		date = date.AddDate(0, 1, 0)
	}
	return date
}

func scheduledPeriod(schedule ReportSchedule, now time.Time) (time.Time, time.Time) {
	day := reportDay(now)
	if schedule.Frequency == "weekly" {
		end := reportWeek(day)
		return end.AddDate(0, 0, -7), end
	}
	end := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, ReportZone)
	return end.AddDate(0, -1, 0), end
}

func (o *Operations) scheduleReports() {
	if o.mailer == nil {
		return
	}
	now := o.clock.Now()
	type entry struct {
		tenant   string
		schedule ReportSchedule
	}
	due := []entry{}
	_ = o.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			for _, schedule := range workflowConfig[ReportSchedule](store, tenant, reportSchedulesKind) {
				if schedule.Enabled && schedule.NextAt != nil && !schedule.NextAt.After(now) {
					due = append(due, entry{tenant, schedule})
				}
				if len(due) >= 20 {
					return nil
				}
			}
		}
		return nil
	})
	for _, item := range due {
		s := item.schedule
		from, end := scheduledPeriod(s, now)
		report, err := o.dashboard.Report(core.User{TenantID: item.tenant}, from, end)
		if err != nil {
			continue
		}
		body, attachment := scheduledContent(report, from, end)
		_ = o.repo.Update(func(store core.Writer) error {
			existing, ok := store.Record(item.tenant, s.ID, reportSchedulesKind)
			var current ReportSchedule
			if !ok || !existing.Enabled || json.Unmarshal([]byte(existing.Content), &current) != nil || current.NextAt == nil || !current.NextAt.Equal(*s.NextAt) {
				return nil
			}
			key := tokenHash("report:" + s.ID + ":" + end.Format(time.DateOnly))
			o.queueMail(store, item.tenant, key, s.Recipient, "Báo cáo Nexa · "+s.Name, body, now, attachment, s.ID)
			next := scheduledNext(s, now)
			s.NextAt = &next
			return o.saveConfig(store, item.tenant, reportSchedulesKind, s.ID, s.Enabled, s)
		})
	}
}

func scheduledContent(report OperationsReport, from, end time.Time) (string, string) {
	text := fmt.Sprintf("Kỳ báo cáo %s đến %s (giờ Việt Nam)\nPhiếu mới: %d\nĐã giải quyết: %d\nTồn hiện tại: %d\n", from.Format("02/01/2006"), end.AddDate(0, 0, -1).Format("02/01/2006"), report.Total.Opened, report.Total.Resolved, report.Total.Backlog)
	var buffer strings.Builder
	buffer.WriteRune('\uFEFF')
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"Nhân viên", "Phiếu mới", "Đã giải quyết", "Phút phản hồi TB", "CSAT", "Lượt đánh giá"})
	for _, row := range report.Agents {
		name := row.Name
		stripped := strings.TrimLeftFunc(name, func(r rune) bool { return r <= 32 || r == '\uFEFF' })
		if stripped != "" && strings.ContainsRune("=+-@", rune(stripped[0])) {
			name = "'" + name
		}
		response, csat := "", ""
		if row.Responses > 0 {
			response = fmt.Sprintf("%.1f", row.AverageResponseMinutes)
		}
		if row.Ratings > 0 {
			csat = fmt.Sprintf("%.1f", row.AverageRating)
		}
		_ = writer.Write([]string{name, strconv.Itoa(row.Opened), strconv.Itoa(row.Resolved), response, csat, strconv.Itoa(row.Ratings)})
	}
	writer.Flush()
	return text, base64.StdEncoding.EncodeToString([]byte(buffer.String()))
}

func (o *Operations) queueMail(store core.Writer, tenant, id, recipient, subject, body string, now time.Time, attachments ...string) {
	if _, ok := store.Record(tenant, id, operationMailKind); ok {
		return
	}
	record := core.Record{ID: id, TenantID: tenant, Kind: operationMailKind, AuthorID: recipient, Subject: subject, Content: body, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if len(attachments) > 0 {
		record.AttachmentName, record.AttachmentData = "bao-cao-nexa.csv", attachments[0]
	}
	if len(attachments) > 1 {
		record.AutomationID = attachments[1]
	}
	store.PutRecord(record)
}

func (o *Operations) deliverMail(ctx context.Context) {
	now := o.clock.Now()
	jobs := []core.Record{}
	_ = o.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			for _, job := range store.Records(tenant, operationMailKind) {
				if job.Enabled && !job.UpdatedAt.After(now) {
					jobs = append(jobs, job)
				}
				if len(jobs) >= 8 {
					return nil
				}
			}
		}
		return nil
	})
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		var member core.User
		scheduled := true
		_ = o.repo.View(func(store core.Reader) error {
			member, _ = store.UserByID(job.AuthorID)
			if job.AutomationID != "" {
				config, ok := store.Record(job.TenantID, job.AutomationID, reportSchedulesKind)
				var schedule ReportSchedule
				scheduled = ok && config.Enabled && json.Unmarshal([]byte(config.Content), &schedule) == nil && schedule.Recipient == job.AuthorID
			}
			return nil
		})
		var delivery error
		status := "skipped"
		if scheduled && member.TenantID == job.TenantID && !member.Disabled && member.Email != "" && now.Sub(job.CreatedAt) < 23*time.Hour {
			delivery = o.mailer.Send(member.Email, job.Subject, job.Content, job.ID, job.AttachmentName, job.AttachmentData)
			status = "sent"
		}
		_ = o.repo.Update(func(store core.Writer) error {
			current, ok := store.Record(job.TenantID, job.ID, operationMailKind)
			if !ok || !current.Enabled {
				return nil
			}
			current.Enabled = false
			current.Status = status
			if delivery != nil {
				current.Sequence++
				current.Enabled = current.Sequence < 8
				current.Status = "failed"
				if current.Enabled {
					current.Status = "retry"
				}
				current.UpdatedAt = now.Add(time.Duration(1<<min(current.Sequence, 6)) * time.Minute)
			}
			if !current.Enabled {
				current.Content = ""
				current.AttachmentData = ""
			}
			store.PutRecord(current)
			return nil
		})
	}
}
