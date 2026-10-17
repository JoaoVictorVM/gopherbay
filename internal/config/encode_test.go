package config

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func goldenConfig() Config {
	return Config{
		Version: 1,
		Keys:    Keys{Release: DefaultReleaseKey},
		Tools: []Tool{
			{Name: "Relógio", Command: "clock-tty --24h", Description: "Relógio no terminal", Dir: Dir{Mode: ModeCurrent}},
			{Name: "Controle de curso", Command: "curso", Dir: Dir{Mode: ModeFixed, Path: `D:\projetos\curso`}},
			{Name: "Uso: barra", Command: `"C:\Program Files\usage\usage.exe" --bar`, Description: "Mostra o uso # não é comentário, d'água", Dir: Dir{Mode: ModeAsk}},
			{Name: "yes", Command: "git log --format='%h \"%s\"'", Dir: Dir{Mode: ModeCurrent}},
		},
	}
}

func TestMarshal_Golden(t *testing.T) {
	got, err := Marshal(goldenConfig())
	if err != nil {
		t.Fatal(err)
	}

	golden := filepath.Join("testdata", "golden", "saved.yaml")
	if *update {
		if err := os.MkdirAll(filepath.Dir(golden), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	// A checkout with autocrlf turns the golden file into CRLF.
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Errorf("Marshal() =\n%s\nwant\n%s", got, want)
	}
	if bytes.Contains(got, []byte("\r")) {
		t.Error("output contains CR")
	}
	if bytes.HasPrefix(got, utf8BOM) {
		t.Error("output starts with a BOM")
	}
}

func TestMarshal_RoundTrip(t *testing.T) {
	cases := map[string]Config{"golden": goldenConfig(), "empty": Empty()}
	for _, name := range []string{"valid_five_tools.yaml", "valid_minimal.yaml", "valid_all_modes.yaml"} {
		cfg, err := Parse(readFixture(t, name), name)
		if err != nil {
			t.Fatal(err)
		}
		cases[name] = cfg
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			data, err := Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			back, err := Parse(data, "config.yaml")
			if err != nil {
				t.Fatalf("Parse(Marshal()) error = %v\n%s", err, data)
			}
			if !reflect.DeepEqual(back, cfg) {
				t.Errorf("round trip = %+v, want %+v\n%s", back, cfg, data)
			}
		})
	}
}

func TestMarshal_OmitsEmptyOptionalFields(t *testing.T) {
	data, err := Marshal(Config{Tools: []Tool{{Name: "a", Command: "b"}}})
	if err != nil {
		t.Fatal(err)
	}

	s := string(data)
	for _, unwanted := range []string{"description", "path"} {
		if strings.Contains(s, unwanted) {
			t.Errorf("output contains %q:\n%s", unwanted, s)
		}
	}
	for _, wanted := range []string{"version: 1", "release:", "mode: current"} {
		if !strings.Contains(s, wanted) {
			t.Errorf("output lacks %q:\n%s", wanted, s)
		}
	}
}
