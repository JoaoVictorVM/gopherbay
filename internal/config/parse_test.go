package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parseOK(t *testing.T, src string) Config {
	t.Helper()
	cfg, err := Parse([]byte(src), "config.yaml")
	if err != nil {
		t.Fatalf("Parse() error = %v\nsource:\n%s", err, src)
	}
	return cfg
}

func parseErr(t *testing.T, src string) *Error {
	t.Helper()
	_, err := Parse([]byte(src), "config.yaml")
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("Parse() error = %v, want *Error\nsource:\n%s", err, src)
	}
	return e
}

func toolNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

func TestParse_FiveToolsInFileOrder(t *testing.T) {
	cfg, err := Parse(readFixture(t, "valid_five_tools.yaml"), "config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"Relógio", "Snake", "Controle de curso", "Uso", "Git log"}
	if got := toolNames(cfg.Tools); !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
	first := cfg.Tools[0]
	if first.Command != "clock-tty --24h" || first.Description != "Relógio no terminal" || first.Dir.Mode != ModeCurrent {
		t.Errorf("first tool = %+v", first)
	}
	if got := cfg.Tools[3].Command; got != `"C:\Program Files\usage\usage.exe" --bar` {
		t.Errorf("quoted command = %q", got)
	}
	if got := cfg.Tools[4].Command; got != `git log --format='%h "%s"'` {
		t.Errorf("command with quotes = %q", got)
	}
	if cfg.Keys.Release != `ctrl+\` || cfg.Version != 1 {
		t.Errorf("version %d, release %q", cfg.Version, cfg.Keys.Release)
	}
}

func TestParse_Defaults(t *testing.T) {
	cfg := parseOK(t, "version: 1\ntools:\n  - name: a\n    command: b\n  - name: c\n    command: d\n    dir: {}\n")

	if cfg.Keys.Release != DefaultReleaseKey {
		t.Errorf("release = %q, want default", cfg.Keys.Release)
	}
	for _, tool := range cfg.Tools {
		if tool.Dir.Mode != ModeCurrent {
			t.Errorf("%s mode = %q, want current", tool.Name, tool.Dir.Mode)
		}
	}
}

func TestParse_AllModes(t *testing.T) {
	cfg, err := Parse(readFixture(t, "valid_all_modes.yaml"), "config.yaml")
	if err != nil {
		t.Fatal(err)
	}

	want := []Dir{{Mode: ModeCurrent}, {Mode: ModeFixed, Path: "/home/joão/projetos"}, {Mode: ModeAsk, Path: `D:\ultimo`}}
	for i, w := range want {
		if cfg.Tools[i].Dir != w {
			t.Errorf("tool %d dir = %+v, want %+v", i, cfg.Tools[i].Dir, w)
		}
	}
	if cfg.Keys.Release != "ctrl+]" {
		t.Errorf("release = %q", cfg.Keys.Release)
	}
}

func TestParse_EmptyOrCommentOnly(t *testing.T) {
	for _, src := range []string{"", "  \n\n", "# só comentário\n# outro\n", "---\n"} {
		t.Run(fmt.Sprintf("%q", src), func(t *testing.T) {
			cfg := parseOK(t, src)
			if cfg.Version != 1 || cfg.Keys.Release != DefaultReleaseKey || cfg.Tools == nil || len(cfg.Tools) != 0 {
				t.Errorf("Parse() = %+v, want Empty()", cfg)
			}
		})
	}
}

func TestParse_IgnoresBOM(t *testing.T) {
	src := "\xEF\xBB\xBFversion: 1\ntools:\n  - name: a\n    command: \"x\n"

	e := parseErr(t, src)

	if e.Line != 4 {
		t.Errorf("line = %d, want 4 (BOM must not shift lines)", e.Line)
	}
	cfg := parseOK(t, "\xEF\xBB\xBFversion: 1\n")
	if cfg.Version != 1 {
		t.Errorf("version = %d", cfg.Version)
	}
}

func TestParse_SyntaxErrorHasLine(t *testing.T) {
	_, err := Parse(readFixture(t, "invalid_syntax.yaml"), `C:\cfg\config.yaml`)

	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want *Error", err)
	}
	if e.Kind != KindSyntax || e.Line != 12 || e.Message == "" || e.Path != `C:\cfg\config.yaml` {
		t.Errorf("error = %+v, want syntax on line 12 with a message", e)
	}
	if want := "Erro no config.yaml (linha 12): " + e.Message; e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

func TestParse_DuplicateNameFixture(t *testing.T) {
	_, err := Parse(readFixture(t, "invalid_duplicate.yaml"), "config.yaml")

	want := `Erro no config.yaml (linha 8): já existe uma ferramenta chamada "snake"`
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}

func tools(n int) string {
	var sb strings.Builder
	sb.WriteString("version: 1\ntools:\n")
	for i := range n {
		fmt.Fprintf(&sb, "  - name: t%d\n    command: c\n", i)
	}
	return sb.String()
}

func TestParse_ValidationRules(t *testing.T) {
	head := "version: 1\ntools:\n"
	tests := []struct {
		name string
		src  string
		rule Rule
		line int
		msg  string
	}{
		{"version missing", "tools: []\n", RuleVersionMissing, 1, "o campo version é obrigatório (use version: 1)"},
		{"version string", "\nversion: \"1\"\n", RuleVersionInvalid, 2, "version deve ser um número inteiro"},
		{"version float", "version: 1.5\n", RuleVersionInvalid, 1, "version deve ser um número inteiro"},
		{"version zero", "version: 0\n", RuleVersionTooOld, 1, "Versão 0 do config é inválida (use version: 1)"},
		{"version too high", "version: 2\n", RuleVersionUnsupported, 1, "Versão 2 do config não é suportada por esta versão do gbay"},
		{"name missing", head + "  - command: x\n", RuleNameRequired, 3, "o campo name é obrigatório"},
		{"name null item", head + "  - null\n", RuleNameRequired, 3, "o campo name é obrigatório"},
		{"name too long", head + "  - name: " + strings.Repeat("a", 41) + "\n    command: x\n", RuleNameTooLong, 3, "name deve ter no máximo 40 caracteres"},
		{"name duplicate ignoring case", head + "  - name: Snake\n    command: x\n  - name: SNAKE\n    command: y\n", RuleNameDuplicate, 5, `já existe uma ferramenta chamada "SNAKE"`},
		{"command missing", head + "  - name: a\n", RuleCommandRequired, 3, "o campo command é obrigatório"},
		{"command unclosed quote", head + "  - name: a\n    command: 'echo \"abc'\n", RuleCommandUnclosedQuote, 4, "command inválido: aspas sem fechamento"},
		{"command empty program", head + "  - name: a\n    command: '\"\" x'\n", RuleCommandEmptyProgram, 4, "command inválido: nome do programa vazio"},
		{"command too long", head + "  - name: a\n    command: " + strings.Repeat("c", 1025) + "\n", RuleCommandTooLong, 4, "command deve ter no máximo 1024 caracteres"},
		{"description too long", head + "  - name: a\n    command: x\n    description: " + strings.Repeat("d", 121) + "\n", RuleDescriptionTooLong, 5, "description deve ter no máximo 120 caracteres"},
		{"mode invalid", head + "  - name: a\n    command: x\n    dir:\n      mode: pasta\n", RuleModeInvalid, 6, `dir.mode inválido "pasta" (use current, fixed ou ask)`},
		{"fixed without path", head + "  - name: a\n    command: x\n    dir:\n      mode: fixed\n", RulePathRequired, 6, "dir.path é obrigatório quando dir.mode é fixed"},
		{"path too long", head + "  - name: a\n    command: x\n    dir:\n      mode: fixed\n      path: " + strings.Repeat("p", 1025) + "\n", RulePathTooLong, 7, "dir.path deve ter no máximo 1024 caracteres"},
		{"too many tools", tools(101), RuleTooManyTools, 3 + 2*100, "no máximo 100 ferramentas (o arquivo tem 101)"},
		{"unknown top level key", "version: 1\ncolor: red\n", RuleUnknownField, 2, `campo desconhecido "color"`},
		{"unknown key in keys", "version: 1\nkeys:\n  quit: q\n", RuleUnknownField, 3, `campo desconhecido "quit"`},
		{"unknown key in tool", head + "  - name: a\n    command: x\n    cmd: y\n", RuleUnknownField, 5, `campo desconhecido "cmd"`},
		{"unknown key in dir", head + "  - name: a\n    command: x\n    dir:\n      folder: z\n", RuleUnknownField, 6, `campo desconhecido "folder"`},
		{"duplicate key", head + "  - name: a\n    name: b\n    command: x\n", RuleDuplicateKey, 4, `campo "name" repetido`},
		{"control character", head + "  - name: \"a\\u001b[31m\"\n    command: x\n", RuleControlCharacter, 3, "name não pode conter caracteres de controle"},
		{"release key invalid", "version: 1\nkeys:\n  release: x\n", RuleReleaseKeyInvalid, 3, `keys.release inválido: "x" (use a notação do Bubble Tea, por exemplo "ctrl+]")`},
		{"release key empty", "version: 1\nkeys:\n  release: \"\"\n", RuleReleaseKeyInvalid, 3, `keys.release inválido: "" (use a notação do Bubble Tea, por exemplo "ctrl+]")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := parseErr(t, tt.src)

			if e.Rule != tt.rule || e.Line != tt.line || e.Message != tt.msg {
				t.Errorf("error = rule %q line %d message %q, want rule %q line %d message %q", e.Rule, e.Line, e.Message, tt.rule, tt.line, tt.msg)
			}
			if e.Kind != KindValidation {
				t.Errorf("kind = %q, want validation", e.Kind)
			}
		})
	}
}

