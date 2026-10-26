package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
)

// fakeStore returns the next queued result on each Load, so a test can make
// a reload succeed or fail.
type fakeStore struct {
	path    string
	cfg     config.Config
	err     *config.Error
	results []fakeLoad
	loads   int
}

type fakeLoad struct {
	cfg config.Config
	err *config.Error
}

func (f *fakeStore) Load() error {
	f.loads++
	if len(f.results) > 0 {
		f.cfg, f.err = f.results[0].cfg, f.results[0].err
		f.results = f.results[1:]
	}
	if f.err != nil {
		f.cfg = config.Empty()
		return f.err
	}
	return nil
}

func (f *fakeStore) Config() config.Config    { return f.cfg.Clone() }
func (f *fakeStore) LoadError() *config.Error { return f.err }
func (f *fakeStore) Path() string             { return f.path }

func configWith(names ...string) config.Config {
	cfg := config.Empty()
	for _, name := range names {
		cfg.Tools = append(cfg.Tools, config.Tool{Name: name, Command: "x", Dir: config.Dir{Mode: config.ModeCurrent}})
	}
	return cfg
}

func newTestModelWith(t *testing.T, store *fakeStore) (*model, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	m := newModel(logger, time.Now(), store)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m, &buf
}

func newTestModel(t *testing.T) (*model, *bytes.Buffer) {
	t.Helper()
	return newTestModelWith(t, &fakeStore{path: "config.yaml", cfg: config.Empty()})
}

func assertQuits(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("Update() cmd = nil, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("Update() cmd does not produce tea.QuitMsg")
	}
}

func TestModel_QuitsOnQ(t *testing.T) {
	m, _ := newTestModel(t)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})

	assertQuits(t, cmd)
}

func TestModel_QuitsOnCtrlC(t *testing.T) {
	m, _ := newTestModel(t)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})

	assertQuits(t, cmd)
}

func TestModel_IgnoresOtherKeys(t *testing.T) {
	keys := []tea.KeyPressMsg{
		{Code: 'x', Text: "x"},
		{Code: 'Q', Text: "Q", ShiftedCode: 'Q'},
		{Code: tea.KeyUp},
		{Code: tea.KeyDown},
		{Code: tea.KeyEnter},
	}
	for _, key := range keys {
		t.Run(key.String(), func(t *testing.T) {
			m, _ := newTestModel(t)
			if _, cmd := m.Update(key); cmd != nil {
				t.Errorf("Update(%q) cmd != nil, want nil", key.String())
			}
		})
	}
}

func TestModel_ViewEmptyState(t *testing.T) {
	m, _ := newTestModel(t)

	v := m.View()

	for _, want := range []string{"gopherbay", emptyToolList, m.footer("sidebar", m.width)} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("View().Content = %q, want it to contain %q", v.Content, want)
		}
	}
	if !v.AltScreen {
		t.Error("View().AltScreen = false, want true")
	}
}

func TestModel_FirstFrameLoggedOnce(t *testing.T) {
	m, buf := newTestModel(t)

	for range 3 {
		m.View()
	}

	if got := strings.Count(buf.String(), `msg="first frame"`); got != 1 {
		t.Errorf("first frame entries = %d, want 1; log = %q", got, buf.String())
	}
	if !strings.Contains(buf.String(), "elapsed_ms=") {
		t.Errorf("log = %q, want elapsed_ms attribute", buf.String())
	}
}

func TestModel_WindowSizeLogged(t *testing.T) {
	m, buf := newTestModel(t)

	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	if cmd != nil {
		t.Errorf("Update() cmd != nil, want nil")
	}
	want := `level=DEBUG msg="window resized" cols=120 rows=30`
	if !strings.Contains(buf.String(), want) {
		t.Errorf("log = %q, want %q", buf.String(), want)
	}
}

