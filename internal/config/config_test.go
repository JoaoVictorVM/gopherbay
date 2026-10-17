package config

import "testing"

func TestEmpty(t *testing.T) {
	c := Empty()

	if c.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", c.Version, CurrentVersion)
	}
	if c.Keys.Release != DefaultReleaseKey {
		t.Errorf("Keys.Release = %q, want %q", c.Keys.Release, DefaultReleaseKey)
	}
	if c.Tools == nil || len(c.Tools) != 0 {
		t.Errorf("Tools = %#v, want empty non nil slice", c.Tools)
	}
}

func TestClone_SharesNoMemory(t *testing.T) {
	orig := Config{Version: 1, Tools: []Tool{{Name: "a", Command: "a"}}}

	cp := orig.Clone()
	cp.Tools[0].Name = "changed"

	if orig.Tools[0].Name != "a" {
		t.Errorf("original changed through the clone: %q", orig.Tools[0].Name)
	}
}

func TestClone_NilToolsBecomeEmpty(t *testing.T) {
	if got := (Config{}).Clone().Tools; got == nil {
		t.Error("Clone().Tools = nil, want empty slice")
	}
}