func TestParse_VersionTooHigh(t *testing.T) {
	_, err := Parse([]byte("version: 2\ntools: []\n"), "config.yaml")

	want := "Erro no config.yaml (linha 1): Versão 2 do config não é suportada por esta versão do gbay"
	if err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
}

func TestParse_VersionCheckedBeforeUnknownKeys(t *testing.T) {
	e := parseErr(t, "version: 2\nthemes: dark\n")

	if e.Rule != RuleVersionUnsupported {
		t.Errorf("rule = %q, want version_unsupported", e.Rule)
	}
}

func TestParse_LowestLineWins(t *testing.T) {
	src := "version: 1\ntools:\n  - name: a\n    command: \"'x\"\n  - name: b\n    command: y\n  - name: c\n    command: z\n    description: " + strings.Repeat("d", 121) + "\n"

	e := parseErr(t, src)

	if e.Line != 4 || e.Rule != RuleCommandUnclosedQuote {
		t.Errorf("error = line %d rule %q, want line 4 command_unclosed_quote", e.Line, e.Rule)
	}
	if e.Violations != 2 {
		t.Errorf("violations = %d, want 2", e.Violations)
	}

	src = "version: 1\nkeys:\n  release: x\ntools:\n  - command: y\n"
	if e := parseErr(t, src); e.Line != 3 || e.Rule != RuleReleaseKeyInvalid {
		t.Errorf("error = line %d rule %q, want line 3 release_key_invalid", e.Line, e.Rule)
	}
}

