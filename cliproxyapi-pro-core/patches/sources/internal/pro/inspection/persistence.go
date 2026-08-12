package inspection

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AtomicWriteFile preserves the previous valid file until the replacement is
// fully written and synced in the same directory.
func AtomicWriteFile(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err = temp.Chmod(mode); err != nil {
		return err
	}
	if _, err = temp.Write(data); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tempPath, path); err != nil {
		return err
	}
	// Windows does not provide a portable way to fsync a directory handle.
	// The file itself has already been synced and atomically replaced above.
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, openErr := os.Open(dir)
	if openErr != nil {
		return fmt.Errorf("open persistence directory: %w", openErr)
	}
	defer directory.Close()
	if syncErr := directory.Sync(); syncErr != nil {
		return fmt.Errorf("sync persistence directory: %w", syncErr)
	}
	return nil
}
