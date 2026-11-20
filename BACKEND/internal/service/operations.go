package service

import (
	"context"
	"encoding/json"
	"time"

	"businessautomation/internal/core"
)

type Operations struct {
	repo      *Workspace
	dashboard *DashboardService
	mailer    *EmailMailer
	clock     Clock
	aiModel   *OpenAIKnowledge
	idleAfter core.IdleWorkflowCandidate
}

func (o *Operations) ConfigureAIJobs(model *OpenAIKnowledge) {
	o.aiModel = model
	if model != nil {
		o.repo.enableAIJobs()
		o.repo.sparringModel = model.model
	}
}

func NewOperations(repo *Workspace, dashboard *DashboardService, mailer *EmailMailer, clock Clock) *Operations {
	return &Operations{repo: repo, dashboard: dashboard, mailer: mailer, clock: clock}
}

func (o *Operations) Run(ctx context.Context) {
	if o.aiModel != nil {
		go o.runQueuedAI(ctx)
	}
	if o.mailer != nil {
		go runAIWork(ctx, func() { o.deliverMail(ctx) })
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		o.scheduleIdle()
		o.runJobs()
		o.scheduleReports()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (o *Operations) runQueuedAI(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	o.runAIJobs()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case job := <-o.repo.realtimeQAReady:
			o.runRealtimeQA(job)
		case <-ticker.C:
			o.runAIJobs()
		}
	}
}

func (o *Operations) saveConfig(store core.Writer, tenant, kind, id string, enabled bool, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, exists := store.Record(tenant, id, kind); !exists {
		count := 0
		for _, record := range store.Records(tenant, kind) {
			if record.Status != "removed" {
				count++
			}
		}
		if count >= 50 {
			return core.Fail(409, "Tối đa 50 cấu hình đang lưu")
		}
	}
	store.PutRecord(core.Record{ID: id, TenantID: tenant, Kind: kind, Enabled: enabled, Content: string(data), UpdatedAt: o.clock.Now().UTC()})
	return nil
}

func (o *Operations) Remove(user core.User, kind, id string) error {
	if kind != workflowRulesKind && kind != reportSchedulesKind {
		return core.Fail(400, "Loại cấu hình không hợp lệ")
	}
	return o.repo.Update(func(store core.Writer) error {
		record, ok := store.Record(user.TenantID, id, kind)
		if !ok || record.Status == "removed" {
			return core.Fail(404, "Không tìm thấy cấu hình")
		}
		record.Enabled = false
		record.Status = "removed"
		store.PutRecord(record)
		return nil
	})
}

func (o *Operations) Analytics(user core.User, from, to time.Time) (Analytics, error) {
	return o.dashboard.Analytics(user, from, to, o.clock.Now())
}

func (o *Operations) MailEnabled() bool { return o.mailer != nil }
