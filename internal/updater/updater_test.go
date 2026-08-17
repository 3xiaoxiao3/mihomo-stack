package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/3xiaoxiao3/mihomo-stack/internal/state"
)

type fakeBuilder struct {
	content []byte
	block   chan struct{}
}

func (b *fakeBuilder) Build(context.Context) ([]byte, error) {
	if b.block != nil {
		<-b.block
	}
	return b.content, nil
}

type fakeValidator struct {
	err error
}

func (v fakeValidator) Validate(context.Context, string) error { return v.err }

type fakeController struct {
	mu          sync.Mutex
	healthCalls int
	failHealth  int
}

type contextController struct {
	reloadCalls int
}

func (c *contextController) Reload(ctx context.Context, _ string) error {
	c.reloadCalls++
	if c.reloadCalls == 1 {
		return ctx.Err()
	}
	return ctx.Err()
}

func (c *contextController) Health(ctx context.Context) error { return ctx.Err() }

func (c *fakeController) Reload(context.Context, string) error { return nil }

func (c *fakeController) Health(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthCalls++
	if c.healthCalls <= c.failHealth {
		return errors.New("unhealthy")
	}
	return nil
}

type memoryRecorder struct {
	records []state.Record
}

func (r *memoryRecorder) Append(record state.Record) error {
	r.records = append(r.records, record)
	return nil
}

func TestUpdateActivatesValidatedCandidate(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(active, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &memoryRecorder{}
	service := New(Config{
		ActiveConfig:    active,
		BackupDir:       filepath.Join(dir, "backups"),
		BackupRetention: 3,
	}, &fakeBuilder{content: []byte("new")}, fakeValidator{}, &fakeController{}, recorder)

	record, err := service.Update(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if !record.Success {
		t.Fatalf("record = %#v", record)
	}
	content, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Fatalf("active content = %q", content)
	}
	backups, err := service.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %#v, err = %v", backups, err)
	}
}

func TestUpdateRollsBackWhenHealthFails(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(active, []byte("known-good"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller := &fakeController{failHealth: 1}
	recorder := &memoryRecorder{}
	service := New(Config{
		ActiveConfig:    active,
		BackupDir:       filepath.Join(dir, "backups"),
		BackupRetention: 3,
	}, &fakeBuilder{content: []byte("bad-runtime")}, fakeValidator{}, controller, recorder)

	record, err := service.Update(context.Background(), "test")
	if err == nil {
		t.Fatal("expected health error")
	}
	if !record.RolledBack {
		t.Fatalf("record = %#v", record)
	}
	content, readErr := os.ReadFile(active)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "known-good" {
		t.Fatalf("active content = %q", content)
	}
}

func TestCanceledCallerCannotPreventRollback(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(active, []byte("known-good"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller := &contextController{}
	service := New(Config{
		ActiveConfig:    active,
		BackupDir:       filepath.Join(dir, "backups"),
		BackupRetention: 3,
		RollbackTimeout: time.Second,
	}, &fakeBuilder{content: []byte("candidate")}, fakeValidator{}, controller, &memoryRecorder{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	record, err := service.Update(ctx, "test")
	if err == nil || !record.RolledBack {
		t.Fatalf("record = %#v, err = %v", record, err)
	}
	content, readErr := os.ReadFile(active)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "known-good" {
		t.Fatalf("active content = %q", content)
	}
	if controller.reloadCalls != 2 {
		t.Fatalf("reload calls = %d", controller.reloadCalls)
	}
}

func TestSuccessfulUpdatesPruneOldBackups(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(active, []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := New(Config{
		ActiveConfig:    active,
		BackupDir:       filepath.Join(dir, "backups"),
		BackupRetention: 2,
	}, &fakeBuilder{content: []byte("next")}, fakeValidator{}, &fakeController{}, &memoryRecorder{})
	for i := 0; i < 4; i++ {
		if _, err := service.Update(context.Background(), "test"); err != nil {
			t.Fatal(err)
		}
	}
	backups, err := service.ListBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 2 {
		t.Fatalf("backup count = %d", len(backups))
	}
}

func TestConcurrentUpdateReturnsBusy(t *testing.T) {
	dir := t.TempDir()
	block := make(chan struct{})
	service := New(Config{
		ActiveConfig:    filepath.Join(dir, "config.yaml"),
		BackupDir:       filepath.Join(dir, "backups"),
		BackupRetention: 3,
	}, &fakeBuilder{content: []byte("new"), block: block}, fakeValidator{}, &fakeController{}, &memoryRecorder{})

	done := make(chan struct{})
	go func() {
		_, _ = service.Update(context.Background(), "first")
		close(done)
	}()
	for !service.Busy() {
	}
	if _, err := service.Update(context.Background(), "second"); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}
	close(block)
	<-done
}

func TestRestoreRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	service := New(Config{
		ActiveConfig:    filepath.Join(dir, "config.yaml"),
		BackupDir:       filepath.Join(dir, "backups"),
		BackupRetention: 3,
	}, &fakeBuilder{}, fakeValidator{}, &fakeController{}, &memoryRecorder{})
	if _, err := service.Restore(context.Background(), "../secret.yaml", "test"); !errors.Is(err, ErrBackupNotFound) {
		t.Fatalf("expected ErrBackupNotFound, got %v", err)
	}
}
