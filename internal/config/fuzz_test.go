package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func FuzzParse(f *testing.F) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.yaml"))
	if err != nil {
		f.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	for _, seed := range []string{
		"\xEF\xBB\xBFversion: 1\n",
		"version: 1\ntools:\n\t- name: a\n",
		"version: 1\ntools:\n  - &t {name: a, command: b}\n  - *t\n",
		"version: 1\ntools: [[[[[[[[[[[[[[[[[[[[]]]]]]]]]]]]]]]]]]]]\n",
		"version: 1\ntools:\n  - name: \"null\"\n    command: \"~\"\n    description: \"123\"\n",
		"version: 1\nkeys:\n  release: \"ctrl++\"\n",
		"version: 1\ntools:\n  - name: ' a '\n    command: '\"x y\" z'\n    dir: {mode: fixed, path: 'C:\\'}\n",
		"version: 1\n---\n",
		"{version: 1, tools: [{name: a, command: b, dir: {mode: ask}}]}",
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		cfg, err := Parse(data, "config.yaml")
		if err != nil {
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("Parse() error %T is not *Error: %v", err, err)
			}
			if e.Kind != KindSyntax && e.Kind != KindValidation && e.Kind != KindRead {
				t.Fatalf("Parse() error kind = %q", e.Kind)
			}
			return
		}

		out, err := Marshal(cfg)
		if err != nil {
			t.Fatalf("Marshal() of an accepted config failed: %v", err)
		}
		back, err := Parse(out, "config.yaml")
		if err != nil {
			t.Fatalf("Parse(Marshal()) error = %v\ninput: %q\nmarshaled: %q", err, data, out)
		}
		if !reflect.DeepEqual(back, cfg) {
			t.Fatalf("round trip changed the config\nbefore: %+v\nafter:  %+v\nmarshaled: %q", cfg, back, out)
		}
	})
}
