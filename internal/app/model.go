package app

import (
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
)

const (
	title          = "gopherbay"
	emptyToolList  = "Nenhuma ferramenta cadastrada."
	defaultStatus  = "Aperte r para recarregar ou q para sair."
	readOnlyHint   = "Corrija o arquivo e aperte r para recarregar"
	readOnlyStatus = "Config inválido: corrija o arquivo e aperte r para recarregar"
	reloadedStatus = "Config recarregado"
)

// configStore is the part of *config.Store the interface uses, so tests can
// drive the model with a fake store.
type configStore interface {
	Load() error
	Config() config.Config
	LoadError() *config.Error
	Path() string
}

// configLoadedMsg carries the result of a reload back into Update, so the
// file is read outside the event loop and the model only changes in Update.
type configLoadedMsg struct {
	cfg config.Config
	err *config.Error
}

type model struct {
	logger           *slog.Logger
	start            time.Time
	firstFrameLogged bool
	width, height    int

	store   configStore
	cfg     config.Config
	loadErr *config.Error
	status  string
}

// newModel expects store to be loaded already, so the first frame shows the config.
func newModel(logger *slog.Logger, start time.Time, store configStore) *model {
	return &model{logger: logger, start: start, store: store, cfg: store.Config(), loadErr: store.LoadError()}
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
		case "r":
			return m, m.reload()
		case "a", "e", "d", "K", "J":
			// These edit the tool list, which would rewrite a file the user still has to fix.
			if m.loadErr != nil {
				m.status = readOnlyStatus
			}
		}
	case configLoadedMsg:
		m.cfg, m.loadErr = msg.cfg, msg.err
		m.status = ""
		if msg.err == nil {
			m.status = reloadedStatus
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.logger.Debug("window resized", "cols", msg.Width, "rows", msg.Height)
	}
	return m, nil
}

func (m *model) reload() tea.Cmd {
	store := m.store
	return func() tea.Msg {
		_ = store.Load()
		return configLoadedMsg{cfg: store.Config(), err: store.LoadError()}
	}
}

// View draws a provisional screen until the sidebar replaces it.
func (m *model) View() tea.View {
	// The first render is the measurement point for the startup time goal.
	if !m.firstFrameLogged {
		m.firstFrameLogged = true
		m.logger.Debug("first frame", "elapsed_ms", time.Since(m.start).Milliseconds())
	}

	lines := []string{title, ""}
	switch {
	case m.loadErr != nil:
		lines = append(lines, m.loadErr.Error(), "Arquivo: "+m.loadErr.Path, "", readOnlyHint)
	case len(m.cfg.Tools) == 0:
		lines = append(lines, emptyToolList)
	default:
		for _, tool := range m.cfg.Tools {
			lines = append(lines, tool.Name)
		}
	}

	status := m.status
	if status == "" {
		status = defaultStatus
	}
	lines = append(lines, "", status)

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}
