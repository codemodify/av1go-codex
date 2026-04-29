package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileAvailableAcceptsLocalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(path, []byte("not-a-real-mp4"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := FileAvailable(path); err != nil {
		t.Fatalf("FileAvailable: %v", err)
	}
}

func TestFileAvailableRejectsBlockedSymlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.mp4")
	target := filepath.Join(blockedRoots[0], "testvideo", "sample.mp4")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("Symlink: %v", err)
	}

	err := FileAvailable(path)
	if err == nil {
		t.Fatal("expected blocked symlink error")
	}
	if !strings.Contains(err.Error(), "policy") {
		t.Fatalf("FileAvailable error = %v, want policy rejection", err)
	}
}
