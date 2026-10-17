// Command prototype is a throwaway check that the embedded terminal stack
// (Bubble Tea v2, xpty and x/vt) keeps real tools faithful inside a pane.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/charmbracelet/x/xpty"
)

const (
	sidebarWidth = 28
	minPaneW     = 20
	minPaneH     = 5

	// ConPTY may flush the last frame only after the process is gone, so the
	// reader gets a short grace period before the emulator is closed.
	drainTimeout = 500 * time.Millisecond

	statusIdle    = "↑↓ mover · enter executar · q sair"
	statusRunning = "ctrl+\\ encerrar ferramenta"
	statusKilled  = "✗ encerrada pelo protótipo (Ctrl+\\)"
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
	log      *slog.Logger
	// pendingEnd holds the exit until the first frame with the sidebar
	// focused, which is where exit detection is measured.
	pendingEnd *exitMsg
	killMs     int64
}

type session struct {
	entry     entry
	pty       xpty.Pty
	cmd       *exec.Cmd
	emu       *vt.SafeEmulator
	ctx       context.Context
	cancel    context.CancelFunc
	screen    chan struct{}
	ended     chan struct{}
	exited    chan exitMsg
	drained   chan struct{}
	killed    atomic.Bool
	started   time.Time
	releaseAt time.Time
	echo      echoMeter
}

// echoMeter approximates key echo as the time from forwarding a printable key
// to the next chunk of output, since the painted frame is not observable.
type echoMeter struct {
	mu      sync.Mutex
	pending []time.Time
	samples []time.Duration
}

func (e *echoMeter) keySent(t time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending = append(e.pending, t)
}

func (e *echoMeter) output(t time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.pending {
		e.samples = append(e.samples, t.Sub(p))
	}
	e.pending = e.pending[:0]
}

func (e *echoMeter) stats() (p50, p95 time.Duration, n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return percentile(e.samples, 0.50), percentile(e.samples, 0.95), len(e.samples)
}

func percentile(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[max(i, 0)]
}

type screenMsg struct{ s *session }

type exitMsg struct {
	s            *session
	code         int
	killed       bool
	waitReturned time.Time
}

type notFoundError struct{ name string }

func (e *notFoundError) Error() string {
	return "Comando não encontrado: " + e.name + " (verifique se está no PATH)"
}

func main() {
	relogio := flag.String("relogio", "clock-tty --24h", "linha de comando do relógio")
	jogo := flag.String("jogo", "snake", "linha de comando do jogo")
	uso := flag.String("uso", "usage-bar", "linha de comando da barra de uso")
	flag.Parse()

	os.Exit(run([]entry{
		{key: "relogio", label: "Relógio", command: *relogio},
		{key: "jogo", label: "Jogo", command: *jogo},
		{key: "uso", label: "Barra de uso", command: *uso},
	}))
}

