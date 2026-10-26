package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
)

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
