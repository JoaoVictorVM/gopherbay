// Command prototype is a throwaway check that the embedded terminal stack
// (Bubble Tea v2, xpty and x/vt) keeps real tools faithful inside a pane.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	sidebarWidth = 28
	minPaneW     = 20
	minPaneH     = 5

	statusIdle = "↑↓ mover · enter executar · q sair"
)

type entry struct {
	key     string
	label   string
	command string
}

type rect struct {
	x, y, w, h int
}

type model struct {
	entries  []entry
	selected int
	width    int
	height   int
	status   string
}

func main() {
	relogio := flag.String("relogio", "clock-tty --24h", "linha de comando do relógio")
	jogo := flag.String("jogo", "snake", "linha de comando do jogo")
	uso := flag.String("uso", "usage-bar", "linha de comando da barra de uso")
	flag.Parse()

	m := &model{
		entries: []entry{
			{key: "relogio", label: "Relógio", command: *relogio},
			{key: "jogo", label: "Jogo", command: *jogo},
			{key: "uso", label: "Barra de uso", command: *uso},
		},
		status: statusIdle,
	}

	if _, err := tea.NewProgram(m, tea.WithFPS(60)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return m, m.sidebarKey(msg)
	}
	return m, nil
}

func (m *model) sidebarKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
			m.status = statusIdle
		}
	case "down", "j":
		if m.selected < len(m.entries)-1 {
			m.selected++
			m.status = statusIdle
		}
	}
	return nil
}

func (m *model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true

	pane, ok := paneRect(m.width, m.height)
	if !ok {
		v.SetContent("Aumente o terminal")
		return v
	}

	lines := joinColumns(m.sidebarLines(pane.h), sidebarWidth, m.paneLines(), pane.w, pane.h)
	lines = append(lines, padLine(m.status, m.width))
	v.SetContent(strings.Join(lines, "\n"))
	return v
}

func (m *model) sidebarLines(height int) []string {
	content := []string{ansi.Style{}.Bold().Styled(" gopherbay · protótipo"), ""}
	for i, e := range m.entries {
		if i != m.selected {
			content = append(content, "   "+e.label)
			continue
		}
		content = append(content,
			ansi.Style{}.Reverse(true).Styled(padLine(" ▸ "+e.label, sidebarWidth-1)),
			ansi.Style{}.Faint().Styled(ansi.Truncate("   "+e.command, sidebarWidth-1, "…")),
		)
	}

	lines := make([]string, height)
	for i := range lines {
		var s string
		if i < len(content) {
			s = content[i]
		}
		lines[i] = padLine(s, sidebarWidth-1) + "│"
	}
	return lines
}

func (m *model) paneLines() []string {
	return nil
}

// paneRect reserves the last row for the status line, so the pane spans every
// row above it to the right of the sidebar.
func paneRect(width, height int) (rect, bool) {
	r := rect{x: sidebarWidth, y: 0, w: width - sidebarWidth, h: height - 1}
	return r, r.w >= minPaneW && r.h >= minPaneH
}

// padLine fits an ANSI line into exactly width cells. The style reset keeps an
// unterminated SGR from the tool bleeding into the next column or row.
func padLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, "")
	}
	if strings.ContainsRune(s, '\x1b') {
		s += ansi.ResetStyle
	}
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}

func joinColumns(left []string, leftW int, right []string, rightW int, height int) []string {
	lines := make([]string, height)
	for i := range lines {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines[i] = padLine(l, leftW) + padLine(r, rightW)
	}
	return lines
}
