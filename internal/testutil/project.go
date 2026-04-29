package testutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var blockedRoots = []string{
	filepath.Clean("/home/user/Projects/av1/av1go"),
	filepath.Clean("/home/user/go/src/github.com/codemodify/av1go-claude"),
}

// ModuleRoot returns the repository root by walking upward until go.mod is found.
func ModuleRoot(tb testing.TB) string {
	tb.Helper()

	wd, err := os.Getwd()
	if err != nil {
		tb.Skipf("unable to determine working directory: %v", err)
		return ""
	}

	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Skip("go.mod not found in parent directories")
			return ""
		}
		dir = parent
	}
}

// SamplePath resolves a file under testvideo relative to the repository root.
func SamplePath(tb testing.TB, name string) string {
	tb.Helper()
	return filepath.Join(ModuleRoot(tb), "testvideo", name)
}

// AvailableSamplePaths returns usable local MP4 sample paths from testvideo/.
func AvailableSamplePaths(tb testing.TB) []string {
	tb.Helper()

	paths, err := filepath.Glob(filepath.Join(ModuleRoot(tb), "testvideo", "*.mp4"))
	if err != nil {
		tb.Fatalf("Glob(testvideo): %v", err)
	}
	sort.Strings(paths)

	available := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := FileAvailable(path); err == nil {
			available = append(available, path)
		}
	}
	return available
}

// FileAvailable reports whether the given path can be read locally without
// resolving into one of the blocked external sample roots.
func FileAvailable(path string) error {
	if blocked, root := pathBlocked(path); blocked {
		return fmt.Errorf("sample unavailable by policy: %s resolves under %s", path, root)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("sample unavailable: %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return fmt.Errorf("sample unavailable: %s: %w", path, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		if blocked, root := pathBlocked(target); blocked {
			return fmt.Errorf("sample unavailable by policy: %s resolves under %s", path, root)
		}
	}
	info, err = os.Stat(path)
	if err != nil {
		return fmt.Errorf("sample unavailable: %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("sample unavailable: %s: is a directory", path)
	}
	return nil
}

// RequireFile skips the test if the given path does not exist or is not readable.
func RequireFile(tb testing.TB, path string) {
	tb.Helper()
	if err := FileAvailable(path); err != nil {
		tb.Skip(err.Error())
	}
}

// RequireTool skips the test if the command is not installed.
func RequireTool(tb testing.TB, name string) string {
	tb.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		tb.Skipf("required tool %q not found: %v", name, err)
	}
	return path
}

// CommandError wraps a tool execution failure with captured output.
func CommandError(name string, args []string, err error, output []byte) error {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return fmt.Errorf("%s %v failed: %w", name, args, err)
	}
	return fmt.Errorf("%s %v failed: %w\n%s", name, args, err, trimmed)
}

// IsMissingFile reports whether err indicates a missing file or path.
func IsMissingFile(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

func pathBlocked(path string) (bool, string) {
	clean := filepath.Clean(path)
	for _, root := range blockedRoots {
		if clean == root || strings.HasPrefix(clean, root+string(os.PathSeparator)) {
			return true, root
		}
	}
	return false, ""
}
