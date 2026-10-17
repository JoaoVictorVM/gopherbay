// Command prototype is a throwaway check that the embedded terminal stack
// (Bubble Tea v2, xpty and x/vt) keeps real tools faithful inside a pane.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/charmbracelet/x/xpty"
)

const (
	sidebarWidth = 28
	minPaneW     = 20
	minPaneH     = 5

	statusIdle    = "↑↓ mover · enter executar · q sair"
	statusRunning = "ctrl+\\ encerrar ferramenta"
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
	sess     *session
	running  bool
}

type session struct {
	entry  entry
	pty    xpty.Pty
	cmd    *exec.Cmd
	emu    *vt.SafeEmulator
	ctx    context.Context
	cancel context.CancelFunc
	screen chan struct{}
	ended  chan struct{}
}

type screenMsg struct{ s *session }

type notFoundError struct{ name string }

func (e *notFoundError) Error() string {
	return "Comando não encontrado: " + e.name + " (verifique se está no PATH)"
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
		if m.running {
			return m, nil
		}
		return m, m.sidebarKey(msg)
	case screenMsg:
		if msg.s == m.sess {
			return m, waitScreen(msg.s)
		}
	}
	return m, nil
}

func (m *model) sidebarKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "enter":
		return m.start()
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

func (m *model) start() tea.Cmd {
	pane, ok := paneRect(m.width, m.height)
	if !ok {
		return nil
	}
	s, err := startSession(m.entries[m.selected], pane.w, pane.h)
	if err != nil {
		var nf *notFoundError
		if errors.As(err, &nf) {
			m.status = nf.Error()
		} else {
			m.status = "Não foi possível iniciar o terminal embutido: " + err.Error()
		}
		return nil
	}
	m.sess, m.running, m.status = s, true, statusRunning
	return waitScreen(s)
}

func startSession(e entry, cols, rows int) (*session, error) {
	fields := strings.Fields(e.command)
	if len(fields) == 0 {
		return nil, &notFoundError{name: e.command}
	}
	path, err := exec.LookPath(fields[0])
	if err != nil {
		return nil, &notFoundError{name: fields[0]}
	}

	p, err := xpty.NewPty(cols, rows)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, fields[1:]...)
	if runtime.GOOS != "windows" {
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	}
	if err := p.Start(cmd); err != nil {
		_ = p.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &session{
		entry:  e,
		pty:    p,
		cmd:    cmd,
		emu:    vt.NewSafeEmulator(cols, rows),
		ctx:    ctx,
		cancel: cancel,
		screen: make(chan struct{}, 1),
		ended:  make(chan struct{}),
	}
	go s.readOutput()
	go s.pumpInput()
	return s, nil
}

func (s *session) readOutput() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			_, _ = s.emu.Write(buf[:n])
			// A pending signal already covers this write, so bursts collapse
			// into a single redraw.
			select {
			case s.screen <- struct{}{}:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

// pumpInput carries encoded keys and the emulator's replies to terminal
// queries. It must keep draining even after the pseudoterminal is gone,
// because the emulator writes those replies into an unbuffered pipe and would
// block the output reader otherwise.
func (s *session) pumpInput() {
	buf := make([]byte, 4096)
	for {
		n, err := s.emu.Read(buf)
		if n > 0 {
			_, _ = s.pty.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func waitScreen(s *session) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-s.screen:
			return screenMsg{s: s}
		case <-s.ended:
			return nil
		}
	}
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

	if m.running && !m.sess.emu.CursorHidden() {
		pos := m.sess.emu.CursorPosition()
		if pos.X < pane.w && pos.Y < pane.h {
			v.Cursor = tea.NewCursor(pane.x+pos.X, pane.y+pos.Y)
		}
	}
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
	if m.sess == nil {
		return nil
	}
	return strings.Split(m.sess.emu.Render(), "\n")
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
