package repo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// A clone reads the data folder of another dolly and never writes to it, and it takes no lock, so a dolly can run on the source
// while it is cloned.

// sourceDir finds the folder with the seed: the path itself, or its graph folder (the path is a data root, as --dir is).
func sourceDir(path string) (string, error) {
	for _, d := range []string{path, filepath.Join(path, "graph")} {
		if _, err := os.Stat(filepath.Join(d, seedFile)); err == nil {
			return d, nil
		}
	}
	return "", fmt.Errorf("dolly: there is no %s in %q or in %q. Give a data folder", seedFile, path, filepath.Join(path, "graph"))
}

// OpenClone gives a world in memory with the data of the folder: the seed plus every change. It has no history of its own
// (a world in memory never has), and the folder is not changed.
func OpenClone(path string) (*World, error) {
	dir, err := sourceDir(path)
	if err != nil {
		return nil, err
	}
	baseline, err := os.ReadFile(filepath.Join(dir, seedFile))
	if err != nil {
		return nil, fmt.Errorf("dolly: reading %q: %w", dir, err)
	}
	events, err := readEvents(filepath.Join(dir, stateFile))
	if err != nil {
		return nil, err
	}
	w, err := replay(baseline, events, len(events))
	if err != nil {
		return nil, fmt.Errorf("dolly: replaying %q: %w", dir, err)
	}
	return w, nil
}

// CopyData writes the seed and the history of the source into the folder dst, which must have no data. Open opens it after that.
// The history is written again line by line, so a last line that is cut (the source is running) is left out, and the next
// change does not join it.
func CopyData(path, dst string) error {
	dir, err := sourceDir(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dst, seedFile)); err == nil {
		return fmt.Errorf("dolly: %q has data already. A clone never replaces data: use a folder that is empty", dst)
	}
	if a, b := mustAbs(dir), mustAbs(dst); a == b {
		return errors.New("dolly: a folder cannot be cloned onto itself")
	}
	baseline, err := os.ReadFile(filepath.Join(dir, seedFile))
	if err != nil {
		return fmt.Errorf("dolly: reading %q: %w", dir, err)
	}
	events, err := readEvents(filepath.Join(dir, stateFile))
	if err != nil {
		return err
	}
	var state bytes.Buffer
	enc := json.NewEncoder(&state) // one JSON value and a newline each
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("dolly: creating %q: %w", dst, err)
	}
	if err := os.WriteFile(filepath.Join(dst, stateFile), state.Bytes(), 0o644); err != nil {
		return fmt.Errorf("dolly: writing %q: %w", dst, err)
	}
	// the seed last: a folder with a seed is a folder with data
	if err := os.WriteFile(filepath.Join(dst, seedFile), baseline, 0o644); err != nil {
		return fmt.Errorf("dolly: writing %q: %w", dst, err)
	}
	return nil
}

func mustAbs(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
