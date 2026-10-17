package config

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func newTestStore(t *testing.T, path string) (*Store, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return NewStore(path, logger), &buf
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func loadedStore(t *testing.T, content []byte) (store *Store, path string, log *bytes.Buffer) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, content)
	store, log = newTestStore(t, path)
	if err := store.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return store, path, log
}

func TestStore_LoadValid(t *testing.T) {
	store, path, log := loadedStore(t, readFixture(t, "valid_five_tools.yaml"))

	if store.LoadError() != nil || store.ReadOnly() {
		t.Errorf("LoadError() = %v, ReadOnly() = %v", store.LoadError(), store.ReadOnly())
	}
	want := []string{"Relógio", "Snake", "Controle de curso", "Uso", "Git log"}
	if got := toolNames(store.Config().Tools); !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
	if store.Path() != path {
		t.Errorf("Path() = %q, want %q", store.Path(), path)
	}
	if !strings.Contains(log.String(), `msg="config loaded"`) || !strings.Contains(log.String(), "tools=5 missing=false reload=false") {
		t.Errorf("log = %q, want config loaded entry", log)
	}
}

func TestStore_ConfigIsACopy(t *testing.T) {
	store, _, _ := loadedStore(t, readFixture(t, "valid_five_tools.yaml"))

	cfg := store.Config()
	cfg.Tools[0].Name = "changed"

	if store.Config().Tools[0].Name != "Relógio" {
		t.Error("changing the returned config changed the store")
	}
}

func TestStore_LoadMissingCreatesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gopherbay")
	path := filepath.Join(dir, "config.yaml")
	store, log := newTestStore(t, path)

	if err := store.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := store.Config()
	if len(cfg.Tools) != 0 || cfg.Keys.Release != DefaultReleaseKey || store.ReadOnly() {
		t.Errorf("config = %+v, read-only %v, want empty valid config", cfg, store.ReadOnly())
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("folder exists after loading a missing file: %v", err)
	}
	if !strings.Contains(log.String(), "missing=true") {
		t.Errorf("log = %q, want missing=true", log)
	}
}

func TestStore_LoadSyntaxError(t *testing.T) {
	original := readFixture(t, "invalid_syntax.yaml")
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, original)
	store, log := newTestStore(t, path)

	err := store.Load()

	var e *Error
	if !errors.As(err, &e) || e.Kind != KindSyntax || e.Line != 12 || e.Path != path {
		t.Fatalf("Load() error = %v, want syntax error on line 12 with the path", err)
	}
	if !store.ReadOnly() || store.LoadError() != e || len(store.Config().Tools) != 0 {
		t.Errorf("ReadOnly() = %v, LoadError() = %v, tools = %d", store.ReadOnly(), store.LoadError(), len(store.Config().Tools))
	}
	if !bytes.Equal(readBytes(t, path), original) {
		t.Error("file bytes changed")
	}
	if !strings.Contains(log.String(), `level=WARN msg="config load failed"`) || !strings.Contains(log.String(), "kind=syntax line=12") {
		t.Errorf("log = %q, want config load failed entry", log)
	}
}

func TestStore_LoadValidationErrorClearsTools(t *testing.T) {
	store, path, _ := loadedStore(t, readFixture(t, "valid_five_tools.yaml"))
	writeFile(t, path, readFixture(t, "invalid_duplicate.yaml"))

	err := store.Load()

	if want := `Erro no config.yaml (linha 8): já existe uma ferramenta chamada "snake"`; err == nil || err.Error() != want {
		t.Errorf("Load() error = %v, want %q", err, want)
	}
	if len(store.Config().Tools) != 0 {
		t.Error("tools kept after a failed reload")
	}
}

func TestStore_LoadUnreadable(t *testing.T) {
	t.Run("folder", func(t *testing.T) {
		path := t.TempDir()
		store, _ := newTestStore(t, path)

		err := store.Load()

		var e *Error
		if !errors.As(err, &e) || e.Kind != KindRead {
			t.Fatalf("Load() error = %v, want read error", err)
		}
		if !strings.HasPrefix(e.Error(), "Não foi possível ler o config.yaml: ") {
			t.Errorf("Error() = %q", e.Error())
		}
		if !store.ReadOnly() {
			t.Error("ReadOnly() = false")
		}
	})
	t.Run("permission", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no POSIX permission bits to deny reading")
		}
		if os.Geteuid() == 0 {
			t.Skip("root reads files regardless of permissions")
		}
		path := filepath.Join(t.TempDir(), "config.yaml")
		writeFile(t, path, []byte("version: 1\n"))
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		store, _ := newTestStore(t, path)

		err := store.Load()

		var e *Error
		if !errors.As(err, &e) || e.Kind != KindRead || !errors.Is(err, os.ErrPermission) {
			t.Errorf("Load() error = %v, want read error wrapping permission denied", err)
		}
	})
	t.Run("too large", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		writeFile(t, path, bytes.Repeat([]byte("#"), MaxFileSize+1))
		store, _ := newTestStore(t, path)

		var e *Error
		if err := store.Load(); !errors.As(err, &e) || e.Rule != RuleFileTooLarge {
			t.Errorf("Load() error = %v, want file_too_large", err)
		}
	})
}

func TestStore_ReloadAfterFix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, readFixture(t, "invalid_syntax.yaml"))
	store, log := newTestStore(t, path)
	if err := store.Load(); err == nil {
		t.Fatal("first Load() succeeded, want error")
	}

	writeFile(t, path, readFixture(t, "valid_five_tools.yaml"))
	if err := store.Load(); err != nil {
		t.Fatalf("reload error = %v", err)
	}

	if store.ReadOnly() || len(store.Config().Tools) != 5 {
		t.Errorf("ReadOnly() = %v, tools = %d, want normal mode with 5 tools", store.ReadOnly(), len(store.Config().Tools))
	}
	if !strings.Contains(log.String(), "reload=true") {
		t.Errorf("log = %q, want reload=true", log)
	}
}

func TestStore_ReloadUpdatesError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, readFixture(t, "invalid_syntax.yaml"))
	store, _ := newTestStore(t, path)
	_ = store.Load()

	writeFile(t, path, readFixture(t, "invalid_duplicate.yaml"))
	_ = store.Load()

	e := store.LoadError()
	if e == nil || e.Line != 8 || e.Rule != RuleNameDuplicate {
		t.Errorf("LoadError() = %+v, want name_duplicate on line 8", e)
	}
}

func TestStore_ReloadUpdatesReleaseKey(t *testing.T) {
	store, path, _ := loadedStore(t, []byte("version: 1\n"))
	if got := store.Config().Keys.Release; got != `ctrl+\` {
		t.Fatalf("release = %q, want default", got)
	}

	writeFile(t, path, []byte("version: 1\nkeys:\n  release: \"ctrl+]\"\n"))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}

	if got := store.Config().Keys.Release; got != "ctrl+]" {
		t.Errorf("release = %q, want ctrl+]", got)
	}
}
