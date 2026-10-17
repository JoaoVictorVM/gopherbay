package config

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

var (
	duplicateKeyPattern = regexp.MustCompile(`^mapping key "(.*)" already defined`)
	unknownFieldPattern = regexp.MustCompile(`^unknown field "(.*)"`)
)

// rawConfig tells an absent keys.release apart from an empty one.
type rawConfig struct {
	Version int     `yaml:"version"`
	Keys    rawKeys `yaml:"keys"`
	Tools   []Tool  `yaml:"tools"`
}

type rawKeys struct {
	Release *string `yaml:"release"`
}

// configParser holds the format version and the migrations, so tests can
// exercise the migration hook with versions that do not exist yet.
type configParser struct {
	current    int
	migrations map[int]migrateFunc
	logger     *slog.Logger
}

func defaultParser() configParser {
	return newParser(slog.New(slog.DiscardHandler))
}

// Parse turns the bytes of a config file into a Config. Any problem is
// returned as an *Error positioned on the line to fix. An empty file, or one
// with only comments, is an empty valid config.
func Parse(data []byte, path string) (Config, error) {
	return defaultParser().parse(data, path)
}

func (p configParser) parse(data []byte, path string) (Config, error) {
	if len(data) > MaxFileSize {
		return Config{}, &Error{Kind: KindRead, Path: path, Rule: RuleFileTooLarge, Message: "o arquivo tem mais de 1 MiB", Violations: 1}
	}
	// Some Windows editors save UTF-8 with a BOM.
	data = bytes.TrimPrefix(data, utf8BOM)

	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return Config{}, yamlError(err, path)
	}

	var bodies []ast.Node
	for _, doc := range file.Docs {
		if doc.Body != nil {
			bodies = append(bodies, doc.Body)
		}
	}
	switch {
	case len(bodies) == 0:
		return Empty(), nil
	case len(bodies) > 1:
		return Config{}, validationError(path, lineOf(bodies[1]), RuleMultipleDocuments, "o arquivo deve ter um único documento YAML")
	}
	body := bodies[0]
	if body.Type() != ast.MappingType && body.Type() != ast.MappingValueType {
		return Config{}, validationError(path, lineOf(body), RuleNotAMapping, "o arquivo deve ser um mapeamento com version, keys e tools")
	}

	// The version is checked before the strict decode so a file written by a
	// newer gbay reports its version instead of an unknown field it added.
	version, err := p.checkVersion(body, path)
	if err != nil {
		return Config{}, err
	}
	if version < p.current {
		if err := p.migrate(body, version, lineOf(field(body, "version")), path); err != nil {
			return Config{}, err
		}
	}

	var raw rawConfig
	if err := yaml.NodeToValue(body, &raw, yaml.DisallowUnknownField()); err != nil {
		return Config{}, yamlError(err, path)
	}

	cfg := normalize(raw)
	if err := validateFile(&cfg, raw.Keys.Release != nil, body, path); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (p configParser) checkVersion(body ast.Node, path string) (int, error) {
	node := field(body, "version")
	if node == nil {
		return 0, validationError(path, 1, RuleVersionMissing, fmt.Sprintf("o campo version é obrigatório (use version: %d)", p.current))
	}
	intNode, ok := node.(*ast.IntegerNode)
	if !ok {
		return 0, validationError(path, lineOf(node), RuleVersionInvalid, "version deve ser um número inteiro")
	}
	v, err := strconv.Atoi(fmt.Sprint(intNode.Value))
	if err != nil {
		return 0, validationError(path, lineOf(node), RuleVersionInvalid, "version deve ser um número inteiro")
	}
	switch {
	case v > p.current:
		return 0, validationError(path, lineOf(node), RuleVersionUnsupported, fmt.Sprintf("Versão %d do config não é suportada por esta versão do gbay", v))
	case v < 0:
		return 0, validationError(path, lineOf(node), RuleVersionTooOld, fmt.Sprintf("Versão %d do config é inválida (use version: %d)", v, p.current))
	}
	return v, nil
}

func normalize(raw rawConfig) Config {
	cfg := Config{Version: CurrentVersion, Keys: Keys{Release: DefaultReleaseKey}, Tools: normalizeTools(raw.Tools)}
	if raw.Keys.Release != nil {
		cfg.Keys.Release = *raw.Keys.Release
	}
	return cfg
}

func normalizeTools(tools []Tool) []Tool {
	out := make([]Tool, len(tools))
	for i, t := range tools {
		t.Name = strings.TrimSpace(t.Name)
		t.Command = strings.TrimSpace(t.Command)
		t.Description = strings.TrimSpace(t.Description)
		t.Dir.Path = strings.TrimSpace(t.Dir.Path)
		if t.Dir.Mode == "" {
			t.Dir.Mode = ModeCurrent
		}
		out[i] = t
	}
	return out
}

