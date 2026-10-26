package app

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite UI golden files for explicit review")

func assertViewFits(t *testing.T, m *model) {
	t.Helper()
	lines := strings.Split(m.View().Content, "\n")
	if len(lines) > m.height {
		t.Fatalf("view height %d exceeds %d", len(lines), m.height)
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > m.width {
			t.Fatalf("line %d has width %d, exceeds %d: %q", i, width, m.width, line)
		}
	}
}

func TestSidebar_TruncatesFiftyCharacterFixture(t *testing.T) {
	m, _ := newTestModelWith(t, &fakeStore{cfg: configWith(strings.Repeat("a", 50))})
	line := ansi.Strip(m.sidebarLines(26, 1)[0])
	if line != "▸ "+strings.Repeat("a", 23)+"…" {
		t.Fatalf("truncated sidebar = %q", line)
	}
	assertViewFits(t, m)
}

func TestSidebar_UnicodeCells(t *testing.T) {
	for _, name := range []string{strings.Repeat("界", 20), strings.Repeat("a\u0301", 30), "Relógio á ç ã é ô", strings.Repeat("👩‍💻", 15)} {
		m, _ := newTestModelWith(t, &fakeStore{cfg: configWith(name)})
		line := m.sidebarLines(26, 1)[0]
		if width := ansi.StringWidth(line); width != 26 {
			t.Errorf("sidebar width = %d for %q", width, name)
		}
		if strings.Contains(line, "\uFFFD") {
			t.Errorf("broken Unicode: %q", line)
		}
		assertViewFits(t, m)
	}
}

func TestSidebar_ScrollThirtyTools(t *testing.T) {
	var names []string
	for i := range 30 {
		names = append(names, fmt.Sprintf("Ferramenta %02d", i))
	}
	m, _ := newTestModelWith(t, &fakeStore{cfg: configWith(names...)})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	for _, key := range []string{"j", "k"} {
		for range 30 {
			press(m, key)
			if m.selected < m.sidebarOffset || m.selected >= m.sidebarOffset+15 {
				t.Fatalf("selection %d outside viewport %d", m.selected, m.sidebarOffset)
			}
			lines := strings.Join(m.sidebarLines(26, 15), "\n")
			if !strings.Contains(ansi.Strip(lines), "▸ "+names[m.selected]) {
				t.Fatal("selected marker not visible")
			}
			assertViewFits(t, m)
		}
	}
}

func TestPane_DetailsAllModes(t *testing.T) {
	for _, tt := range []struct {
		mode       config.DirMode
		path, want string
	}{
		{config.ModeCurrent, "ignored", "Pasta: atual"},
		{config.ModeFixed, `D:\projetos\curso`, `Pasta: fixa (D:\projetos\curso)`},
		{config.ModeAsk, "ignored", "Pasta: perguntar ao rodar"},
	} {
		cfg := configWith("Relógio")
		cfg.Tools[0].Command = "clock-tty --24h"
		cfg.Tools[0].Dir = config.Dir{Mode: tt.mode, Path: tt.path}
		m, _ := newTestModelWith(t, &fakeStore{cfg: cfg})
		content := ansi.Strip(m.View().Content)
		for _, want := range []string{"Relógio", "clock-tty --24h", "Sem descrição", tt.want} {
			if !strings.Contains(content, want) {
				t.Errorf("missing %q in %q", want, content)
			}
		}
		if strings.Contains(content, "ignored") {
			t.Fatal("unused folder path rendered")
		}
	}
}

func TestPane_LongContentScroll(t *testing.T) {
	cfg := configWith("Longa", "Curta")
	cfg.Tools[0].Command = strings.Repeat("x", 1020) + "FIM"
	m, _ := newTestModelWith(t, &fakeStore{cfg: cfg})
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	if strings.Contains(ansi.Strip(m.View().Content), "FIM") {
		t.Fatal("long command did not wrap")
	}
	for range 10 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "FIM") {
		t.Fatal("end of long command inaccessible")
	}
	assertViewFits(t, m)
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.paneOffset == 0 {
		t.Fatal("PageUp must move one page, not reset all content")
	}
	press(m, "j")
	if m.paneOffset != 0 {
		t.Fatal("selection change must reset pane viewport")
	}
}

func TestView_ResizeSequence(t *testing.T) {
	m, _ := newTestModelWith(t, &fakeStore{cfg: configWith("a", "b")})
	press(m, "j")
	for _, size := range [][2]int{{59, 20}, {80, 15}, {60, 16}, {120, 30}, {80, 20}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		assertViewFits(t, m)
		if m.selected != 1 {
			t.Fatal("resize changed selection")
		}
		if size[0] < 60 || size[1] < 16 {
			if got := m.View().Content; got != smallTerminalMessage {
				t.Fatalf("small terminal view = %q", got)
			}
		}
	}
}

func TestView_UnknownSize(t *testing.T) {
	m, _ := newTestModel(t)
	m.width, m.height = 0, 0
	if v := m.View(); v.Content != "" || !v.AltScreen {
		t.Fatalf("unknown-size view = %+v", v)
	}
}

