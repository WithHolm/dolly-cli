package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
)

// One dolly at a time uses a data folder. A lock file says which one. The
// file names the process and, when the server is listening, its address, so the
// message for a second dolly can say where to connect.

type lockInfo struct {
	PID int    `json:"pid"`
	URL string `json:"url,omitempty"`
}

// processAlive reports whether a process with this ID runs. Where it cannot tell
// (Windows), it says yes, so a lock is never taken from a dolly that runs.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}

// Running gives the address of the dolly that uses this data folder, when one
// runs and listens. The commands use it to find a dolly without being told
// where it is.
func Running(dir string) (url string, ok bool) {
	data, err := os.ReadFile(filepath.Join(dir, lockFile))
	if err != nil {
		return "", false
	}
	var li lockInfo
	if json.Unmarshal(data, &li) != nil || li.URL == "" || !processAlive(li.PID) {
		return "", false
	}
	return li.URL, true
}

func lock(dir string) error { return lockOnce(dir, true) }

func lockOnce(dir string, retry bool) error {
	path := filepath.Join(dir, lockFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, os.ErrExist) {
		var li lockInfo
		if data, rerr := os.ReadFile(path); rerr == nil {
			_ = json.Unmarshal(data, &li)
		}
		// A dolly that crashed leaves its lock. A process that is gone holds nothing. A lock that has our own
		// process ID, and that this process does not hold, is from an earlier run too: in a container, dolly is
		// process 1 every time, and the volume keeps the lock of a container that was killed.
		if retry && ((li.PID == os.Getpid() && !heldHere(dir)) || !processAlive(li.PID)) {
			slog.Info("Dolly took over a lock that no process holds.", "file", path, "pid", li.PID)
			if rmErr := os.Remove(path); rmErr == nil {
				return lockOnce(dir, false)
			}
		}
		msg := fmt.Sprintf("dolly: the folder %q is in use by another dolly (process %d).", dir, li.PID)
		if li.URL != "" {
			msg += fmt.Sprintf(" It listens at %s. Stop it, or point your tool at that address.", li.URL)
		}
		return fmt.Errorf("%s If no dolly runs, delete the file %s", msg, path)
	}
	if err != nil {
		return fmt.Errorf("dolly: locking %q: %w", dir, err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(lockInfo{PID: os.Getpid()}); err != nil {
		return err
	}
	markHeld(dir, true)
	return nil
}

func unlock(dir string) {
	markHeld(dir, false)
	_ = os.Remove(filepath.Join(dir, lockFile))
}

// held are the folders that this process has locked and not unlocked.
var held = struct {
	sync.Mutex
	dirs map[string]bool
}{dirs: map[string]bool{}}

func heldKey(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

func heldHere(dir string) bool {
	held.Lock()
	defer held.Unlock()
	return held.dirs[heldKey(dir)]
}

func markHeld(dir string, on bool) {
	held.Lock()
	defer held.Unlock()
	if on {
		held.dirs[heldKey(dir)] = true
	} else {
		delete(held.dirs, heldKey(dir))
	}
}

// SetAddress writes the address of the server in the lock file.
func (w *World) SetAddress(url string) {
	if w.j == nil {
		return
	}
	data, _ := json.Marshal(lockInfo{PID: os.Getpid(), URL: url})
	_ = os.WriteFile(filepath.Join(w.j.dir, lockFile), data, 0o644)
}
