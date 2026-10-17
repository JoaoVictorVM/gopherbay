package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JoaoVictorVM/gopherbay/internal/logging"
)

type runResult struct {
	err    error
	output string
	stderr string
	log    string
}

// The pipe stays open until Run returns so the program only ends through the
// keys sent, and the deadline turns a hang into a failure.
func runApp(ctx context.Context, t *testing.T, opts Options, keys string) runResult {
	t.Helper()
	if opts.ConfigDir == "" {
		opts.ConfigDir = t.TempDir()
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	pr, pw := io.Pipe()
	defer pw.Close()
	var out, stderr bytes.Buffer
	opts.Input, opts.Output, opts.Stderr = pr, &out, &stderr

	if keys != "" {
		go func() { _, _ = pw.Write([]byte(keys)) }()
	}

	err := Run(ctx, opts)
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatal("Run() did not return before the deadline")
	}

	res := runResult{err: err, output: out.String(), stderr: stderr.String()}
	if data, readErr := os.ReadFile(filepath.Join(opts.ConfigDir, logging.FileName)); readErr == nil {
		res.log = string(data)
	}
	return res
}

func TestRun_QuitsOnQ(t *testing.T) {
	res := runApp(context.Background(), t, Options{}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	for _, want := range []string{`msg="starting gbay"`, `msg="gbay exited"`, "duration="} {
		if !strings.Contains(res.log, want) {
			t.Errorf("log = %q, want %q", res.log, want)
		}
	}
}

func TestRun_RotatesLog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, logging.FileName), []byte("previous run\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runApp(context.Background(), t, Options{ConfigDir: dir}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	old, err := os.ReadFile(filepath.Join(dir, logging.OldFileName))
	if err != nil {
		t.Fatalf("old log missing: %v", err)
	}
	if string(old) != "previous run\n" {
		t.Errorf("old log = %q, want previous content", old)
	}
	if strings.Contains(res.log, "previous run") {
		t.Errorf("log = %q, should hold only the new run", res.log)
	}
}

func TestRun_DebugFlag(t *testing.T) {
	res := runApp(context.Background(), t, Options{Debug: true}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	if !strings.Contains(res.log, `level=DEBUG msg="first frame"`) {
		t.Errorf("log = %q, want first frame debug entry", res.log)
	}
	if !strings.Contains(res.log, "debug=true") {
		t.Errorf("log = %q, want debug=true in the startup entry", res.log)
	}
}

func TestRun_InfoByDefault(t *testing.T) {
	res := runApp(context.Background(), t, Options{}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	if strings.Contains(res.log, "level=DEBUG") {
		t.Errorf("log = %q, want no debug entries", res.log)
	}
}

func TestRun_ConfigOverrideLogged(t *testing.T) {
	wd := t.TempDir()
	t.Chdir(wd)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	res := runApp(context.Background(), t, Options{ConfigPath: "demo.yaml"}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	want := filepath.Join(cwd, "demo.yaml")
	if !strings.Contains(res.log, "config_path="+want) && !strings.Contains(res.log, "config_path="+quoteLogValue(want)) {
		t.Errorf("log = %q, want config_path=%s", res.log, want)
	}
}

func TestRun_LogOpenFailureContinues(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gopherbay")
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	res := runApp(context.Background(), t, Options{ConfigDir: dir}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v, want nil", res.err)
	}
	want := "gbay: aviso: não foi possível abrir o log em " + dir + ": "
	if !strings.HasPrefix(res.stderr, want) {
		t.Errorf("stderr = %q, want prefix %q", res.stderr, want)
	}
}

func TestRun_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := runApp(ctx, t, Options{}, "")

	if res.err != nil {
		t.Errorf("Run() error = %v, want nil", res.err)
	}
}

func TestRun_NoStrayOutput(t *testing.T) {
	res := runApp(context.Background(), t, Options{Debug: true}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	if res.stderr != "" {
		t.Errorf("stderr = %q, want empty", res.stderr)
	}
	for _, entry := range []string{"starting gbay", "first frame", "level=", "gbay exited"} {
		if strings.Contains(res.output, entry) {
			t.Errorf("output contains log entry %q: %q", entry, res.output)
		}
	}
	if !strings.Contains(res.log, `msg="first frame"`) {
		t.Errorf("log = %q, want entries in the file", res.log)
	}
}

func quoteLogValue(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}

const twoTools = "version: 1\ntools:\n  - name: Relógio\n    command: clock-tty\n  - name: Snake\n    command: snake\n"

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRun_LoadsConfigBeforeFirstFrame(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, filepath.Join(dir, "config.yaml"), twoTools)

	res := runApp(context.Background(), t, Options{ConfigDir: dir, Debug: true}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	loaded := strings.Index(res.log, `msg="config loaded"`)
	frame := strings.Index(res.log, `msg="first frame"`)
	if loaded < 0 || frame < 0 || loaded > frame {
		t.Errorf("log = %q, want config loaded before first frame", res.log)
	}
}

func TestRun_ConfigOverrideLoadsThatFile(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(t.TempDir(), "demo.yaml")
	writeConfig(t, other, twoTools)

	res := runApp(context.Background(), t, Options{ConfigDir: dir, ConfigPath: other}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	if !strings.Contains(res.log, `msg="config loaded"`) || !strings.Contains(res.log, "tools=2") {
		t.Errorf("log = %q, want config loaded with tools=2", res.log)
	}
	if !strings.Contains(res.log, "path="+other) && !strings.Contains(res.log, "path="+quoteLogValue(other)) {
		t.Errorf("log = %q, want path=%s", res.log, other)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config.yaml created in the config dir: %v", err)
	}
}

func TestRun_MissingConfigCreatesNothing(t *testing.T) {
	dir := t.TempDir()

	res := runApp(context.Background(), t, Options{ConfigDir: dir}, "q")

	if res.err != nil {
		t.Fatalf("Run() error = %v", res.err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config.yaml exists after a run without one: %v", err)
	}
	if !strings.Contains(res.log, "missing=true") {
		t.Errorf("log = %q, want missing=true", res.log)
	}
}
