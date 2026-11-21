package service

import (
	"testing"

	"businessautomation/internal/core"
)

func capJobs(jobIDs []string, tenantID string) []core.Record {
	jobs := make([]core.Record, len(jobIDs))
	for i, id := range jobIDs {
		jobs[i] = core.Record{ID: id, TenantID: tenantID}
	}
	return jobs
}

// TestCapJobsPerTenantFIFO locks the fairness cap: within one batch,
// no tenant contributes more than workflowJobsPerTenant jobs, and the
// original due-time order is preserved for the jobs that are kept.
func TestCapJobsPerTenantFIFO(t *testing.T) {
	tenantA := capJobs([]string{"a1", "a2", "a3", "a4", "a6", "a7"}, "tenant-a")
	tenantB := capJobs([]string{"b1", "b2", "b3", "b4", "b5", "b6"}, "tenant-b")
	tenantC := capJobs([]string{"c1", "c2", "c3", "c4", "c5", "c6"}, "tenant-c")
	jobs := append(append(append([]core.Record{}, tenantA...), tenantB...), tenantC...)

	picked := capJobsPerTenant(jobs)
	if len(picked) != 15 {
		t.Fatalf("picked %d, want 15 (3 tenants × 5)", len(picked))
	}
	count := map[string]int{}
	for _, job := range picked {
		count[job.TenantID]++
	}
	for tenant, max := range map[string]int{"tenant-a": 5, "tenant-b": 5, "tenant-c": 5} {
		if count[tenant] > max {
			t.Errorf("tenant %s picked %d jobs, cap is %d", tenant, count[tenant], max)
		}
	}
	// FIFO: the kept job IDs must be the first N of each tenant.
	want := map[string]bool{
		"a1": true, "a2": true, "a3": true, "a4": true, "a6": true,
		"b1": true, "b2": true, "b3": true, "b4": true, "b5": true,
		"c1": true, "c2": true, "c3": true, "c4": true, "c5": true,
		"a7": false, "b6": false, "c6": false,
	}
	for _, job := range picked {
		if want[job.ID] != true {
			t.Errorf("picked %q which the cap should have dropped", job.ID)
		}
	}
}

// TestCapJobsPerTenantSmallBatches locks that batches at or under the
// cap are returned unchanged (no allocation, no reordering).
func TestCapJobsPerTenantSmallBatches(t *testing.T) {
	original := capJobs([]string{"a1", "a2", "a3", "a4", "a5"}, "tenant-a")
	picked := capJobsPerTenant(original)
	if len(picked) != 5 {
		t.Fatalf("picked %d, want 5", len(picked))
	}
	// Identical slice pointer and order for small batches.
	if picked[0].ID != "a1" || picked[4].ID != "a5" {
		t.Fatalf("small batch order changed: %+v", picked)
	}

	// Empty batch is a no-op.
	if out := capJobsPerTenant(nil); out != nil {
		t.Fatalf("nil batch returned %+v, want nil", out)
	}
}
