package config

import (
	"github.com/goccy/go-yaml"
)

// Marshal writes cfg as YAML in the layout users write by hand: version,
// keys, then tools, two space indentation and LF line endings. Comments from
// the loaded file are not preserved.
func Marshal(cfg Config) ([]byte, error) {
	out := cfg.Clone()
	out.Version = CurrentVersion
	if out.Keys.Release == "" {
		out.Keys.Release = DefaultReleaseKey
	}
	for i := range out.Tools {
		if out.Tools[i].Dir.Mode == "" {
			out.Tools[i].Dir.Mode = ModeCurrent
		}
	}
	// Single quotes keep backslashes literal, so a Windows path is written as
	// 'D:\projetos\curso' instead of the escaped "D:\\projetos\\curso".
	return yaml.MarshalWithOptions(out, yaml.Indent(2), yaml.IndentSequence(true), yaml.UseSingleQuote(true))
}
