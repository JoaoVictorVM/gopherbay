package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openLog(t *testing.T, dir string, level slog.Level) *File {
	t.Helper()
	l, err := Open(dir, level)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func TestOpen_CreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "gopherbay")

	l := openLog(t, dir, slog.LevelInfo)

	if l.RotateErr != nil {
		t.Errorf("RotateErr = %v, want nil", l.RotateErr)
	}
	if l.Path != filepath.Join(dir, FileName) {
		t.Errorf("Path = %q, want %q", l.Path, filepath.Join(dir, FileName))
	}
	if _, err := os.Stat(l.Path); err != nil {
		t.Errorf("log file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, OldFileName)); !os.IsNotExist(err) {
		t.Errorf("old log should not exist on first start, Stat error = %v", err)
	}
}

func TestOpen_RotatesPreviousLog(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, FileName), "previous run\n")

	l := openLog(t, dir, slog.LevelInfo)
	l.Logger.Info("new run")
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if got := readFile(t, filepath.Join(dir, OldFileName)); got != "previous run\n" {
		t.Errorf("old log = %q, want previous content", got)
	}
	got := readFile(t, filepath.Join(dir, FileName))
	if strings.Contains(got, "previous run") || !strings.Contains(got, "new run") {
		t.Errorf("log = %q, want only the new entries", got)
	}
}

func TestOpen_ReplacesOlderBackup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, OldFileName), "two runs ago\n")
	writeFile(t, filepath.Join(dir, FileName), "previous run\n")

	openLog(t, dir, slog.LevelInfo)

	if got := readFile(t, filepath.Join(dir, OldFileName)); got != "previous run\n" {
		t.Errorf("old log = %q, want the previous run", got)
	}
}

func TestOpen_InfoLevelOmitsDebug(t *testing.T) {
	dir := t.TempDir()
	l := openLog(t, dir, slog.LevelInfo)

	l.Logger.Debug("debug entry")
	l.Logger.Info("info entry")

	got := readFile(t, l.Path)
	if strings.Contains(got, "debug entry") {
		t.Errorf("log = %q, debug entry should be omitted", got)
	}
	if !strings.Contains(got, "info entry") {
		t.Errorf("log = %q, want info entry", got)
	}
}

func TestOpen_DebugLevelIncludesDebug(t *testing.T) {
	dir := t.TempDir()
	l := openLog(t, dir, slog.LevelDebug)

	l.Logger.Debug("debug entry")

	if got := readFile(t, l.Path); !strings.Contains(got, "level=DEBUG msg=\"debug entry\"") {
		t.Errorf("log = %q, want debug entry", got)
	}
}

func TestOpen_FailsWhenDirIsFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gopherbay")
	writeFile(t, dir, "not a directory")

	l, err := Open(dir, slog.LevelInfo)
	if err == nil {
		_ = l.Close()
		t.Fatal("Open() error = nil, want error")
	}
}
