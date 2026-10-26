package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const smallTerminalMessage = "Aumente o terminal (mínimo 60×16)"

func fitLine(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

func viewport(lines []string, offset, width, height int) string {
	rows := make([]string, height)
	for row := range height {
		line := ""
		if i := offset + row; i >= 0 && i < len(lines) {
			line = lines[i]
		}
		rows[row] = fitLine(line, width)
	}
	return strings.Join(rows, "\n")
}

func (m *model) View() tea.View {
	// The first render remains the measurement point for startup latency.
	if !m.firstFrameLogged {
		m.firstFrameLogged = true
		m.logger.Debug("first frame", "elapsed_ms", time.Since(m.start).Milliseconds())
	}
	v := tea.NewView("")
	v.AltScreen = true
	if m.width <= 0 || m.height <= 0 {
		return v
	}
	l := calculateLayout(m.width, m.height)
	if !l.valid {
		v.Content = viewport(strings.Split(ansi.Hardwrap(smallTerminalMessage, m.width, true), "\n"), 0, m.width, min(m.height, len([]rune(smallTerminalMessage))))
		v.Content = strings.TrimRight(v.Content, " \n")
		return v
	}
	sidebar := m.styles.border.Render(strings.Join(m.sidebarLines(l.sidebarInner.width, l.sidebarInner.height), "\n"))
	lines, offset := m.paneLines(l.paneInner.width), m.paneOffset
	if m.helpOpen {
		lines, offset = m.helpLines(l.paneInner.width), m.helpOffset
	}
	pane := m.styles.border.Render(viewport(lines, offset, l.paneInner.width, l.paneInner.height))
	foot := m.footer("sidebar", m.width)
	if m.helpOpen {
		foot = "↑↓ rolar · ?/esc voltar · q sair"
	}
	v.Content = strings.Join([]string{
		fitLine(m.styles.title.Render(title), m.width),
		lipgloss.JoinHorizontal(lipgloss.Top, sidebar, pane),
		fitLine(m.styles.status.Render(visibleText(m.status)), m.width),
		fitLine(m.styles.status.Render(foot), m.width),
	}, "\n")
	return v
}
