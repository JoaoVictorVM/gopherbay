package config

import (
	"fmt"
	"strings"
	"testing"
)

func validTool(name string) Tool {
	return Tool{Name: name, Command: "prog --flag", Dir: Dir{Mode: ModeCurrent}}
}

func TestValidateTools(t *testing.T) {
	tests := []struct {
		name  string
		tool  Tool
		field string
		rule  Rule
		msg   string
	}{
		{"missing name", Tool{Name: "  ", Command: "x"}, FieldName, RuleNameRequired, "o campo name é obrigatório"},
		{"name too long", Tool{Name: strings.Repeat("a", 41), Command: "x"}, FieldName, RuleNameTooLong, "name deve ter no máximo 40 caracteres"},
		{"name control", Tool{Name: "a\x1b[31m", Command: "x"}, FieldName, RuleControlCharacter, "name não pode conter caracteres de controle"},
		{"name tab", Tool{Name: "a\tb", Command: "x"}, FieldName, RuleControlCharacter, "name não pode conter caracteres de controle"},
		{"missing command", Tool{Name: "a"}, FieldCommand, RuleCommandRequired, "o campo command é obrigatório"},
		{"command too long", Tool{Name: "a", Command: strings.Repeat("c", 1025)}, FieldCommand, RuleCommandTooLong, "command deve ter no máximo 1024 caracteres"},
		{"unclosed quote", Tool{Name: "a", Command: `echo "abc`}, FieldCommand, RuleCommandUnclosedQuote, "command inválido: aspas sem fechamento"},
		{"empty program", Tool{Name: "a", Command: `"" x`}, FieldCommand, RuleCommandEmptyProgram, "command inválido: nome do programa vazio"},
		{"command control", Tool{Name: "a", Command: "a\nb"}, FieldCommand, RuleControlCharacter, "command não pode conter caracteres de controle"},
		{"description too long", Tool{Name: "a", Command: "x", Description: strings.Repeat("d", 121)}, FieldDescription, RuleDescriptionTooLong, "description deve ter no máximo 120 caracteres"},
		{"description control", Tool{Name: "a", Command: "x", Description: "a\x7f"}, FieldDescription, RuleControlCharacter, "description não pode conter caracteres de controle"},
		{"invalid mode", Tool{Name: "a", Command: "x", Dir: Dir{Mode: "pasta"}}, FieldMode, RuleModeInvalid, `dir.mode inválido "pasta" (use current, fixed ou ask)`},
		{"fixed without path", Tool{Name: "a", Command: "x", Dir: Dir{Mode: ModeFixed, Path: " "}}, FieldMode, RulePathRequired, "dir.path é obrigatório quando dir.mode é fixed"},
		{"path too long", Tool{Name: "a", Command: "x", Dir: Dir{Mode: ModeFixed, Path: strings.Repeat("p", 1025)}}, FieldPath, RulePathTooLong, "dir.path deve ter no máximo 1024 caracteres"},
		{"path control", Tool{Name: "a", Command: "x", Dir: Dir{Mode: ModeFixed, Path: "a\x00"}}, FieldPath, RuleControlCharacter, "dir.path não pode conter caracteres de controle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateTools([]Tool{validTool("other"), tt.tool})

			if len(got) != 1 {
				t.Fatalf("ValidateTools() = %+v, want one violation", got)
			}
			v := got[0]
			if v.Index != 1 || v.Field != tt.field || v.Rule != tt.rule || v.Message != tt.msg {
				t.Errorf("violation = %+v, want index 1, field %q, rule %q, message %q", v, tt.field, tt.rule, tt.msg)
			}
		})
	}
}

func TestValidateTools_Valid(t *testing.T) {
	tools := []Tool{
		validTool("Relógio"),
		{Name: strings.Repeat("ç", 40), Command: `"C:\Program Files\x.exe" -v`, Description: strings.Repeat("é", 120)},
		{Name: "fixed", Command: "x", Dir: Dir{Mode: ModeFixed, Path: `D:\projetos\curso`}},
		{Name: "ask with long path", Command: "x", Dir: Dir{Mode: ModeAsk, Path: strings.Repeat("p", 2000)}},
		{Name: "  trimmed  ", Command: "  x  "},
	}

	if got := ValidateTools(tools); len(got) != 0 {
		t.Errorf("ValidateTools() = %+v, want none", got)
	}
}

func TestValidateTools_DuplicateIgnoresCase(t *testing.T) {
	tests := [][2]string{{"Snake", "snake"}, {"Relógio", "RELÓGIO"}, {"Snake", " SNAKE "}}
	for _, pair := range tests {
		t.Run(pair[1], func(t *testing.T) {
			got := ValidateTools([]Tool{validTool(pair[0]), validTool(pair[1])})

			if len(got) != 1 || got[0].Index != 1 || got[0].Rule != RuleNameDuplicate {
				t.Fatalf("ValidateTools() = %+v, want one name_duplicate on index 1", got)
			}
			want := fmt.Sprintf("já existe uma ferramenta chamada %q", strings.TrimSpace(pair[1]))
			if got[0].Message != want {
				t.Errorf("message = %q, want %q", got[0].Message, want)
			}
		})
	}
}

func TestValidateTools_TooMany(t *testing.T) {
	tools := make([]Tool, MaxTools+1)
	for i := range tools {
		tools[i] = validTool(fmt.Sprintf("t%d", i))
	}

	got := ValidateTools(tools)

	if len(got) != 1 || got[0].Rule != RuleTooManyTools || got[0].Index != MaxTools {
		t.Fatalf("ValidateTools() = %+v, want one too_many_tools on index %d", got, MaxTools)
	}
	if want := "no máximo 100 ferramentas (o arquivo tem 101)"; got[0].Message != want {
		t.Errorf("message = %q, want %q", got[0].Message, want)
	}
	if got := ValidateTools(tools[:MaxTools]); len(got) != 0 {
		t.Errorf("100 tools: %+v, want none", got)
	}
}

func TestValidateReleaseKey(t *testing.T) {
	valid := []struct{ in, want string }{
		{`ctrl+\`, `ctrl+\`},
		{"ctrl+]", "ctrl+]"},
		{"alt+x", "alt+x"},
		{"f12", "f12"},
		{"CTRL+]", "ctrl+]"},
		{"alt+ctrl+x", "ctrl+alt+x"},
		{"shift+f5", "shift+f5"},
		{"ctrl+space", "ctrl+space"},
		{"ctrl++", "ctrl++"},
		{" ctrl+] ", "ctrl+]"},
		{"super+esc", "super+esc"},
	}
	for _, tt := range valid {
		t.Run("valid "+tt.in, func(t *testing.T) {
			got, err := ValidateReleaseKey(tt.in)
			if err != nil || got != tt.want {
				t.Errorf("ValidateReleaseKey(%q) = %q, %v, want %q", tt.in, got, err, tt.want)
			}
		})
	}

	for _, in := range []string{"x", "shift+a", "ctrl+", "hyper+foo", "", "space", "+", "ctrl+ctrl+x", "win+x", "ctrl+ab", "ctrl+é+x"} {
		t.Run("invalid "+in, func(t *testing.T) {
			got, err := ValidateReleaseKey(in)
			if err == nil {
				t.Fatalf("ValidateReleaseKey(%q) = %q, want error", in, got)
			}
			want := `keys.release inválido: "` + in + `" (use a notação do Bubble Tea, por exemplo "ctrl+]")`
			if err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
		})
	}
}
