package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
	return newModel(logger, time.Now(), store), &buf
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

func TestModel_ViewPlaceholder(t *testing.T) {
	m, _ := newTestModel(t)

	v := m.View()

	for _, want := range []string{"gopherbay", "Nenhuma ferramenta cadastrada.", "Aperte q para sair."} {
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

	content := m.View().Content

	want := "gopherbay\n\nRelógio\nSnake\nCurso\n\nAperte q para sair."
	if content != want {
		t.Errorf("View().Content = %q, want %q", content, want)
	}
}
