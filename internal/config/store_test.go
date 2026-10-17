package config

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
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

func sampleTools() []Tool {
	return []Tool{
		{Name: "Relógio", Command: "clock-tty --24h", Description: "Relógio no terminal", Dir: Dir{Mode: ModeCurrent}},
		{Name: "Curso", Command: "curso", Dir: Dir{Mode: ModeFixed, Path: `D:\projetos\curso`}},
		{Name: "Perguntar", Command: "x", Dir: Dir{Mode: ModeAsk}},
	}
}

func tempFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			out = append(out, e.Name())
		}
	}
	return out
}

func setBeforeRename(t *testing.T, fn func(string) error) {
	t.Helper()
	beforeRename = fn
	t.Cleanup(func() { beforeRename = nil })
}

func TestStore_SaveRefusedWhileReadOnly(t *testing.T) {
	original := readFixture(t, "invalid_syntax.yaml")
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, original)
	store, log := newTestStore(t, path)
	_ = store.Load()

	err := store.Save(sampleTools())

	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("Save() error = %v, want ErrReadOnly", err)
	}
	if !bytes.Equal(readBytes(t, path), original) {
		t.Error("file bytes changed")
	}
	if !strings.Contains(log.String(), `msg="config save aborted"`) || !strings.Contains(log.String(), "reason=read_only") {
		t.Errorf("log = %q, want save aborted read_only", log)
	}
}

func TestStore_SavesToLoadedPath(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "other", "demo.yaml")
	writeFile(t, path, []byte("version: 1\n"))
	store, _ := newTestStore(t, path)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}

	if err := store.Save(sampleTools()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	cfg, err := Parse(readBytes(t, path), path)
	if err != nil || len(cfg.Tools) != 3 {
		t.Fatalf("saved file = %+v, %v", cfg, err)
	}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
		if d != nil && d.Name() == "config.yaml" {
			t.Errorf("unexpected %s", p)
		}
		return nil
	})
}

func TestStore_SaveCreatesFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "config.yaml")
	store, log := newTestStore(t, path)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}

	if err := store.Save(sampleTools()); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := Parse(readBytes(t, path), path); err != nil {
		t.Errorf("saved file does not parse: %v", err)
	}
	if !strings.Contains(log.String(), `msg="config saved"`) || !strings.Contains(log.String(), "tools=3") {
		t.Errorf("log = %q, want config saved entry", log)
	}
}

