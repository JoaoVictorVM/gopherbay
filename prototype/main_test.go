package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestPaneRect(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		want          rect
		wantOK        bool
	}{
		{"120x30", 120, 30, rect{x: 28, y: 0, w: 92, h: 29}, true},
		{"80x24", 80, 24, rect{x: 28, y: 0, w: 52, h: 23}, true},
		{"exact minimum", sidebarWidth + minPaneW, minPaneH + 1, rect{x: 28, y: 0, w: minPaneW, h: minPaneH}, true},
		{"too narrow", sidebarWidth + minPaneW - 1, 30, rect{x: 28, y: 0, w: minPaneW - 1, h: 29}, false},
		{"too short", 120, minPaneH, rect{x: 28, y: 0, w: 92, h: minPaneH - 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := paneRect(tt.width, tt.height)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("paneRect(%d, %d) = %+v, %v; want %+v, %v", tt.width, tt.height, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestPadLine(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		width     int
		wantPlain string
	}{
		{"plain text", "abc", 6, "abc   "},
		{"styled text", "\x1b[31mabc\x1b[m", 5, "abc  "},
		{"wide characters", "日本語", 5, "日本 "},
		{"longer than width", "abcdefgh", 4, "abcd"},
		{"styled and longer", "\x1b[1;32mabcdefgh", 3, "abc"},
		{"empty", "", 3, "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := padLine(tt.in, tt.width)
			if w := ansi.StringWidth(got); w != tt.width {
				t.Errorf("width = %d, want %d (%q)", w, tt.width, got)
			}
			if plain := ansi.Strip(got); plain != tt.wantPlain {
				t.Errorf("plain = %q, want %q", plain, tt.wantPlain)
			}
		})
	}

	t.Run("keeps SGR and resets it", func(t *testing.T) {
		got := padLine("\x1b[31mabc", 5)
		if !strings.HasPrefix(got, "\x1b[31mabc") {
			t.Errorf("SGR not preserved: %q", got)
		}
		if !strings.Contains(got, ansi.ResetStyle) {
			t.Errorf("missing style reset: %q", got)
		}
	})

	t.Run("truncation keeps escape sequences whole", func(t *testing.T) {
		got := padLine("ab\x1b[38;2;10;20;30mcdef", 3)
		if !strings.Contains(got, "\x1b[38;2;10;20;30m") {
			t.Errorf("escape sequence split: %q", got)
		}
	})
}

func TestJoinColumns(t *testing.T) {
	tests := []struct {
		name        string
		left, right []string
	}{
		{"sidebar shorter", []string{"a"}, []string{"1", "2", "3", "4"}},
		{"sidebar longer", []string{"a", "b", "c", "d", "e", "f"}, []string{"1"}},
		{"both empty", nil, nil},
	}
	const leftW, rightW, height = 6, 10, 4
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinColumns(tt.left, leftW, tt.right, rightW, height)
			if len(got) != height {
				t.Fatalf("got %d lines, want %d", len(got), height)
			}
			for i, line := range got {
				if w := ansi.StringWidth(line); w != leftW+rightW {
					t.Errorf("line %d width = %d, want %d (%q)", i, w, leftW+rightW, line)
				}
			}
		})
	}
}

func TestIsReleaseKey(t *testing.T) {
	tests := []struct {
		name string
		key  tea.Key
		want bool
	}{
		{"ctrl+backslash by code", tea.Key{Code: '\\', Mod: tea.ModCtrl}, true},
		{"ctrl+backslash by base code", tea.Key{Code: ']', BaseCode: '\\', Mod: tea.ModCtrl}, true},
		{"control character 0x1C", tea.Key{Code: 0x1c}, true},
		{"plain backslash", tea.Key{Code: '\\', Text: "\\"}, false},
		{"ctrl+c", tea.Key{Code: 'c', Mod: tea.ModCtrl}, false},
		{"ctrl+]", tea.Key{Code: ']', Mod: tea.ModCtrl}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReleaseKey(tt.key); got != tt.want {
				t.Errorf("isReleaseKey(%+v) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
