package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WriteJSONAtomic encodes v as indented JSON to path via a temp file in the same
// directory followed by an atomic rename, so a crash mid-write cannot corrupt
// the destination and a concurrent reader never observes a partially written
// file. The parent directory is created if needed.
//
// It is also crash-durable: the temp file's contents are fsynced before the
// rename (so the rename can never expose an unflushed/empty file), and the
// parent directory is fsynced afterward on a best-effort basis so the rename
// itself survives power loss (directory fsync is not supported on every
// platform, so its failure is ignored).
func WriteJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(v); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}
