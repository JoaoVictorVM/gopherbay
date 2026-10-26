package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type shortcut struct {
	key, description, helpKey string
	bar, compact              bool
}

type shortcutGroup struct {
	context, title string
	keys           []shortcut
}

func (m *model) shortcuts() []shortcutGroup {
	return []shortcutGroup{
		{"sidebar", "Sidebar", []shortcut{
			{"↑↓", "navegar", "↑/↓, k/j", true, true},
			{"enter", "rodar", "", true, true},
			{"a", "add", "", true, false},
			{"e", "editar", "", true, false},
			{"d", "remover", "", true, false},
			{"K/J", "reordenar", "", false, false},
			{"r", "recarregar", "", false, false},
			{"?", "ajuda", "", true, true},
			{"q", "sair", "q/ctrl+c", true, true},
			{"pgup/pgdown", "rolar detalhes", "", false, false},
		}},
		{"session", "Ferramenta em execução", []shortcut{
			{visibleText(m.cfg.Keys.Release), "encerrar ferramenta", "", true, true},
			{"demais teclas", "enviar à ferramenta", "", false, false},
		}},
		{"folder", "Prompt de pasta", []shortcut{
			{"enter", "confirmar", "", true, true},
			{"tab", "completar", "", true, true},
			{"esc", "cancelar", "", true, true},
		}},
		{"form", "Formulário", []shortcut{
			{"tab", "próximo", "", true, true},
			{"shift+tab", "anterior", "", true, false},
			{"←/→", "modo de pasta", "", false, false},
			{"enter", "salvar", "", true, true},
			{"esc", "cancelar", "", true, true},
			{"s", "confirmar remoção", "", false, false},
			{"n/esc", "cancelar remoção", "", false, false},
		}},
	}
}

func (m *model) footer(context string, width int) string {
	for _, group := range m.shortcuts() {
		if group.context != context {
			continue
		}
		var full, compact []string
		for _, key := range group.keys {
			item := key.key + " " + key.description
			if key.bar {
				full = append(full, item)
				if key.compact {
					compact = append(compact, item)
				}
			}
		}
		text := strings.Join(full, " · ")
		if ansi.StringWidth(text) > width {
			text = strings.Join(compact, " · ")
		}
		return ansi.Truncate(text, width, "…")
	}
	return ""
}

func (m *model) helpLines(width int) []string {
	lines := []string{m.styles.title.Render("Ajuda"), ""}
	for _, group := range m.shortcuts() {
		lines = append(lines, m.styles.title.Render(group.title))
		var entries []string
		for _, key := range group.keys {
			name := key.key
			if key.helpKey != "" {
				name = key.helpKey
			}
			entries = append(entries, name+": "+key.description)
		}
		lines = append(lines, strings.Join(entries, " · "), "")
	}
	return strings.Split(ansi.Wrap(strings.Join(lines, "\n"), width, ""), "\n")
}

func (m *model) handleHelpKey(key string) bool {
	if key == "?" {
		m.helpOpen = !m.helpOpen
		if m.helpOpen {
			m.helpOffset = 0
		}
		return true
	}
	if !m.helpOpen {
		return false
	}
	l := calculateLayout(m.width, m.height)
	switch key {
	case "esc":
		m.helpOpen = false
	case "up", "k":
		m.helpOffset--
	case "down", "j":
		m.helpOffset++
	case "pgup":
		m.helpOffset -= l.paneInner.height
	case "pgdown":
		m.helpOffset += l.paneInner.height
	}
	m.clampOffsets()
	return true
}
