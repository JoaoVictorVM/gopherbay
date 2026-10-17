package cmdline

import (
	"errors"
	"slices"
	"testing"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []string
	}{
		{"plain words", "clock-tty --24h", []string{"clock-tty", "--24h"}},
		{"repeated spaces and tabs", "  a \t\t b   c\t", []string{"a", "b", "c"}},
		{"double quotes", `say "hello world"`, []string{"say", "hello world"}},
		{"single quotes", `say 'hello world'`, []string{"say", "hello world"}},
		{"adjacent quoted and plain text", `a"b c"d`, []string{"ab cd"}},
		{"other quote inside double quotes", `echo "it's"`, []string{"echo", "it's"}},
		{"other quote inside single quotes", `git log --format='%h "%s"'`, []string{"git", "log", `--format=%h "%s"`}},
		{"windows path unquoted", `C:\tools\clock.exe --24h`, []string{`C:\tools\clock.exe`, "--24h"}},
		{"windows path quoted", `"C:\Program Files\x\x.exe" -v`, []string{`C:\Program Files\x\x.exe`, "-v"}},
		{"trailing backslash", `dir C:\`, []string{"dir", `C:\`}},
		{"empty quoted argument", `prog "" x`, []string{"prog", "", "x"}},
		{"only empty quotes", `""`, []string{""}},
		{"accents", "relógio --cor ação", []string{"relógio", "--cor", "ação"}},
		{"empty input", "", []string{}},
		{"blank input", " \t ", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split(tt.line)
			if err != nil {
				t.Fatalf("Split(%q) error = %v", tt.line, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Split(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestSplit_UnclosedQuote(t *testing.T) {
	for _, line := range []string{`echo "abc`, `echo 'abc`, `"`, `a 'b "c' "d`} {
		t.Run(line, func(t *testing.T) {
			got, err := Split(line)
			if !errors.Is(err, ErrUnclosedQuote) {
				t.Errorf("Split(%q) = %q, %v, want ErrUnclosedQuote", line, got, err)
			}
		})
	}
}

func TestSplit_NoShellOperators(t *testing.T) {
	got, err := Split("a ; b > c && d | e")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", ";", "b", ">", "c", "&&", "d", "|", "e"}
	if !slices.Equal(got, want) {
		t.Errorf("Split() = %q, want %q", got, want)
	}
}
