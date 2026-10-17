package app

import (
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
)

const (
	title         = "gopherbay"
	emptyToolList = "Nenhuma ferramenta cadastrada."
	defaultStatus = "Aperte q para sair."
)

// configStore is the part of *config.Store the interface uses, so tests can
// drive the model with a fake store.
type configStore interface {
	Load() error
	Config() config.Config
	LoadError() *config.Error
	Path() string
}

type model struct {
	logger           *slog.Logger
	start            time.Time
	firstFrameLogged bool
	width, height    int

	store configStore
	cfg   config.Config
}

// newModel expects store to be loaded already, so the first frame shows the config.
func newModel(logger *slog.Logger, start time.Time, store configStore) *model {
	return &model{logger: logger, start: start, store: store, cfg: store.Config()}
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

// View draws a provisional screen until the sidebar replaces it.
func (m *model) View() tea.View {
	// The first render is the measurement point for the startup time goal.
	if !m.firstFrameLogged {
		m.firstFrameLogged = true
		m.logger.Debug("first frame", "elapsed_ms", time.Since(m.start).Milliseconds())
	}

	lines := []string{title, ""}
	if len(m.cfg.Tools) == 0 {
		lines = append(lines, emptyToolList)
	}
	for _, tool := range m.cfg.Tools {
		lines = append(lines, tool.Name)
	}
	lines = append(lines, "", defaultStatus)

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}
