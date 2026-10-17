package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type cliResult struct {
	code   int
	stdout string
	stderr string
}

func isolateConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(base, "gopherbay")
}

func runCLI(t *testing.T, keys string, args ...string) cliResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pr, pw := io.Pipe()
	defer pw.Close()
	if keys != "" {
		go func() { _, _ = pw.Write([]byte(keys)) }()
	}

	var stdout, stderr bytes.Buffer
	code := run(ctx, args, pr, &stdout, &stderr)
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatal("run() did not return before the deadline")
	}
	return cliResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestRun_Version(t *testing.T) {
	isolateConfigDir(t)

	res := runCLI(t, "", "--version")

	if res.code != 0 {
		t.Errorf("exit code = %d, want 0", res.code)
	}
	if want := "gbay dev (commit none, data desconhecida)\n"; res.stdout != want {
		t.Errorf("stdout = %q, want %q", res.stdout, want)
	}
	if res.stderr != "" {
		t.Errorf("stderr = %q, want empty", res.stderr)
	}
}

func TestRun_VersionSkipsLogging(t *testing.T) {
	logDir := isolateConfigDir(t)

	runCLI(t, "", "--version", "--debug")

	if _, err := os.Stat(filepath.Join(logDir, "gbay.log")); !os.IsNotExist(err) {
		t.Errorf("gbay.log should not exist after --version, Stat error = %v", err)
	}
}

func TestRun_UnknownFlag(t *testing.T) {
	isolateConfigDir(t)

	res := runCLI(t, "", "--flag-inexistente")

	if res.code != 2 {
		t.Errorf("exit code = %d, want 2", res.code)
	}
	if res.stdout != "" {
		t.Errorf("stdout = %q, want empty", res.stdout)
	}
	if !strings.HasPrefix(res.stderr, "gbay: argumentos inválidos: ") {
		t.Errorf("stderr = %q, want the pt-BR error line", res.stderr)
	}
	if !strings.Contains(res.stderr, "Uso: gbay [opções]") {
		t.Errorf("stderr = %q, want usage", res.stderr)
	}
}

func TestRun_UnexpectedArgument(t *testing.T) {
	isolateConfigDir(t)

	res := runCLI(t, "", "foo")

	if res.code != 2 {
		t.Errorf("exit code = %d, want 2", res.code)
	}
	if !strings.HasPrefix(res.stderr, "gbay: argumento inesperado: foo\n") {
		t.Errorf("stderr = %q, want unexpected argument line", res.stderr)
	}
	if !strings.Contains(res.stderr, "Uso: gbay [opções]") {
		t.Errorf("stderr = %q, want usage", res.stderr)
	}
}

func TestRun_Help(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			isolateConfigDir(t)

			res := runCLI(t, "", arg)

			if res.code != 0 {
				t.Errorf("exit code = %d, want 0", res.code)
			}
			if res.stdout != usage {
				t.Errorf("stdout = %q, want usage", res.stdout)
			}
			if res.stderr != "" {
				t.Errorf("stderr = %q, want empty", res.stderr)
			}
		})
	}
}

func TestRun_StartsInterface(t *testing.T) {
	logDir := isolateConfigDir(t)

	res := runCLI(t, "q")

	if res.code != 0 {
		t.Errorf("exit code = %d, want 0; stderr = %q", res.code, res.stderr)
	}
	if res.stderr != "" {
		t.Errorf("stderr = %q, want empty", res.stderr)
	}
	if _, err := os.Stat(filepath.Join(logDir, "gbay.log")); err != nil {
		t.Errorf("gbay.log missing: %v", err)
	}
}

func TestRun_DebugFlagReachesLog(t *testing.T) {
	logDir := isolateConfigDir(t)

	res := runCLI(t, "q", "--debug")

	if res.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", res.code, res.stderr)
	}
	data, err := os.ReadFile(filepath.Join(logDir, "gbay.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "level=DEBUG") {
		t.Errorf("log = %q, want a debug entry", data)
	}
}

func TestRun_ConfigFlagForms(t *testing.T) {
	for _, args := range [][]string{{"--config", "demo.yaml"}, {"--config=demo.yaml"}, {"-config", "demo.yaml"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			logDir := isolateConfigDir(t)
			t.Chdir(t.TempDir())

			res := runCLI(t, "q", args...)

			if res.code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr = %q", res.code, res.stderr)
			}
			data, err := os.ReadFile(filepath.Join(logDir, "gbay.log"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "demo.yaml") {
				t.Errorf("log = %q, want the --config path", data)
			}
		})
	}
}
