package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
)

func (m *model) selectedTool() (config.Tool, bool) {
	if m.loadErr != nil || m.selected < 0 || m.selected >= len(m.cfg.Tools) {
		return config.Tool{}, false
	}
	return m.cfg.Tools[m.selected], true
}

func (m *model) moveSelection(delta int) {
	if _, ok := m.selectedTool(); !ok {
		return
	}
	next := max(0, min(m.selected+delta, len(m.cfg.Tools)-1))
	if next != m.selected {
		m.selected, m.paneOffset = next, 0
		m.clampOffsets()
	}
}

func (m *model) clampOffsets() {
	l := calculateLayout(m.width, m.height)
	if !l.valid {
		return
	}
	rows := l.sidebarInner.height
	m.sidebarOffset = max(0, min(m.sidebarOffset, len(m.cfg.Tools)-rows))
	if _, ok := m.selectedTool(); ok {
		if m.selected < m.sidebarOffset {
			m.sidebarOffset = m.selected
		} else if m.selected >= m.sidebarOffset+rows {
			m.sidebarOffset = m.selected - rows + 1
		}
	}
	m.paneOffset = max(0, min(m.paneOffset, len(m.paneLines(l.paneInner.width))-l.paneInner.height))
	m.helpOffset = max(0, min(m.helpOffset, len(m.helpLines(l.paneInner.width))-l.paneInner.height))
}

func (m *model) sidebarLines(width, height int) []string {
	lines := make([]string, height)
	for row := range height {
		index := m.sidebarOffset + row
		if m.loadErr != nil || index >= len(m.cfg.Tools) {
			lines[row] = strings.Repeat(" ", width)
			continue
		}
		name := strings.ReplaceAll(visibleText(m.cfg.Tools[index].Name), "\n", " ")
		prefix := "  "
		style := m.styles.text
		if index == m.selected {
			prefix, style = "▸ ", m.styles.selected
		}
		line := fitLine(prefix+ansi.Truncate(name, width-2, "…"), width)
		lines[row] = style.Render(line)
	}
	return lines
}