func TestParse_TrimsValues(t *testing.T) {
	cfg := parseOK(t, "version: 1\ntools:\n  - name: '  "+strings.Repeat("a", 40)+"  '\n    command: '  prog --x  '\n    description: ' d '\n")

	tool := cfg.Tools[0]
	if tool.Name != strings.Repeat("a", 40) || tool.Command != "prog --x" || tool.Description != "d" {
		t.Errorf("tool = %+v, want trimmed values", tool)
	}
}

func TestParse_NormalizesReleaseKey(t *testing.T) {
	cfg := parseOK(t, "version: 1\nkeys:\n  release: ALT+CTRL+X\n")

	if cfg.Keys.Release != "ctrl+alt+x" {
		t.Errorf("release = %q, want ctrl+alt+x", cfg.Keys.Release)
	}
}

func TestParse_CountsCodePoints(t *testing.T) {
	src := func(n int) string {
		return "version: 1\ntools:\n  - name: " + strings.Repeat("ç", n) + "\n    command: x\n"
	}

	parseOK(t, src(40))
	if e := parseErr(t, src(41)); e.Rule != RuleNameTooLong {
		t.Errorf("rule = %q, want name_too_long", e.Rule)
	}
}

func TestParse_PathKeptForOtherModes(t *testing.T) {
	long := strings.Repeat("p", 2000)

	cfg := parseOK(t, "version: 1\ntools:\n  - name: a\n    command: x\n    dir:\n      mode: ask\n      path: "+long+"\n")

	if cfg.Tools[0].Dir.Path != long {
		t.Errorf("path length = %d, want 2000", len(cfg.Tools[0].Dir.Path))
	}
}

func TestParse_TooLarge(t *testing.T) {
	data := []byte("version: 1\n# " + strings.Repeat("x", MaxFileSize) + "\n")

	_, err := Parse(data, "config.yaml")

	var e *Error
	if !errors.As(err, &e) || e.Kind != KindRead || e.Rule != RuleFileTooLarge {
		t.Fatalf("error = %v, want read file_too_large", err)
	}
	if want := "Não foi possível ler o config.yaml: o arquivo tem mais de 1 MiB"; e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

func TestParse_MultipleDocuments(t *testing.T) {
	e := parseErr(t, "version: 1\n---\nversion: 1\n")

	if e.Rule != RuleMultipleDocuments || e.Line != 3 {
		t.Errorf("error = rule %q line %d, want multiple_documents on line 3", e.Rule, e.Line)
	}
}

func TestParse_TopLevelNotMapping(t *testing.T) {
	for _, src := range []string{"- a\n- b\n", "plain\n", "42\n"} {
		t.Run(src, func(t *testing.T) {
			if e := parseErr(t, src); e.Rule != RuleNotAMapping || e.Line != 1 {
				t.Errorf("error = rule %q line %d, want not_a_mapping on line 1", e.Rule, e.Line)
			}
		})
	}
}

func TestParse_TypeErrorKeepsParserMessage(t *testing.T) {
	e := parseErr(t, "version: 1\ntools:\n  - name: [1, 2]\n    command: x\n")

	if e.Rule != RuleType || e.Line != 3 || e.Message == "" {
		t.Errorf("error = %+v, want type error on line 3", e)
	}
}