func TestHelp_FourContexts(t *testing.T) {
	m, _ := newTestModel(t)
	press(m, "?")
	content := ansi.Strip(m.View().Content)
	for _, want := range []string{"Sidebar", "Ferramenta em execução", "Prompt de pasta", "Formulário", "k/j", "ctrl+c", "K/J", "shift+tab", "n/esc"} {
		if !strings.Contains(content, want) {
			t.Errorf("help missing %q: %s", want, content)
		}
	}
}

func TestHelp_UsesReloadedReleaseKey(t *testing.T) {
	cfg := config.Empty()
	cfg.Keys.Release = "ctrl+]"
	store := &fakeStore{cfg: config.Empty(), results: []fakeLoad{{cfg: cfg}}}
	m, _ := newTestModelWith(t, store)
	press(m, "?")
	m.Update(press(m, "r")())
	content := ansi.Strip(m.View().Content)
	if !m.helpOpen || !strings.Contains(content, "ctrl+]") || strings.Contains(content, `ctrl+\`) {
		t.Fatalf("help did not use reloaded key: %s", content)
	}
	if got := m.footer("session", 60); got != "ctrl+] encerrar ferramenta" {
		t.Fatalf("session footer = %q", got)
	}
}

func TestHelp_ScrollAtMinimumSize(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	press(m, "?")
	if strings.Contains(ansi.Strip(m.View().Content), "cancelar remoção") {
		t.Fatal("test needs help longer than viewport")
	}
	for range 10 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if content := ansi.Strip(m.View().Content); !strings.Contains(content, "n/esc:") || !strings.Contains(content, "remoção") {
		t.Fatalf("last help entry inaccessible: %s", content)
	}
	assertViewFits(t, m)
	for range 100 {
		press(m, "k")
	}
	if m.helpOffset != 0 || !strings.Contains(ansi.Strip(m.View().Content), "Ajuda") {
		t.Fatal("help cannot return to first line")
	}
}

func TestView_FooterFits(t *testing.T) {
	m, _ := newTestModel(t)
	for _, context := range []string{"sidebar", "session", "folder", "form"} {
		for _, width := range []int{60, 80, 120} {
			footer := m.footer(context, width)
			if strings.Contains(footer, "\n") || ansi.StringWidth(footer) > width {
				t.Errorf("%s footer does not fit %d: %q", context, width, footer)
			}
		}
	}
	if got := m.footer("sidebar", 120); got != "↑↓ navegar · enter rodar · a add · e editar · d remover · ? ajuda · q sair" {
		t.Fatalf("full footer = %q", got)
	}
	if got := m.footer("sidebar", 60); got != "↑↓ navegar · enter rodar · ? ajuda · q sair" {
		t.Fatalf("compact footer = %q", got)
	}
}

func TestPane_ConfigErrorPresentation(t *testing.T) {
	store := readOnlyStore(12)
	store.err.Message = "bad \x1b[2J control"
	store.err.Path = ""
	m, _ := newTestModelWith(t, store)
	content := ansi.Strip(m.View().Content)
	for _, want := range []string{"linha 12", "bad �[2J control", `Arquivo: C:\cfg\config.yaml`, readOnlyHint} {
		if !strings.Contains(content, want) {
			t.Errorf("diagnostic missing %q: %q", want, content)
		}
	}
	if strings.Contains(content, emptyToolList) || store.err.Message != "bad \x1b[2J control" {
		t.Fatal("error was hidden or mutated")
	}
}

// Run go test ./internal/app -run '^TestView_Golden$' -update-golden to
// deliberately replace expected frames, then inspect their diff before committing.
func TestView_Golden(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
		store         *fakeStore
		help          bool
	}{
		{"sidebar_details", 120, 30, &fakeStore{cfg: configWith("Relógio", "Snake", "Curso")}, false},
		{"empty", 120, 30, &fakeStore{cfg: config.Empty()}, false},
		{"too_small_width", 59, 20, &fakeStore{cfg: configWith("Relógio")}, false},
		{"too_small_height", 80, 15, &fakeStore{cfg: configWith("Relógio")}, false},
		{"help", 120, 30, &fakeStore{cfg: config.Empty()}, true},
		{"read_only", 120, 30, readOnlyStore(12), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModelWith(t, tt.store)
			m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			if tt.help {
				press(m, "?")
			}
			content := m.View().Content
			if tt.name == "sidebar_details" && (!strings.Contains(content, "\x1b[1") || !strings.Contains(content, "▸")) {
				t.Fatal("reference frame lost bold styling or selected marker")
			}
			// Quoting each line makes ANSI and trailing padding visible in reviews.
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				lines[i] = strconv.Quote(line)
			}
			got := strings.Join(lines, "\n") + "\n"
			path := filepath.Join("testdata", "golden", tt.name+".golden")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
				t.Errorf("frame differs from %s; inspect output before explicitly updating\n%s", path, got)
			}
			assertViewFits(t, m)
		})
	}
}