func TestModel_ListsToolsInOrder(t *testing.T) {
	m, _ := newTestModelWith(t, &fakeStore{cfg: configWith("Relógio", "Snake", "Curso")})

	content := ansi.Strip(m.View().Content)
	previous := -1
	for _, name := range []string{"Relógio", "Snake", "Curso"} {
		index := strings.Index(content, name)
		if index <= previous {
			t.Errorf("tool %q out of order: %q", name, content)
		}
		previous = index
	}
}

func syntaxError(line int) *config.Error {
	return &config.Error{Kind: config.KindSyntax, Path: `C:\cfg\config.yaml`, Line: line, Rule: config.RuleSyntax, Message: "value is not allowed in this context"}
}

func readOnlyStore(line int) *fakeStore {
	return &fakeStore{path: `C:\cfg\config.yaml`, cfg: config.Empty(), err: syntaxError(line)}
}

func press(m *model, key string) tea.Cmd {
	r := []rune(key)[0]
	_, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: key})
	return cmd
}

func TestModel_ReadOnlyView(t *testing.T) {
	m, _ := newTestModelWith(t, readOnlyStore(12))

	content := ansi.Strip(m.View().Content)
	for _, want := range []string{m.loadErr.Error(), `Arquivo: C:\cfg\config.yaml`, readOnlyHint} {
		if !strings.Contains(content, want) {
			t.Errorf("View().Content = %q, want %q", content, want)
		}
	}
}

func TestModel_ReadOnlyReadError(t *testing.T) {
	store := &fakeStore{path: "/cfg/config.yaml", cfg: config.Empty(), err: &config.Error{Kind: config.KindRead, Path: "/cfg/config.yaml", Message: "permission denied"}}
	m, _ := newTestModelWith(t, store)

	for _, want := range []string{"Não foi possível ler o config.yaml: permission denied", "Arquivo: /cfg/config.yaml"} {
		if content := ansi.Strip(m.View().Content); !strings.Contains(content, want) {
			t.Errorf("View().Content = %q, want %q", content, want)
		}
	}
}

func TestModel_ReadOnlyBlocksEditKeys(t *testing.T) {
	for _, key := range []string{"a", "e", "d", "K", "J"} {
		t.Run(key, func(t *testing.T) {
			store := readOnlyStore(3)
			m, _ := newTestModelWith(t, store)

			if cmd := press(m, key); cmd != nil {
				t.Errorf("Update(%q) cmd != nil, want nil", key)
			}

			if !strings.Contains(ansi.Strip(m.View().Content), readOnlyStatus) {
				t.Errorf("View().Content = %q, want the read-only status", m.View().Content)
			}
			if store.loads != 0 {
				t.Errorf("store loaded %d times, want untouched", store.loads)
			}
		})
	}
}

func TestModel_EditKeysIgnoredInNormalMode(t *testing.T) {
	m, _ := newTestModelWith(t, &fakeStore{cfg: configWith("a")})

	for _, key := range []string{"a", "e", "d", "K", "J", "?"} {
		if cmd := press(m, key); cmd != nil {
			t.Errorf("Update(%q) cmd != nil, want nil", key)
		}
	}
	if m.status != "" {
		t.Errorf("status = %q, want empty", m.status)
	}
}

func TestModel_ReloadKey(t *testing.T) {
	stores := map[string]*fakeStore{"normal": {cfg: configWith("a")}, "read-only": readOnlyStore(3)}
	for name, store := range stores {
		t.Run(name, func(t *testing.T) {
			m, _ := newTestModelWith(t, store)

			cmd := press(m, "r")

			if cmd == nil {
				t.Fatal("Update(r) cmd = nil, want a reload command")
			}
			if store.loads != 0 {
				t.Error("store loaded inside Update, want it deferred to the command")
			}
			if _, ok := cmd().(configLoadedMsg); !ok {
				t.Error("reload command does not produce configLoadedMsg")
			}
			if store.loads != 1 {
				t.Errorf("store loaded %d times, want 1", store.loads)
			}
		})
	}
}

