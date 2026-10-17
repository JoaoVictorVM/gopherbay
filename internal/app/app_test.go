package app

import (
	"bytes"
	"context"
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
