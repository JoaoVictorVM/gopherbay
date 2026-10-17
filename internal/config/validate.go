package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/JoaoVictorVM/gopherbay/internal/cmdline"
)

// Rule identifies a validation rule. The values appear in the log.
type Rule string

// Validation rules.
const (
	RuleVersionMissing       Rule = "version_missing"
	RuleVersionInvalid       Rule = "version_invalid"
	RuleVersionUnsupported   Rule = "version_unsupported"
	RuleVersionTooOld        Rule = "version_too_old"
	RuleUnknownField         Rule = "unknown_field"
	RuleDuplicateKey         Rule = "duplicate_key"
	RuleNameRequired         Rule = "name_required"
	RuleNameTooLong          Rule = "name_too_long"
	RuleNameDuplicate        Rule = "name_duplicate"
	RuleCommandRequired      Rule = "command_required"
	RuleCommandTooLong       Rule = "command_too_long"
	RuleCommandUnclosedQuote Rule = "command_unclosed_quote"
	RuleCommandEmptyProgram  Rule = "command_empty_program"
	RuleDescriptionTooLong   Rule = "description_too_long"
	RuleModeInvalid          Rule = "mode_invalid"
	RulePathRequired         Rule = "path_required"
	RulePathTooLong          Rule = "path_too_long"
	RuleControlCharacter     Rule = "control_character"
	RuleReleaseKeyInvalid    Rule = "release_key_invalid"
	RuleTooManyTools         Rule = "too_many_tools"
	RuleMultipleDocuments    Rule = "multiple_documents"
	RuleNotAMapping          Rule = "not_a_mapping"
	RuleFileTooLarge         Rule = "file_too_large"
	RuleUnreadable           Rule = "unreadable"
	RuleTagUnsupported       Rule = "tag_unsupported"
	RuleSyntax               Rule = "syntax"
	RuleType                 Rule = "type"
)

// Tool fields as named in the file, used to locate a violation.
const (
	FieldName        = "name"
	FieldCommand     = "command"
	FieldDescription = "description"
	FieldMode        = "dir.mode"
	FieldPath        = "dir.path"
)

// Violation is one broken rule in a tool list.
type Violation struct {
	// Index is the position of the tool in the list.
	Index int
	// Field is empty when the violation is about the whole item.
	Field   string
	Rule    Rule
	Message string
}

// ValidateTools checks tools against every rule and returns the violations in
// list order. Values are measured after trimming spaces, as they are stored.
func ValidateTools(tools []Tool) []Violation {
	var out []Violation
	add := func(i int, field string, rule Rule, msg string) {
		out = append(out, Violation{Index: i, Field: field, Rule: rule, Message: msg})
	}

	for i, t := range tools {
		if i == MaxTools {
			add(i, "", RuleTooManyTools, fmt.Sprintf("no máximo %d ferramentas (o arquivo tem %d)", MaxTools, len(tools)))
		}

		name := strings.TrimSpace(t.Name)
		switch {
		case name == "":
			add(i, FieldName, RuleNameRequired, "o campo name é obrigatório")
		case hasControl(name):
			add(i, FieldName, RuleControlCharacter, controlMessage(FieldName))
		case utf8.RuneCountInString(name) > MaxNameLen:
			add(i, FieldName, RuleNameTooLong, fmt.Sprintf("name deve ter no máximo %d caracteres", MaxNameLen))
		case slices.ContainsFunc(tools[:i], func(o Tool) bool { return strings.EqualFold(strings.TrimSpace(o.Name), name) }):
			add(i, FieldName, RuleNameDuplicate, fmt.Sprintf("já existe uma ferramenta chamada %q", name))
		}

		if v, ok := validateCommand(strings.TrimSpace(t.Command)); !ok {
			add(i, FieldCommand, v.Rule, v.Message)
		}

		desc := strings.TrimSpace(t.Description)
		switch {
		case hasControl(desc):
			add(i, FieldDescription, RuleControlCharacter, controlMessage(FieldDescription))
		case utf8.RuneCountInString(desc) > MaxDescriptionLen:
			add(i, FieldDescription, RuleDescriptionTooLong, fmt.Sprintf("description deve ter no máximo %d caracteres", MaxDescriptionLen))
		}

		switch t.Dir.Mode {
		case ModeCurrent, ModeAsk, "":
		case ModeFixed:
			path := strings.TrimSpace(t.Dir.Path)
			switch {
			case path == "":
				add(i, FieldMode, RulePathRequired, "dir.path é obrigatório quando dir.mode é fixed")
			case hasControl(path):
				add(i, FieldPath, RuleControlCharacter, controlMessage(FieldPath))
			case utf8.RuneCountInString(path) > MaxPathLen:
				add(i, FieldPath, RulePathTooLong, fmt.Sprintf("dir.path deve ter no máximo %d caracteres", MaxPathLen))
			}
		default:
			add(i, FieldMode, RuleModeInvalid, fmt.Sprintf("dir.mode inválido %q (use current, fixed ou ask)", string(t.Dir.Mode)))
		}
	}
	return out
}

