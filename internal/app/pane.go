package app

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
)

func visibleText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) && r != '\n' {
			return '\uFFFD'
		}
		return r
	}, s)
}

func (m *model) paneLines(width int) []string {
	var content string
	switch {
	case m.loadErr != nil:
		path := m.loadErr.Path
		if path == "" {
			path = m.store.Path()
		}
		content = m.styles.error.Render(visibleText(m.loadErr.Error())) + "\nArquivo: " + visibleText(path) + "\n\n" + readOnlyHint
	default:
		tool, ok := m.selectedTool()
		if !ok {
			content = emptyToolList
			break
		}
		description := tool.Description
		if description == "" {
			description = "Sem descrição"
		}
		folder := "Pasta: atual"
		switch tool.Dir.Mode {
		case config.ModeFixed:
			folder = "Pasta: fixa (" + visibleText(tool.Dir.Path) + ")"
		case config.ModeAsk:
			folder = "Pasta: perguntar ao rodar"
		}
		content = m.styles.title.Render(visibleText(tool.Name)) + "\n\nComando: " + visibleText(tool.Command) + "\nDescrição: " + visibleText(description) + "\n" + folder
	}
	return strings.Split(ansi.Wrap(content, width, ""), "\n")
}

func (m *model) scrollPane(pages int) {
	l := calculateLayout(m.width, m.height)
	if !l.valid {
		return
	}
	m.paneOffset += pages * l.paneInner.height
	m.clampOffsets()
}
