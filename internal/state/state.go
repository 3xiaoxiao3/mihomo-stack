package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Record struct {
	ID         string    `json:"id"`
	Trigger    string    `json:"trigger"`
	Stage      string    `json:"stage"`
	Success    bool      `json:"success"`
	RolledBack bool      `json:"rolled_back"`
	Message    string    `json:"message,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type Data struct {
	History []Record `json:"history"`
}

type Store struct {
	mu        sync.RWMutex
	path      string
	retention int
	data      Data
}

func Open(path string, retention int) (*Store, error) {
	store := &Store{path: path, retention: retention, data: Data{History: []Record{}}}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(content, &store.data); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	if store.data.History == nil {
		store.data.History = []Record{}
	}
	if len(store.data.History) > retention {
		store.data.History = store.data.History[len(store.data.History)-retention:]
	}
	return store, nil
}

func (s *Store) Append(record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.History = append(s.data.History, record)
	if len(s.data.History) > s.retention {
		s.data.History = s.data.History[len(s.data.History)-s.retention:]
	}
	return s.persistLocked()
}

func (s *Store) History() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Record, len(s.data.History))
	for i := range s.data.History {
		result[len(result)-1-i] = s.data.History[i]
	}
	return result
}

func (s *Store) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	content, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create state temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure state temporary file: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("activate state: %w", err)
	}
	return syncDirectory(filepath.Dir(s.path))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open state directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}
	return nil
}
