package repo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLock(t *testing.T, dir string, li lockInfo) {
	t.Helper()
	b, _ := json.Marshal(li)
	if err := os.WriteFile(filepath.Join(dir, lockFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLock_TakesOverALockThatNoRunningDollyHolds(t *testing.T) {
	// the lock of a container that was killed: the same process ID as ours, because dolly is process 1 in a container
	dir := t.TempDir()
	writeLock(t, dir, lockInfo{PID: os.Getpid(), URL: "http://127.0.0.1:8650"})
	if err := lock(dir); err != nil {
		t.Fatalf("a lock with our own process ID is from an earlier run: %v", err)
	}
	unlock(dir)

	// the lock of a process that is gone
	writeLock(t, dir, lockInfo{PID: 2147483000})
	if err := lock(dir); err != nil {
		t.Fatalf("a process that is gone holds nothing: %v", err)
	}
	unlock(dir)
}

// A second open in the same process is a conflict, not an old lock: this process holds the first.
func TestLock_RefusesASecondLockInTheSameProcess(t *testing.T) {
	dir := t.TempDir()
	if err := lock(dir); err != nil {
		t.Fatal(err)
	}
	if err := lock(dir); err == nil || !strings.Contains(err.Error(), "in use by another dolly") {
		t.Fatalf("the process holds the lock, so a second one must be refused: %v", err)
	}
	unlock(dir) // after an unlock, the folder is free
	if err := lock(dir); err != nil {
		t.Fatalf("after unlock: %v", err)
	}
	unlock(dir)
}

func TestLock_RefusesALockOfAnotherRunningProcess(t *testing.T) {
	dir := t.TempDir()
	// process 1 runs on every Unix machine, and it is not us
	if os.Getpid() == 1 {
		t.Skip("this test runs as process 1")
	}
	writeLock(t, dir, lockInfo{PID: 1, URL: "http://127.0.0.1:8650"})
	err := lock(dir)
	if err == nil || !strings.Contains(err.Error(), "in use by another dolly (process 1)") || !strings.Contains(err.Error(), "http://127.0.0.1:8650") {
		t.Fatalf("a lock of a process that runs: %v", err)
	}
	// and a dolly that holds the lock holds it against a second one
	dir = t.TempDir()
	if err := lock(dir); err != nil {
		t.Fatal(err)
	}
	writeLock(t, dir, lockInfo{PID: 1})
	if err := lock(dir); err == nil {
		t.Error("the second dolly must be refused")
	}
}
