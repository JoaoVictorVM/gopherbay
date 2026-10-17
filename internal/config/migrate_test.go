package config

import (
	"bytes"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml/ast"
)

func testParser(current int, steps map[int]migrateFunc) (configParser, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	return configParser{current: current, migrations: steps, logger: logger}, &buf
}

// renameToolKey stands in for a real migration: a legacy format that called
// the command field "cmd".
func renameToolKey(from, to string) migrateFunc {
	return func(body ast.Node) error {
		seq, ok := field(body, "tools").(*ast.SequenceNode)
		if !ok {
			return nil
		}
		for _, item := range seq.Values {
			for _, mv := range mappingValues(item) {
				tk := mv.Key.GetToken()
				if tk.Value != from {
					continue
				}
				tk.Value = to
				if s, ok := mv.Key.(*ast.StringNode); ok {
					s.Value = to
				}
			}
		}
		return nil
	}
}

func TestMigrate_SyntheticVersion(t *testing.T) {
	p, log := testParser(1, map[int]migrateFunc{0: renameToolKey("cmd", "command")})

	cfg, err := p.parse([]byte("version: 0\ntools:\n  - name: a\n    cmd: prog --x\n"), "config.yaml")
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}

	if cfg.Version != 1 || len(cfg.Tools) != 1 || cfg.Tools[0].Command != "prog --x" {
		t.Errorf("config = %+v, want migrated command", cfg)
	}
	if !strings.Contains(log.String(), `msg="config migrated" from=0 to=1`) {
		t.Errorf("log = %q, want config migrated entry", log)
	}
}

func TestMigrate_Chain(t *testing.T) {
	var order []int
	step := func(v int) migrateFunc {
		return func(ast.Node) error {
			order = append(order, v)
			return nil
		}
	}
	p, _ := testParser(3, map[int]migrateFunc{1: step(1), 2: step(2)})

	if _, err := p.parse([]byte("version: 1\n"), "config.yaml"); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(order, []int{1, 2}) {
		t.Errorf("steps = %v, want [1 2]", order)
	}
}

func TestMigrate_MissingStep(t *testing.T) {
	ran := false
	p, _ := testParser(3, map[int]migrateFunc{2: func(ast.Node) error { ran = true; return nil }})

	_, err := p.parse([]byte("version: 1\n"), "config.yaml")

	var e *Error
	if !errors.As(err, &e) || e.Rule != RuleVersionTooOld || e.Line != 1 {
		t.Fatalf("error = %v, want version_too_old on line 1", err)
	}
	if want := "Versão 1 do config é inválida (use version: 3)"; e.Message != want {
		t.Errorf("message = %q, want %q", e.Message, want)
	}
	if ran {
		t.Error("a step ran although the chain has a gap")
	}
}

func TestMigrate_StepFails(t *testing.T) {
	boom := errors.New("boom")
	p, _ := testParser(2, map[int]migrateFunc{1: func(ast.Node) error { return boom }})

	_, err := p.parse([]byte("version: 1\n"), "config.yaml")

	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want it to wrap the step error", err)
	}
}

func TestMigrate_KeepsLines(t *testing.T) {
	p, _ := testParser(1, map[int]migrateFunc{0: renameToolKey("cmd", "command")})
	src := "version: 0\ntools:\n  - name: a\n    cmd: x\n  - name: b\n    cmd: 'echo \"y'\n"

	_, err := p.parse([]byte(src), "config.yaml")

	var e *Error
	if !errors.As(err, &e) || e.Rule != RuleCommandUnclosedQuote || e.Line != 6 {
		t.Errorf("error = %v, want command_unclosed_quote on line 6", err)
	}
}

func TestMigrate_NegativeVersion(t *testing.T) {
	e := parseErr(t, "version: -1\n")

	if e.Rule != RuleVersionTooOld {
		t.Errorf("rule = %q, want version_too_old", e.Rule)
	}
}

func TestMigrations_EmptyInV1(t *testing.T) {
	if len(migrations) != 0 {
		t.Errorf("migrations has %d steps, want none in version 1", len(migrations))
	}
	if CurrentVersion != 1 {
		t.Errorf("CurrentVersion = %d, want 1", CurrentVersion)
	}
}