func TestStore_SaveThenReloadRoundTrip(t *testing.T) {
	store, path, _ := loadedStore(t, readFixture(t, "valid_five_tools.yaml"))
	tools := store.Config().Tools
	tools[1].Command = "snake --fast"
	tools[1].Description = "Jogo da cobrinha"

	if err := store.Save(tools); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	other, _ := newTestStore(t, path)
	if err := other.Load(); err != nil {
		t.Fatal(err)
	}
	got := other.Config()
	if !slices.Equal(got.Tools, tools) || got.Keys.Release != `ctrl+\` {
		t.Errorf("reloaded = %+v, want %+v", got.Tools, tools)
	}
	if !slices.Equal(store.Config().Tools, tools) {
		t.Error("in-memory config differs from the saved list")
	}
}

func TestStore_SavePreservesOrder(t *testing.T) {
	store, path, _ := loadedStore(t, readFixture(t, "valid_five_tools.yaml"))
	tools := store.Config().Tools
	tools[0], tools[1] = tools[1], tools[0]

	if err := store.Save(tools); err != nil {
		t.Fatal(err)
	}

	cfg, err := Parse(readBytes(t, path), path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Snake", "Relógio", "Controle de curso", "Uso", "Git log"}
	if got := toolNames(cfg.Tools); !slices.Equal(got, want) {
		t.Errorf("file order = %q, want %q", got, want)
	}
}

func TestStore_SaveInvalidRejected(t *testing.T) {
	original := readFixture(t, "valid_five_tools.yaml")
	store, path, _ := loadedStore(t, original)

	err := store.Save([]Tool{{Name: "Snake", Command: "a"}, {Name: "snake", Command: "b"}})

	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Violations) != 1 || ve.Violations[0].Rule != RuleNameDuplicate {
		t.Fatalf("Save() error = %v, want ValidationError with name_duplicate", err)
	}
	if !bytes.Equal(readBytes(t, path), original) {
		t.Error("file bytes changed")
	}
}

func TestStore_SaveTrimsValues(t *testing.T) {
	store, _, _ := loadedStore(t, []byte("version: 1\n"))

	if err := store.Save([]Tool{{Name: "  a  ", Command: " b "}}); err != nil {
		t.Fatal(err)
	}

	if got := store.Config().Tools[0]; got.Name != "a" || got.Command != "b" || got.Dir.Mode != ModeCurrent {
		t.Errorf("stored tool = %+v, want trimmed with mode current", got)
	}
}

func TestStore_SaveFailureKeepsOriginal(t *testing.T) {
	original := readFixture(t, "valid_five_tools.yaml")
	store, path, log := loadedStore(t, original)
	before := store.Config()
	setBeforeRename(t, func(string) error { return errors.New("disco cheio") })

	err := store.Save(sampleTools())

	var se *SaveError
	if !errors.As(err, &se) {
		t.Fatalf("Save() error = %v, want *SaveError", err)
	}
	if want := "Não foi possível salvar o config.yaml: renomear: disco cheio"; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	if !bytes.Equal(readBytes(t, path), original) {
		t.Error("original bytes changed")
	}
	if left := tempFiles(t, filepath.Dir(path)); len(left) != 0 {
		t.Errorf("temporary files left: %q", left)
	}
	if !slices.Equal(store.Config().Tools, before.Tools) {
		t.Error("in-memory config changed after a failed save")
	}
	if !strings.Contains(log.String(), `level=ERROR msg="config save failed"`) {
		t.Errorf("log = %q, want config save failed", log)
	}
}

const crashEnv = "GBAY_CONFIG_CRASH_PATH"

// TestStore_CrashHelper is the child process of TestStore_SaveCrashBeforeRename.
func TestStore_CrashHelper(t *testing.T) {
	path := os.Getenv(crashEnv)
	if path == "" {
		t.Skip("runs only as the child of TestStore_SaveCrashBeforeRename")
	}
	store := NewStore(path, slog.New(slog.DiscardHandler))
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	beforeRename = func(string) error {
		os.Exit(3)
		return nil
	}
	_ = store.Save(sampleTools())
	t.Fatal("Save() returned, want the process to exit before the rename")
}

func TestStore_SaveCrashBeforeRename(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a child process")
	}
	original := readFixture(t, "valid_five_tools.yaml")
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, path, original)

	cmd := exec.Command(os.Args[0], "-test.run=^TestStore_CrashHelper$", "-test.count=1") //nolint:gosec // G702: re-runs this same test binary

	cmd.Env = append(os.Environ(), crashEnv+"="+path)
	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("child error = %v, want exit code 3; output:\n%s", err, out)
	}
	if !bytes.Equal(readBytes(t, path), original) {
		t.Error("original bytes changed by the interrupted save")
	}
	if left := tempFiles(t, filepath.Dir(path)); len(left) != 1 {
		t.Errorf("temporary files = %q, want the one left by the crash", left)
	}
	store, _ := newTestStore(t, path)
	if err := store.Load(); err != nil || len(store.Config().Tools) != 5 {
		t.Errorf("Load() after crash = %v, tools %d, want the original 5", err, len(store.Config().Tools))
	}
}

func TestStore_CleansStaleTempFiles(t *testing.T) {
	store, path, log := loadedStore(t, []byte("version: 1\n"))
	dir := filepath.Dir(path)
	stale := filepath.Join(dir, ".config.yaml.111.tmp")
	recent := filepath.Join(dir, ".config.yaml.222.tmp")
	foreign := filepath.Join(dir, ".other.yaml.333.tmp")
	old := time.Now().Add(-2 * time.Minute)
	for _, f := range []string{stale, recent, foreign} {
		writeFile(t, f, []byte("x"))
	}
	for _, f := range []string{stale, foreign} {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.Save(sampleTools()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale temp file still exists: %v", err)
	}
	for _, f := range []string{recent, foreign} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s removed: %v", filepath.Base(f), err)
		}
	}
	if !strings.Contains(log.String(), `msg="stale temp file removed"`) {
		t.Errorf("log = %q, want stale temp file removed", log)
	}
}

func TestStore_ConsecutiveSaves(t *testing.T) {
	store, path, _ := loadedStore(t, readFixture(t, "valid_five_tools.yaml"))

	tools := sampleTools()
	if err := store.Save(tools); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := store.Save(tools[:1]); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	cfg, err := Parse(readBytes(t, path), path)
	if err != nil || len(cfg.Tools) != 1 {
		t.Errorf("file = %+v, %v, want one tool", cfg, err)
	}
}

func TestStore_SaveFollowsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "dotfiles", "gbay.yaml")
	writeFile(t, target, []byte("version: 1\n"))
	link := filepath.Join(root, "gopherbay", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not permitted here: %v", err)
	}
	store, _ := newTestStore(t, link)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}

	if err := store.Save(sampleTools()); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link replaced by a regular file: %v", err)
	}
	if cfg, err := Parse(readBytes(t, target), target); err != nil || len(cfg.Tools) != 3 {
		t.Errorf("target = %+v, %v, want the saved tools", cfg, err)
	}
}

func TestStore_PreservesFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	store, path, _ := loadedStore(t, []byte("version: 1\n"))
	if err := os.Chmod(path, 0o644); err != nil { //nolint:gosec // G302: the test needs a mode broader than the default to see it kept
		t.Fatal(err)
	}

	if err := store.Save(sampleTools()); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, %v, want 0644", fi.Mode().Perm(), err)
	}
}

func TestStore_NewFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no POSIX permission bits")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	store, _ := newTestStore(t, path)
	_ = store.Load()

	if err := store.Save(sampleTools()); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, %v, want 0600", fi.Mode().Perm(), err)
	}
}

func TestStore_NoFilesInWorkingDir(t *testing.T) {
	fixture := readFixture(t, "valid_five_tools.yaml")
	wd := t.TempDir()
	t.Chdir(wd)
	dir := t.TempDir()

	missing, _ := newTestStore(t, filepath.Join(dir, "missing", "config.yaml"))
	_ = missing.Load()
	_ = missing.Save(sampleTools())
	_ = missing.Load()

	valid := filepath.Join(dir, "valid.yaml")
	writeFile(t, valid, fixture)
	store, _ := newTestStore(t, valid)
	_ = store.Load()
	_ = store.Save(sampleTools())
	setBeforeRename(t, func(string) error { return errors.New("falha") })
	_ = store.Save(sampleTools()[:1])
	_ = store.Load()

	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("working folder has %d entries, want none", len(entries))
	}
}
