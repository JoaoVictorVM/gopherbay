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

	tea "charm.land/bubbletea/v2"

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

// runSteps runs gbay while drive sends keys and waits for log entries, so a
// test can change files between keys. It quits with q after drive returns.
func runSteps(t *testing.T, opts Options, drive func(send func(string), waitLog func(entry string, count int))) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pr, pw := io.Pipe()
	defer pw.Close()
	var out, stderr bytes.Buffer
	opts.Input, opts.Output, opts.Stderr = pr, &out, &stderr

	logPath := filepath.Join(opts.ConfigDir, logging.FileName)
	readLog := func() string {
		data, _ := os.ReadFile(logPath)
		return string(data)
	}
	waitLog := func(entry string, count int) {
		for strings.Count(readLog(), entry) < count {
			if ctx.Err() != nil {
				t.Errorf("log never had %d %q entries: %q", count, entry, readLog())
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	send := func(keys string) {
		if _, err := pw.Write([]byte(keys)); err != nil {
			t.Errorf("send %q: %v", keys, err)
		}
	}

	done := make(chan error, 1)
	go func() { done <- Run(ctx, opts) }()
	drive(send, waitLog)
	send("q")

	if err := <-done; err != nil {
		t.Errorf("Run() error = %v", err)
	}
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatal("Run() did not return before the deadline")
	}
	return readLog()
}

const brokenConfig = "version: 1\ntools:\n  - name: a\n   command: b\n"

func TestRun_InvalidConfigOpensReadOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeConfig(t, path, brokenConfig)

	log := runSteps(t, Options{ConfigDir: dir}, func(send func(string), waitLog func(string, int)) {
		waitLog(`msg="config load failed"`, 1)
		send("a")
		send("e")
	})

	if !strings.Contains(log, "kind=syntax line=4") {
		t.Errorf("log = %q, want the syntax error on line 4", log)
	}
	if strings.Contains(log, `msg="config saved"`) {
		t.Errorf("log = %q, want no save", log)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != brokenConfig {
		t.Errorf("config bytes = %q, %v, want unchanged", data, err)
	}
}

func TestRun_ReloadWithR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeConfig(t, path, brokenConfig)

	log := runSteps(t, Options{ConfigDir: dir}, func(send func(string), waitLog func(string, int)) {
		waitLog(`msg="config load failed"`, 1)
		writeConfig(t, path, twoTools)
		send("r")
		waitLog(`msg="config loaded"`, 1)
	})

	failed := strings.Index(log, `msg="config load failed"`)
	loaded := strings.Index(log, `msg="config loaded"`)
	if failed < 0 || loaded < failed {
		t.Errorf("log = %q, want a failed load followed by a successful one", log)
	}
	if !strings.Contains(log, "tools=2 missing=false reload=true") {
		t.Errorf("log = %q, want the reload with 2 tools", log)
	}
}

func TestRun_NoFilesInWorkingDir(t *testing.T) {
	wd := t.TempDir()
	t.Chdir(wd)
	dir := t.TempDir()

	runSteps(t, Options{ConfigDir: dir}, func(send func(string), waitLog func(string, int)) {
		waitLog(`msg="config loaded"`, 1)
		send("r")
		waitLog(`msg="config loaded"`, 2)
	})

	entries, err := os.ReadDir(wd)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("working folder has %d entries, want none", len(entries))
	}
}

func TestProgram_SidebarNavigation(t *testing.T) {
	for _, keys := range []string{"jq", "j?jkq"} {
		t.Run(keys, func(t *testing.T) {
			runSidebarProgram(t, keys)
		})
	}
}

func runSidebarProgram(t *testing.T, keys string) {
	t.Helper()
	m, _ := newTestModelWith(t, &fakeStore{cfg: configWith("Relógio", "Snake")})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pr, pw := io.Pipe()
	defer pw.Close()
	var out bytes.Buffer
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithoutSignalHandler(), tea.WithInput(pr), tea.WithOutput(&out), tea.WithWindowSize(120, 30))
	go func() { _, _ = pw.Write([]byte(keys)) }()
	final, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	got := final.(*model)
	tool, ok := got.selectedTool()
	if !ok || tool.Name != "Snake" {
		t.Fatalf("program did not navigate: %+v, %v", tool, ok)
	}
	if !strings.Contains(out.String(), "\x1b[?1049l") {
		t.Fatalf("program did not leave alternate screen: %q", out.String())
	}
}