type located struct {
	line int
	rule Rule
	msg  string
}

// validateFile runs every rule on cfg, normalizes the release key in place and
// reports the violation on the lowest line, so the user fixes the file top down.
func validateFile(cfg *Config, hasRelease bool, body ast.Node, path string) error {
	var found []located
	if hasRelease {
		key, err := ValidateReleaseKey(cfg.Keys.Release)
		if err != nil {
			found = append(found, located{lineAt(body, "keys", "release"), RuleReleaseKeyInvalid, err.Error()})
		}
		cfg.Keys.Release = key
	}
	for _, v := range ValidateTools(cfg.Tools) {
		segments := []string{"tools", strconv.Itoa(v.Index)}
		if v.Field != "" {
			segments = append(segments, strings.Split(v.Field, ".")...)
		}
		found = append(found, located{lineAt(body, segments...), v.Rule, v.Message})
	}
	if len(found) == 0 {
		return nil
	}

	first := slices.MinFunc(found, func(a, b located) int { return a.line - b.line })
	e := validationError(path, first.line, first.rule, first.msg)
	e.Violations = len(found)
	return e
}

func validationError(path string, line int, rule Rule, msg string) *Error {
	return &Error{Kind: KindValidation, Path: path, Line: line, Rule: rule, Message: msg, Violations: 1}
}

// yamlError translates a parser or decoder error. Duplicate and unknown keys
// are rules of the config format and get pt-BR messages; the other syntax and
// type errors keep the parser's own text.
func yamlError(err error, path string) *Error {
	var ye yaml.Error
	if !errors.As(err, &ye) {
		return &Error{Kind: KindSyntax, Path: path, Rule: RuleSyntax, Message: err.Error(), Violations: 1, Err: err}
	}
	line := 0
	if tk := ye.GetToken(); tk != nil && tk.Position != nil {
		line = tk.Position.Line
	}
	msg := ye.GetMessage()

	e := &Error{Kind: KindSyntax, Path: path, Line: line, Rule: RuleSyntax, Message: msg, Violations: 1, Err: err}
	if m := duplicateKeyPattern.FindStringSubmatch(msg); m != nil {
		e.Kind, e.Rule, e.Message = KindValidation, RuleDuplicateKey, fmt.Sprintf("campo %q repetido", m[1])
		return e
	}
	var unknown *yaml.UnknownFieldError
	if errors.As(err, &unknown) {
		name := msg
		if m := unknownFieldPattern.FindStringSubmatch(msg); m != nil {
			name = m[1]
		}
		e.Kind, e.Rule, e.Message = KindValidation, RuleUnknownField, fmt.Sprintf("campo desconhecido %q", name)
		return e
	}
	var dup *yaml.DuplicateKeyError
	if errors.As(err, &dup) {
		e.Kind, e.Rule, e.Message = KindValidation, RuleDuplicateKey, "campo repetido"
		return e
	}
	var syntax *yaml.SyntaxError
	if !errors.As(err, &syntax) {
		e.Kind, e.Rule = KindValidation, RuleType
	}
	return e
}

// lineAt returns the line of the node at the path made of mapping keys and
// sequence indexes, falling back to the closest existing parent, so a missing
// field is reported on the line of the item that lacks it.
func lineAt(body ast.Node, segments ...string) int {
	line := lineOf(body)
	node := body
	for _, seg := range segments {
		next := child(node, seg)
		if next == nil {
			break
		}
		node = next
		line = lineOf(node)
	}
	return line
}

func child(node ast.Node, seg string) ast.Node {
	if seq, ok := node.(*ast.SequenceNode); ok {
		i, err := strconv.Atoi(seg)
		if err != nil || i < 0 || i >= len(seq.Values) {
			return nil
		}
		return seq.Values[i]
	}
	return field(node, seg)
}

func field(node ast.Node, key string) ast.Node {
	for _, mv := range mappingValues(node) {
		if tk := mv.Key.GetToken(); tk != nil && tk.Value == key {
			return mv.Value
		}
	}
	return nil
}

func mappingValues(node ast.Node) []*ast.MappingValueNode {
	switch n := node.(type) {
	case *ast.MappingNode:
		return n.Values
	case *ast.MappingValueNode:
		return []*ast.MappingValueNode{n}
	}
	return nil
}

func lineOf(node ast.Node) int {
	if mv := mappingValues(node); len(mv) > 0 {
		node = mv[0].Key
	}
	if tk := node.GetToken(); tk != nil && tk.Position != nil {
		return tk.Position.Line
	}
	return 0
}
