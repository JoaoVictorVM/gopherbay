package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPath_Default(t *testing.T) {
	dir := t.TempDir()

	got, err := ResolveConfigPath("", dir)
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}
	if want := filepath.Join(dir, "config.yaml"); got != want {
		t.Errorf("ResolveConfigPath() = %q, want %q", got, want)
	}
}

func TestResolveConfigPath_Absolute(t *testing.T) {
	override := filepath.Join(t.TempDir(), "demo", "..", "demo.yaml")

	got, err := ResolveConfigPath(override, "unused")
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}
	if want := filepath.Clean(override); got != want {
		t.Errorf("ResolveConfigPath() = %q, want %q", got, want)
	}
}

func TestResolveConfigPath_Relative(t *testing.T) {
	wd := t.TempDir()
	t.Chdir(wd)

	got, err := ResolveConfigPath(filepath.Join("testdata", "demo.yaml"), "unused")
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cwd, "testdata", "demo.yaml"); got != want {
		t.Errorf("ResolveConfigPath() = %q, want %q", got, want)
	}
}

func TestDefaultConfigDir(t *testing.T) {
	isolateConfigDir(t)

	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() error = %v", err)
	}
	got, err := DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir() error = %v", err)
	}
	if want := filepath.Join(base, "gopherbay"); got != want {
		t.Errorf("DefaultConfigDir() = %q, want %q", got, want)
	}
}

func isolateConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
}