func TestModel_ReloadFixesReadOnly(t *testing.T) {
	store := readOnlyStore(3)
	store.results = []fakeLoad{{cfg: configWith("Relógio", "Snake")}}
	m, _ := newTestModelWith(t, store)

	m.Update(press(m, "r")())

	for _, want := range []string{"Relógio", "Snake", reloadedStatus} {
		if content := ansi.Strip(m.View().Content); !strings.Contains(content, want) {
			t.Errorf("View().Content = %q, want %q", content, want)
		}
	}
}

func TestModel_ReloadKeepsReadOnlyWithNewError(t *testing.T) {
	store := readOnlyStore(3)
	store.results = []fakeLoad{{cfg: config.Empty(), err: syntaxError(9)}}
	m, _ := newTestModelWith(t, store)
	press(m, "a")

	m.Update(press(m, "r")())

	content := m.View().Content
	if !strings.Contains(content, "Erro no config.yaml (linha 9): ") || strings.Contains(content, "(linha 3)") {
		t.Errorf("View().Content = %q, want the new error only", content)
	}
	if m.status != "" {
		t.Errorf("View().Content = %q, want the status cleared", content)
	}
}

func TestModel_ReloadIntoReadOnly(t *testing.T) {
	store := &fakeStore{cfg: configWith("a")}
	store.results = []fakeLoad{{cfg: config.Empty(), err: syntaxError(2)}}
	m, _ := newTestModelWith(t, store)

	m.Update(press(m, "r")())

	if content := m.View().Content; !strings.Contains(content, "Erro no config.yaml (linha 2)") || strings.Contains(content, "\na\n") {
		t.Errorf("View().Content = %q, want the error and no tools", content)
	}
}

func TestModel_QuitInReadOnly(t *testing.T) {
	m, _ := newTestModelWith(t, readOnlyStore(3))

	assertQuits(t, press(m, "q"))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assertQuits(t, cmd)
}

func TestModel_InitialSelection(t *testing.T) {
	m, _ := newTestModelWith(t, &fakeStore{cfg: configWith("Relógio", "Snake")})
	tool, ok := m.selectedTool()
	if !ok || tool.Name != "Relógio" || m.selected != 0 {
		t.Fatalf("initial selection = %+v, %v, index %d", tool, ok, m.selected)
	}
}

func TestModel_NavigationClamps(t *testing.T) {
	for _, keys := range [][]tea.KeyPressMsg{
		{{Code: tea.KeyDown}, {Code: tea.KeyUp}},
		{{Code: 'j', Text: "j"}, {Code: 'k', Text: "k"}},
	} {
		m, _ := newTestModelWith(t, &fakeStore{cfg: configWith("a", "b", "c")})
		for range 5 {
			m.Update(keys[0])
		}
		if m.selected != 2 {
			t.Fatalf("down wrapped: %d", m.selected)
		}
		for range 5 {
			m.Update(keys[1])
		}
		if m.selected != 0 {
			t.Fatalf("up wrapped: %d", m.selected)
		}
		press(m, "J")
		press(m, "K")
		if m.selected != 0 {
			t.Fatal("uppercase shortcuts moved selection")
		}
	}
}

func TestModel_EmptySelection(t *testing.T) {
	for _, store := range []*fakeStore{{cfg: config.Empty()}, readOnlyStore(3)} {
		m, _ := newTestModelWith(t, store)
		press(m, "j")
		press(m, "k")
		if tool, ok := m.selectedTool(); ok || tool != (config.Tool{}) || m.selected != -1 {
			t.Fatalf("unexpected selection: %+v, %v, %d", tool, ok, m.selected)
		}
	}
}

