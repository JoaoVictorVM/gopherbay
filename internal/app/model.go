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
	emptyToolList  = "Nenhuma ferramenta cadastrada. Aperte a para adicionar."
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

	selected      int
	sidebarOffset int
	paneOffset    int
	helpOpen      bool
	helpOffset    int
	reloading     bool
	styles        styles
}

// newModel expects store to be loaded already, so the first frame shows the config.
func newModel(logger *slog.Logger, start time.Time, store configStore) *model {
	m := &model{logger: logger, start: start, store: store, cfg: store.Config(), loadErr: store.LoadError(), selected: -1, styles: newStyles()}
	if m.loadErr == nil && len(m.cfg.Tools) > 0 {
		m.selected = 0
	}
	return m
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
		default:
			if calculateLayout(m.width, m.height).valid {
				if m.handleHelpKey(msg.String()) {
					return m, nil
				}
				switch msg.String() {
				case "up", "k":
					m.moveSelection(-1)
				case "down", "j":
					m.moveSelection(1)
				case "pgup":
					m.scrollPane(-1)
				case "pgdown":
					m.scrollPane(1)
				}
			}
		}
	case configLoadedMsg:
		previous, hadSelection := m.selectedTool()
		previousIndex := m.selected
		m.cfg, m.loadErr = msg.cfg, msg.err
		m.reloading = false
		m.status = ""
		if msg.err == nil {
			m.status = reloadedStatus
		}
		m.selected = -1
		if msg.err == nil && len(m.cfg.Tools) > 0 {
			m.selected = max(0, min(previousIndex, len(m.cfg.Tools)-1))
			if hadSelection {
				for i, tool := range m.cfg.Tools {
					if strings.EqualFold(tool.Name, previous.Name) {
						m.selected = i
						break
					}
				}
			}
		}
		m.sidebarOffset, m.paneOffset = 0, 0
		m.clampOffsets()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampOffsets()
		m.logger.Debug("window resized", "cols", msg.Width, "rows", msg.Height)
	}
	return m, nil
}

func (m *model) reload() tea.Cmd {
	if m.reloading {
		return nil
	}
	m.reloading = true
	store := m.store
	return func() tea.Msg {
		_ = store.Load()
		return configLoadedMsg{cfg: store.Config(), err: store.LoadError()}
	}
}
