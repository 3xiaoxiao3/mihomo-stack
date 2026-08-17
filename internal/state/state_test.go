package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsAndPrunesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := Open(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two", "three"} {
		if err := store.Append(Record{ID: id, StartedAt: time.Now(), FinishedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	reopened, err := Open(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	history := reopened.History()
	if len(history) != 2 || history[0].ID != "three" || history[1].ID != "two" {
		t.Fatalf("history = %#v", history)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o", info.Mode().Perm())
	}
}

func TestOpenRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, 10); err == nil {
		t.Fatal("expected corrupt-state error")
	}
}
