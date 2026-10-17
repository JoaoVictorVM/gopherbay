package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const versionPkg = "github.com/JoaoVictorVM/gopherbay/internal/version"

func buildBinary(t *testing.T, ldflags string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the real binary; skipped with -short")
	}
	bin := filepath.Join(t.TempDir(), "gbay")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	args := []string{"build", "-trimpath", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	cmd := exec.CommandContext(t.Context(), "go", append(args, ".")...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

func runBinary(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Env = append(os.Environ(), "APPDATA="+dir, "XDG_CONFIG_HOME="+dir, "HOME="+dir)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut

	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		code = 0
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		t.Fatalf("running %s: %v", bin, err)
	}
	return out.String(), errOut.String(), code
}

func TestBinary_VersionWithLdflags(t *testing.T) {
	bin := buildBinary(t, "-X "+versionPkg+".Version=v9.9.9 -X "+versionPkg+".Commit=deadbee -X "+versionPkg+".Date=2026-11-01")

	stdout, stderr, code := runBinary(t, bin, "--version")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if want := "gbay v9.9.9 (commit deadbee, 2026-11-01)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestBinary_VersionPlainBuild(t *testing.T) {
	bin := buildBinary(t, "")

	stdout, _, code := runBinary(t, bin, "--version")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if want := "gbay dev (commit none, data desconhecida)\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestBinary_UnknownFlagExitCode(t *testing.T) {
	bin := buildBinary(t, "")

	stdout, stderr, code := runBinary(t, bin, "--flag-inexistente")

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "Uso: gbay [opções]") {
		t.Errorf("stderr = %q, want usage", stderr)
	}
}
