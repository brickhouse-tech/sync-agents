package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBackupPath_NeverReturnsExistingPath guards the never-delete
// guarantee: os.Rename onto an existing file silently replaces it, so
// backupPath must hand back a free name even when the plain and the
// timestamped backup names are both already taken.
func TestBackupPath_NeverReturnsExistingPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.md")

	first := backupPath(target)
	if first != target+BackupSuffix {
		t.Fatalf("first backup = %q, want %q", first, target+BackupSuffix)
	}

	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		b := backupPath(target)
		if _, err := os.Lstat(b); err == nil {
			t.Fatalf("iteration %d: backupPath returned existing path %q", i, b)
		}
		if seen[b] {
			t.Fatalf("iteration %d: backupPath repeated %q", i, b)
		}
		seen[b] = true
		if err := os.WriteFile(b, []byte("occupied"), 0o644); err != nil {
			t.Fatalf("occupy %q: %v", b, err)
		}
	}
}