func TestModel_SelectedToolSnapshot(t *testing.T) {
	cfg := configWith("Curso")
	cfg.Tools[0].Command = "curso --lista"
	cfg.Tools[0].Description = "Controle de cursos"
	cfg.Tools[0].Dir = config.Dir{Mode: config.ModeFixed, Path: `D:\projetos\curso`}
	m, _ := newTestModelWith(t, &fakeStore{cfg: cfg})
	tool, ok := m.selectedTool()
	if !ok || tool != cfg.Tools[0] {
		t.Fatalf("incomplete selected record: %+v", tool)
	}
	tool.Name = "changed"
	if m.cfg.Tools[0].Name != "Curso" {
		t.Fatal("selected record aliases model")
	}
}

func TestModel_ReloadReconcilesSelection(t *testing.T) {
	for _, tt := range []struct {
		name  string
		names []string
		want  int
	}{
		{"same name ignoring case", []string{"B", "a", "c"}, 0},
		{"removed middle", []string{"a", "c"}, 1},
		{"clamp last", []string{"a"}, 0},
		{"empty", nil, -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{cfg: configWith("a", "b", "c"), results: []fakeLoad{{cfg: configWith(tt.names...)}}}
			m, _ := newTestModelWith(t, store)
			press(m, "j")
			m.Update(press(m, "r")())
			if m.selected != tt.want {
				t.Fatalf("selected = %d, want %d", m.selected, tt.want)
			}
		})
	}
}

func TestModel_ReloadSingleFlight(t *testing.T) {
	store := &fakeStore{cfg: configWith("a")}
	m, _ := newTestModelWith(t, store)
	cmd := press(m, "r")
	if cmd == nil || !m.reloading || store.loads != 0 {
		t.Fatal("reload must be pending without synchronous I/O")
	}
	for range 3 {
		if press(m, "r") != nil {
			t.Fatal("concurrent reload scheduled")
		}
	}
	m.Update(cmd())
	if m.reloading || store.loads != 1 || press(m, "r") == nil {
		t.Fatal("reload did not become available after completion")
	}
}

func TestModel_HelpRouting(t *testing.T) {
	for _, closeKey := range []tea.KeyPressMsg{{Code: '?', Text: "?"}, {Code: tea.KeyEscape}} {
		m, _ := newTestModelWith(t, &fakeStore{cfg: configWith("a", "b")})
		press(m, "j")
		press(m, "?")
		if !m.helpOpen {
			t.Fatal("help did not open")
		}
		press(m, "j")
		press(m, "k")
		if m.selected != 1 {
			t.Fatal("help navigation changed selected tool")
		}
		m.Update(closeKey)
		if m.helpOpen || !strings.Contains(ansi.Strip(m.View().Content), "Comando: x") {
			t.Fatal("help did not restore details")
		}
	}
}

func TestModel_ReadOnlyWithHelp(t *testing.T) {
	m, _ := newTestModelWith(t, readOnlyStore(12))
	press(m, "?")
	for _, key := range []string{"a", "e", "d", "K", "J"} {
		press(m, key)
		if m.status != readOnlyStatus || m.selected != -1 || m.store.(*fakeStore).loads != 0 {
			t.Fatalf("read-only help allowed action %s", key)
		}
	}
	assertQuits(t, press(m, "q"))
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assertQuits(t, cmd)
	press(m, "?")
	if m.helpOpen || !strings.Contains(ansi.Strip(m.View().Content), "Erro no config.yaml") {
		t.Fatal("closing help lost diagnostic")
	}
}

func TestModel_SmallTerminalPreservesState(t *testing.T) {
	m, _ := newTestModelWith(t, &fakeStore{cfg: configWith("a", "b")})
	press(m, "j")
	press(m, "?")
	m.Update(tea.WindowSizeMsg{Width: 59, Height: 20})
	press(m, "k")
	press(m, "?")
	if m.selected != 1 || !m.helpOpen {
		t.Fatal("small terminal changed navigation context")
	}
	assertQuits(t, press(m, "q"))
	cmd := press(m, "r")
	if cmd == nil {
		t.Fatal("small terminal blocked reload")
	}
	m.Update(cmd())
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	if m.selected != 1 || !m.helpOpen {
		t.Fatal("restored terminal lost context")
	}
}