func run(entries []entry) int {
	// The log lives outside the repository and the current folder on purpose;
	// it is truncated on every start so each run is measured on its own.
	logFile, err := os.Create(filepath.Join(os.TempDir(), "gbay-prototipo.log"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer logFile.Close()

	m := &model{
		entries: entries,
		status:  statusIdle,
		log:     slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	if _, err := tea.NewProgram(m, tea.WithFPS(60)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if pane, ok := paneRect(m.width, m.height); ok && m.running {
			m.sess.resize(pane.w, pane.h)
			m.log.Debug("session resized", "cols", pane.w, "rows", pane.h)
		}
	case tea.KeyPressMsg:
		if !m.running {
			return m, m.sidebarKey(msg)
		}
		if isReleaseKey(tea.Key(msg)) {
			m.log.Info("release key pressed")
			m.sess.releaseAt = time.Now()
			m.sess.kill()
			return m, nil
		}
		m.sess.sendKey(msg)
	case tea.PasteMsg:
		if m.running {
			m.sess.emu.Paste(msg.Content)
		}
	case screenMsg:
		if msg.s == m.sess && m.running {
			return m, waitScreen(msg.s)
		}
	case exitMsg:
		if msg.s == m.sess {
			m.running = false
			m.status = exitStatus(msg)
			m.pendingEnd = &msg
			if msg.killed {
				m.killMs = time.Since(msg.s.releaseAt).Milliseconds()
			}
		}
	}
	return m, nil
}

func exitStatus(msg exitMsg) string {
	switch {
	case msg.killed:
		return statusKilled
	case msg.code == 0:
		return "✓ terminou · código 0"
	default:
		return fmt.Sprintf("✗ terminou · código %d", msg.code)
	}
}

// isReleaseKey accepts the shapes Ctrl+\ can take: Windows Terminal on an
// ABNT2 keyboard may report the layout key in BaseCode, or only the raw
// control character 0x1C.
func isReleaseKey(k tea.Key) bool {
	if k.Code == 0x1c {
		return true
	}
	return k.Mod&tea.ModCtrl != 0 && (k.Code == '\\' || k.BaseCode == '\\')
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
	default:
		// Only the key shape is recorded, never its text, to find out how the
		// terminal reports combinations such as Ctrl+\.
		m.log.Debug("unhandled key", "code", msg.Code, "mod", msg.Mod)
	}
	return nil
}

func (m *model) start() tea.Cmd {
	pane, ok := paneRect(m.width, m.height)
	if !ok {
		return nil
	}
	e := m.entries[m.selected]
	s, err := startSession(e, pane.w, pane.h)
	if err != nil {
		m.log.Error("start failed", "entry", e.key, "command", e.command, "error", err)
		var nf *notFoundError
		if errors.As(err, &nf) {
			m.status = nf.Error()
		} else {
			m.status = "Não foi possível iniciar o terminal embutido: " + err.Error()
		}
		return nil
	}
	m.log.Info("session started", "entry", e.key, "command", e.command, "cols", pane.w, "rows", pane.h)
	m.sess, m.running, m.status = s, true, statusRunning
	return tea.Batch(waitScreen(s), waitExit(s))
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
		entry:   e,
		pty:     p,
		cmd:     cmd,
		emu:     vt.NewSafeEmulator(cols, rows),
		ctx:     ctx,
		cancel:  cancel,
		screen:  make(chan struct{}, 1),
		ended:   make(chan struct{}),
		exited:  make(chan exitMsg, 1),
		drained: make(chan struct{}),
		started: time.Now(),
	}
	go s.readOutput()
	go s.pumpInput()
	go s.wait()
	return s, nil
}

func (s *session) readOutput() {
	buf := make([]byte, 32*1024)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.echo.output(time.Now())
			_, _ = s.emu.Write(buf[:n])
			// A pending signal already covers this write, so bursts collapse
			// into a single redraw.
			select {
			case s.screen <- struct{}{}:
			default:
			}
		}
		if err != nil {
			close(s.drained)
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

func (s *session) wait() {
	err := xpty.WaitProcess(s.ctx, s.cmd)
	returned := time.Now()

	code := 0
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	case err != nil:
		code = -1
	}

	_ = s.pty.Close()
	select {
	case <-s.drained:
	case <-time.After(drainTimeout):
	}
	_ = s.emu.Close()
	close(s.ended)
	s.exited <- exitMsg{s: s, code: code, killed: s.killed.Load(), waitReturned: returned}
}

func (s *session) kill() {
	s.killed.Store(true)
	s.cancel()
	// WaitProcess only honors the context on Windows; elsewhere it is a plain
	// cmd.Wait, so the process has to be killed directly.
	if runtime.GOOS != "windows" {
		_ = s.cmd.Process.Kill()
	}
}

// resize follows the emulator first, so the redraw the tool sends after the
// pseudoterminal changes size lands on a screen that already has the new
// geometry.
func (s *session) resize(cols, rows int) {
	s.emu.Resize(cols, rows)
	_ = s.pty.Resize(cols, rows)
}

func (s *session) sendKey(msg tea.KeyPressMsg) {
	k := vt.KeyPressEvent(msg)
	// The emulator matches keys by comparing whole structs, so fields it does
	// not encode would make known keys fall through to plain text.
	k.IsRepeat = false
	if k.Mod&vt.ModCtrl != 0 {
		k.Text = ""
	}
	if k.Text != "" {
		s.echo.keySent(time.Now())
	}
	s.emu.SendKey(k)
}

func waitExit(s *session) tea.Cmd {
	return func() tea.Msg {
		return <-s.exited
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
	if m.pendingEnd != nil {
		m.logEnd(*m.pendingEnd)
		m.pendingEnd = nil
	}

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

func (m *model) logEnd(msg exitMsg) {
	p50, p95, n := msg.s.echo.stats()
	attrs := []any{
		"entry", msg.s.entry.key,
		"exit_code", msg.code,
		"killed", msg.killed,
		"duration", msg.waitReturned.Sub(msg.s.started),
		"exit_detect_ms", time.Since(msg.waitReturned).Milliseconds(),
	}
	if msg.killed {
		attrs = append(attrs, "kill_ms", m.killMs)
	}
	attrs = append(attrs, "echo_p50_ms", p50.Milliseconds(), "echo_p95_ms", p95.Milliseconds(), "echo_samples", n)
	m.log.Info("session ended", attrs...)
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
