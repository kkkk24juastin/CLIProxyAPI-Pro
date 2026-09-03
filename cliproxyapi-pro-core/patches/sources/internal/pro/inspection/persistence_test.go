package inspection

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFileReplacesContentAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	if err := AtomicWriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "second" {
		t.Fatalf("content = %q", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}
