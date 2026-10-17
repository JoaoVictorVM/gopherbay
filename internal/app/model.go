package app

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
)

const placeholderContent = "gopherbay\n\nAperte q para sair."

type model struct {
	logger           *slog.Logger
	start            time.Time
	firstFrameLogged bool
	width, height    int
}

func newModel(logger *slog.Logger, start time.Time) *model {
	return &model{logger: logger, start: start}
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.logger.Debug("window resized", "cols", msg.Width, "rows", msg.Height)
	}
	return m, nil
}

func (m *model) View() tea.View {
	// The first render is the measurement point for the startup time goal.
	if !m.firstFrameLogged {
		m.firstFrameLogged = true
		m.logger.Debug("first frame", "elapsed_ms", time.Since(m.start).Milliseconds())
	}
	v := tea.NewView(placeholderContent)
	v.AltScreen = true
	return v
}
