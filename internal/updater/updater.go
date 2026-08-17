package updater

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/3xiaoxiao3/mihomo-stack/internal/state"
)

var (
	ErrBusy           = errors.New("another configuration transaction is running")
	ErrBackupNotFound = errors.New("backup not found")
)

type Builder interface {
	Build(context.Context) ([]byte, error)
}

type Validator interface {
	Validate(context.Context, string) error
}

type Controller interface {
	Reload(context.Context, string) error
	Health(context.Context) error
}

type Recorder interface {
	Append(state.Record) error
}

type Config struct {
	ActiveConfig    string
	BackupDir       string
	BackupRetention int
	HealthDelay     time.Duration
	RollbackTimeout time.Duration
}

type Backup struct {
	ID        string    `json:"id"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	config      Config
	builder     Builder
	validator   Validator
	controller  Controller
	recorder    Recorder
	transaction sync.Mutex
	busy        atomic.Bool
	now         func() time.Time
}

func New(config Config, builder Builder, validator Validator, controller Controller, recorder Recorder) *Service {
	return &Service{
		config:     config,
		builder:    builder,
		validator:  validator,
		controller: controller,
		recorder:   recorder,
		now:        time.Now,
	}
}

func (s *Service) Busy() bool {
	return s.busy.Load()
}

func (s *Service) Update(ctx context.Context, trigger string) (record state.Record, resultErr error) {
	if !s.transaction.TryLock() {
		return state.Record{}, ErrBusy
	}
	s.busy.Store(true)
	defer func() {
		s.busy.Store(false)
		s.transaction.Unlock()
	}()

	record = state.Record{
		ID:        newID(s.now()),
		Trigger:   trigger,
		Stage:     "build",
		StartedAt: s.now().UTC(),
	}
	defer func() {
		record.FinishedAt = s.now().UTC()
		if resultErr != nil {
			record.Success = false
			record.Message = resultErr.Error()
		} else {
			record.Success = true
			record.Stage = "complete"
		}
		if err := s.recorder.Append(record); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("persist update history: %w", err)
			record.Success = false
			record.Message = resultErr.Error()
		}
	}()

	if err := s.ensureDirectories(); err != nil {
		return record, err
	}
	defer func() {
		if err := s.pruneBackups(); err != nil && resultErr == nil {
			resultErr = err
		}
	}()
	candidate, err := s.builder.Build(ctx)
	if err != nil {
		return record, fmt.Errorf("build candidate: %w", err)
	}
	candidatePath, err := s.writeCandidate(candidate)
	if err != nil {
		return record, err
	}
	defer os.Remove(candidatePath)

	record.Stage = "validate"
	if err := s.validator.Validate(ctx, candidatePath); err != nil {
		return record, err
	}

	record.Stage = "backup"
	backupPath, _, hadActive, err := s.backupActive()
	if err != nil {
		return record, err
	}

	record.Stage = "activate"
	activated, err := activateCandidate(candidatePath, s.config.ActiveConfig)
	if err != nil {
		if activated {
			record.Stage = "rollback"
			rolledBack, rollbackErr := s.restoreAfterFailure(ctx, backupPath, hadActive)
			record.RolledBack = rolledBack
			if rollbackErr != nil {
				return record, errors.Join(err, fmt.Errorf("automatic rollback failed: %w", rollbackErr))
			}
		}
		return record, err
	}

	record.Stage = "reload"
	if err := s.reloadAndCheck(ctx); err != nil {
		record.Stage = "rollback"
		rolledBack, rollbackErr := s.restoreAfterFailure(ctx, backupPath, hadActive)
		record.RolledBack = rolledBack
		if rollbackErr != nil {
			return record, errors.Join(err, fmt.Errorf("automatic rollback failed: %w", rollbackErr))
		}
		return record, err
	}
	return record, nil
}

func (s *Service) Restore(ctx context.Context, backupID, trigger string) (record state.Record, resultErr error) {
	if !validBackupID(backupID) {
		return state.Record{}, ErrBackupNotFound
	}
	if !s.transaction.TryLock() {
		return state.Record{}, ErrBusy
	}
	s.busy.Store(true)
	defer func() {
		s.busy.Store(false)
		s.transaction.Unlock()
	}()

	record = state.Record{
		ID:        newID(s.now()),
		Trigger:   trigger,
		Stage:     "validate-backup",
		StartedAt: s.now().UTC(),
	}
	defer func() {
		record.FinishedAt = s.now().UTC()
		if resultErr != nil {
			record.Message = resultErr.Error()
		} else {
			record.Success = true
			record.Stage = "complete"
		}
		if err := s.recorder.Append(record); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("persist rollback history: %w", err)
			record.Success = false
			record.Message = resultErr.Error()
		}
	}()
	defer func() {
		if err := s.pruneBackups(); err != nil && resultErr == nil {
			resultErr = err
		}
	}()

	backupPath := filepath.Join(s.config.BackupDir, backupID)
	if info, err := os.Stat(backupPath); err != nil || !info.Mode().IsRegular() {
		return record, ErrBackupNotFound
	}
	if err := s.validator.Validate(ctx, backupPath); err != nil {
		return record, fmt.Errorf("validate backup: %w", err)
	}

	record.Stage = "backup-current"
	preRestorePath, _, hadActive, err := s.backupActive()
	if err != nil {
		return record, err
	}
	record.Stage = "activate-backup"
	if err := copyAtomic(backupPath, s.config.ActiveConfig); err != nil {
		return record, fmt.Errorf("activate backup: %w", err)
	}
	record.Stage = "reload"
	if err := s.reloadAndCheck(ctx); err != nil {
		record.Stage = "rollback"
		rolledBack, rollbackErr := s.restoreAfterFailure(ctx, preRestorePath, hadActive)
		record.RolledBack = rolledBack
		if rollbackErr != nil {
			return record, errors.Join(err, fmt.Errorf("restore previous active config failed: %w", rollbackErr))
		}
		return record, err
	}
	return record, nil
}

func (s *Service) ListBackups() ([]Backup, error) {
	entries, err := os.ReadDir(s.config.BackupDir)
	if errors.Is(err, os.ErrNotExist) {
		return []Backup{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	backups := make([]Backup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !validBackupID(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("read backup metadata: %w", err)
		}
		backups = append(backups, Backup{
			ID:        entry.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt.After(backups[j].CreatedAt) })
	return backups, nil
}

func (s *Service) ensureDirectories() error {
	for _, directory := range []string{filepath.Dir(s.config.ActiveConfig), s.config.BackupDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("create data directory: %w", err)
		}
	}
	return nil
}

func (s *Service) writeCandidate(content []byte) (string, error) {
	directory := filepath.Dir(s.config.ActiveConfig)
	file, err := os.CreateTemp(directory, ".candidate-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create candidate: %w", err)
	}
	path := file.Name()
	failed := true
	defer func() {
		if failed {
			file.Close()
			os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", fmt.Errorf("secure candidate: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		return "", fmt.Errorf("write candidate: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync candidate: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close candidate: %w", err)
	}
	failed = false
	return path, nil
}

func (s *Service) backupActive() (path, id string, existed bool, err error) {
	info, statErr := os.Stat(s.config.ActiveConfig)
	if errors.Is(statErr, os.ErrNotExist) {
		return "", "", false, nil
	}
	if statErr != nil {
		return "", "", false, fmt.Errorf("inspect active config: %w", statErr)
	}
	if !info.Mode().IsRegular() {
		return "", "", false, errors.New("active config is not a regular file")
	}
	id = newID(s.now()) + ".yaml"
	path = filepath.Join(s.config.BackupDir, id)
	if err := copyAtomic(s.config.ActiveConfig, path); err != nil {
		return "", "", false, fmt.Errorf("back up active config: %w", err)
	}
	return path, id, true, nil
}

func (s *Service) reloadAndCheck(ctx context.Context) error {
	if err := s.controller.Reload(ctx, s.config.ActiveConfig); err != nil {
		return err
	}
	if s.config.HealthDelay > 0 {
		timer := time.NewTimer(s.config.HealthDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	if err := s.controller.Health(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) restoreAfterFailure(_ context.Context, backupPath string, hadActive bool) (bool, error) {
	if !hadActive {
		if err := os.Remove(s.config.ActiveConfig); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("remove failed first config: %w", err)
		}
		return true, nil
	}
	if err := copyAtomic(backupPath, s.config.ActiveConfig); err != nil {
		return false, err
	}
	timeout := s.config.RollbackTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	rollbackContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.reloadAndCheck(rollbackContext); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) pruneBackups() error {
	backups, err := s.ListBackups()
	if err != nil {
		return err
	}
	if len(backups) <= s.config.BackupRetention {
		return nil
	}
	for _, backup := range backups[s.config.BackupRetention:] {
		if err := os.Remove(filepath.Join(s.config.BackupDir, backup.ID)); err != nil {
			return fmt.Errorf("prune backup: %w", err)
		}
	}
	return nil
}

func activateCandidate(candidatePath, activePath string) (bool, error) {
	if err := os.Rename(candidatePath, activePath); err != nil {
		return false, fmt.Errorf("activate candidate: %w", err)
	}
	if err := os.Chmod(activePath, 0o600); err != nil {
		return true, fmt.Errorf("secure active config: %w", err)
	}
	if err := syncDirectory(filepath.Dir(activePath)); err != nil {
		return true, err
	}
	return true, nil
}

func copyAtomic(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destinationPath), ".copy-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, source); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(destinationPath))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func validBackupID(id string) bool {
	return id != "" && filepath.Base(id) == id && strings.HasSuffix(id, ".yaml") && !strings.Contains(id, "..")
}

func newID(now time.Time) string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return now.UTC().Format("20060102T150405.000000000Z")
	}
	return now.UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(random)
}
