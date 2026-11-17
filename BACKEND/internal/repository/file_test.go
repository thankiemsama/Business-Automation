package repository

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"businessautomation/internal/core"
)

func TestRollbackAndConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Update(func(st core.Writer) error {
		st.PutRecord(core.Record{ID: "bad", TenantID: "tenant", Kind: "customers"})
		return errors.New("rollback")
	}); err == nil {
		t.Fatal("expected error")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := r.Update(func(st core.Writer) error {
				key := string(rune('a' + i))
				st.PutRecord(core.Record{ID: key, TenantID: "tenant", Kind: "customers", Keywords: []string{"original"}})
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	check, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = check.View(func(st core.Reader) error {
		records := st.Records("tenant", "customers")
		if len(records) != 20 {
			t.Errorf("got %d records", len(records))
		}
		if _, ok := st.Record("tenant", "bad", "customers"); ok {
			t.Error("rollback failed")
		}
		records[0].Keywords[0] = "mutated"
		if len(st.Records("other", "")) != 0 {
			t.Error("tenant isolation failed")
		}
		return nil
	})
	_ = check.View(func(st core.Reader) error {
		for _, r := range st.Records("tenant", "") {
			if r.Keywords[0] != "original" {
				t.Error("view mutated state")
			}
		}
		return nil
	})
}

func TestFailedPersistenceDoesNotPublish(t *testing.T) {
	// A regular file cannot serve as the database parent directory.
	path := filepath.Join(t.TempDir(), "state.json")
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Update(func(st core.Writer) error { st.PutRecord(core.Record{ID: "original", TenantID: "tenant"}); return nil }); err != nil {
		t.Fatal(err)
	}
	r.path = filepath.Join(path, "invalid.json")
	if err = r.Update(func(st core.Writer) error { st.PutRecord(core.Record{ID: "bad", TenantID: "tenant"}); return nil }); err == nil {
		t.Fatal("expected persistence failure")
	}
	_ = r.View(func(st core.Reader) error {
		if len(st.Records("tenant", "")) != 1 {
			t.Error("failed write published")
		}
		return nil
	})
}
