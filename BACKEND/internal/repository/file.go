package repository

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sync"

	"businessautomation/internal/core"
)

// File is a single-process repository. Updates publish only after an atomic file replacement.
type File struct {
	mu    sync.RWMutex
	path  string
	state databaseState
}

func Open(path string) (*File, error) {
	return openFile(path, true)
}

func openFile(path string, allowMissing bool) (*File, error) {
	repository := &File{
		path: path,
		state: databaseState{
			Users:    make(map[string]core.User),
			Sessions: make(map[string]core.Session),
			Records:  make(map[string]core.Record),
		},
	}
	contents, err := os.ReadFile(path)
	if allowMissing && errors.Is(err, os.ErrNotExist) {
		return repository, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(contents, &repository.state); err != nil {
		return nil, err
	}
	if repository.state.Users == nil || repository.state.Sessions == nil || repository.state.Records == nil {
		return nil, errors.New("invalid database structure")
	}
	for id, record := range repository.state.Records {
		if core.VersionedResource(record.Kind) && record.Version == 0 {
			record.Version = 1
			repository.state.Records[id] = record
		}
	}
	return repository, nil
}

// Published maps are immutable. The adapter detaches each record at its boundary.
func clone(state databaseState) databaseState {
	copiedState := databaseState{
		Users:    maps.Clone(state.Users),
		Sessions: maps.Clone(state.Sessions),
		Records:  maps.Clone(state.Records),
	}
	return copiedState
}

func (repository *File) View(read func(core.Reader) error) error {
	repository.mu.RLock()
	published := repository.state
	repository.mu.RUnlock()
	return read(struct{ core.Reader }{snapshot{state: &published}})
}

func (repository *File) Update(update func(core.Writer) error) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	nextState := clone(repository.state)
	if err := update(snapshot{state: &nextState}); err != nil {
		return err
	}
	if err := repository.persist(nextState); err != nil {
		return err
	}
	repository.state = nextState
	return nil
}

func (repository *File) ViewTenant(tenantID string, run func(core.Reader) error) error {
	if tenantID == "" {
		return errTenantScope
	}
	return repository.View(func(store core.Reader) error { return withTenantReader(store, tenantID, run) })
}

func (repository *File) UpdateTenant(tenantID string, run func(core.Writer) error) error {
	if tenantID == "" {
		return errTenantScope
	}
	return repository.Update(func(store core.Writer) error {
		return withTenantWriter(store, tenantID, run)
	})
}

func (repository *File) persist(state databaseState) error {
	contents, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(repository.path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	temporaryFile, err := os.CreateTemp(directory, "state-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporaryFile.Name())
	if err := writeSnapshot(temporaryFile, contents); err != nil {
		return err
	}
	return os.Rename(temporaryFile.Name(), repository.path)
}

func writeSnapshot(file *os.File, contents []byte) (err error) {
	// Close on every path, but preserve the original write/sync error when one exists.
	defer func() {
		closeError := file.Close()
		if err == nil {
			err = closeError
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		return err
	}
	return file.Sync()
}
