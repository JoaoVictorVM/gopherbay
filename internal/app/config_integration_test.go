package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/JoaoVictorVM/gopherbay/internal/config"
)

func configUI(t *testing.T, content string) (*model, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if content != "" {
		writeConfig(t, path, content)
	}
	logger := slog.New(slog.DiscardHandler)
	store := config.NewStore(path, logger)
	_ = store.Load()
	m := newModel(logger, time.Now(), store)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return m, path
}

func TestConfigUI_OrderAndDetails(t *testing.T) {
	var tools []string
	for i := range 5 {
		tools = append(tools, fmt.Sprintf("  - name: Ferramenta %d\n    command: tool-%d --opcao\n    description: Descrição %d\n    dir:\n      mode: fixed\n      path: /projetos/pasta-%d\n", i, i, i, i))
	}
	m, path := configUI(t, "version: 1\ntools:\n"+strings.Join(tools, ""))
	for i := range 5 {
		content := ansi.Strip(m.View().Content)
		for _, want := range []string{fmt.Sprintf("▸ Ferramenta %d", i), fmt.Sprintf("Comando: tool-%d --opcao", i), fmt.Sprintf("Descrição: Descrição %d", i), fmt.Sprintf("Pasta: fixa (/projetos/pasta-%d)", i)} {
			if !strings.Contains(content, want) {
				t.Fatalf("missing %q: %s", want, content)
			}
		}
		if i < 4 {
			press(m, "j")
		}
	}
	writeConfig(t, path, "version: 1\ntools:\n"+tools[4]+strings.Join(tools[:4], ""))
	m.Update(press(m, "r")())
	if m.selected != 0 || !strings.Contains(ansi.Strip(m.View().Content), "▸ Ferramenta 4") {
		t.Fatal("reload did not follow reordered file")
	}
}

func TestConfigUI_ReadOnlyReload(t *testing.T) {
	m, path := configUI(t, brokenConfig)
	if m.loadErr == nil || m.selected != -1 {
		t.Fatal("invalid config did not enter read-only mode")
	}
	press(m, "?")
	if !m.helpOpen {
		t.Fatal("read-only mode blocked help")
	}
	press(m, "?")
	for _, key := range []string{"a", "e", "d", "K", "J"} {
		press(m, key)
		if m.status != readOnlyStatus {
			t.Fatalf("edit key %s did not report read-only mode", key)
		}
	}
	before, err := os.ReadFile(path)
	if err != nil || string(before) != brokenConfig {
		t.Fatalf("invalid config overwritten: %q, %v", before, err)
	}
	writeConfig(t, path, twoTools)
	m.Update(press(m, "r")())
	tool, ok := m.selectedTool()
	if !ok || tool.Name != "Relógio" || m.loadErr != nil || m.status != reloadedStatus {
		t.Fatal("corrected file did not recover selection and details")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Comando: clock-tty") {
		t.Fatal("recovered details missing")
	}
}

func TestConfigUI_ReloadTransitions(t *testing.T) {
	m, path := configUI(t, twoTools)
	writeConfig(t, path, brokenConfig)
	m.Update(press(m, "r")())
	if m.loadErr == nil || m.loadErr.Line != 4 {
		t.Fatal("reload missed invalid YAML")
	}
	writeConfig(t, path, "\n"+brokenConfig)
	m.Update(press(m, "r")())
	if m.loadErr == nil || m.loadErr.Line != 5 {
		t.Fatal("reload did not refresh diagnostic line")
	}
	if tool, ok := m.selectedTool(); ok || tool != (config.Tool{}) {
		t.Fatal("error left stale selection")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	m.Update(press(m, "r")())
	if m.loadErr != nil || !strings.Contains(ansi.Strip(m.View().Content), emptyToolList) {
		t.Fatal("missing file should recover to empty config")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("reload created missing config: %v", err)
	}
	writeConfig(t, path, twoTools)
	m.Update(press(m, "r")())
	if m.selected != 0 {
		t.Fatal("empty-to-valid transition did not select first tool")
	}
}

func TestConfigUI_RejectsFiftyCharacterName(t *testing.T) {
	m, _ := configUI(t, "version: 1\ntools:\n  - name: "+strings.Repeat("a", 50)+"\n    command: x\n")
	if m.loadErr == nil || m.loadErr.Rule != config.RuleNameTooLong {
		t.Fatalf("long name not rejected: %+v", m.loadErr)
	}
}