func validateCommand(cmd string) (Violation, bool) {
	switch {
	case cmd == "":
		return Violation{Rule: RuleCommandRequired, Message: "o campo command é obrigatório"}, false
	case hasControl(cmd):
		return Violation{Rule: RuleControlCharacter, Message: controlMessage(FieldCommand)}, false
	case utf8.RuneCountInString(cmd) > MaxCommandLen:
		return Violation{Rule: RuleCommandTooLong, Message: fmt.Sprintf("command deve ter no máximo %d caracteres", MaxCommandLen)}, false
	}
	args, err := cmdline.Split(cmd)
	if errors.Is(err, cmdline.ErrUnclosedQuote) {
		return Violation{Rule: RuleCommandUnclosedQuote, Message: "command inválido: aspas sem fechamento"}, false
	}
	if err != nil || len(args) == 0 || args[0] == "" {
		return Violation{Rule: RuleCommandEmptyProgram, Message: "command inválido: nome do programa vazio"}, false
	}
	return Violation{}, true
}

// Names, commands and paths are printed in the terminal, so an escape
// character here could inject terminal sequences.
func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

func controlMessage(field string) string {
	return field + " não pode conter caracteres de controle"
}

// Modifiers in the order Bubble Tea writes them in Key.Keystroke.
var keyModifiers = []string{"ctrl", "alt", "shift", "meta", "hyper", "super"}

var namedKeys = map[string]bool{
	"enter": true, "tab": true, "backspace": true, "esc": true,
	"up": true, "down": true, "left": true, "right": true,
	"insert": true, "delete": true, "home": true, "end": true, "pgup": true, "pgdown": true,
}

func init() {
	for i := 1; i <= 20; i++ {
		namedKeys[fmt.Sprintf("f%d", i)] = true
	}
}

// ValidateReleaseKey checks a keystroke in Bubble Tea notation and returns it
// lowercased with the modifiers in canonical order, so it compares equal to
// tea.KeyPressMsg.String().
func ValidateReleaseKey(s string) (string, error) {
	invalid := fmt.Errorf("keys.release inválido: \"%s\" (use a notação do Bubble Tea, por exemplo \"ctrl+]\")", s)

	ks := strings.ToLower(strings.TrimSpace(s))
	if ks == "" {
		return "", invalid
	}

	// The plus key itself is written as "+" or "ctrl++".
	var key, rest string
	if ks == "+" || strings.HasSuffix(ks, "++") {
		key, rest = "+", strings.TrimSuffix(ks[:len(ks)-1], "+")
	} else if i := strings.LastIndex(ks, "+"); i >= 0 {
		key, rest = ks[i+1:], ks[:i]
	} else {
		key = ks
	}

	present := map[string]bool{}
	if rest != "" {
		for _, mod := range strings.Split(rest, "+") {
			if !slices.Contains(keyModifiers, mod) || present[mod] {
				return "", invalid
			}
			present[mod] = true
		}
	}

	switch {
	case namedKeys[key]:
	case key == "space" || isPrintableKey(key):
		// A plain or shifted character is ordinary typing, which the running
		// tool must receive, so it needs a modifier that is not shift.
		if !present["ctrl"] && !present["alt"] && !present["meta"] && !present["hyper"] && !present["super"] {
			return "", invalid
		}
	default:
		return "", invalid
	}

	var sb strings.Builder
	for _, mod := range keyModifiers {
		if present[mod] {
			sb.WriteString(mod + "+")
		}
	}
	sb.WriteString(key)
	return sb.String(), nil
}

func isPrintableKey(key string) bool {
	r, size := utf8.DecodeRuneInString(key)
	return size == len(key) && r != utf8.RuneError && unicode.IsPrint(r) && !unicode.IsSpace(r)
}
