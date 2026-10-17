// Package config loads, validates and atomically saves the gbay config file.
package config

import "slices"

// CurrentVersion is the config format this build reads and writes.
const CurrentVersion = 1

// DefaultReleaseKey is the keystroke that returns focus to gbay while a tool runs.
const DefaultReleaseKey = `ctrl+\`

// Limits enforced on load and on save.
const (
	MaxTools          = 100
	MaxNameLen        = 40
	MaxDescriptionLen = 120
	MaxCommandLen     = 1024
	MaxPathLen        = 1024
	// MaxFileSize keeps hostile files from exhausting memory; 100 tools at the
	// maximum field sizes stay far below it.
	MaxFileSize = 1 << 20
)

// DirMode selects the working folder of a tool.
type DirMode string

// Working folder modes.
const (
	ModeCurrent DirMode = "current"
	ModeFixed   DirMode = "fixed"
	ModeAsk     DirMode = "ask"
)

// Config is the whole config file. Tools keep file order, which is sidebar order.
type Config struct {
	Version int    `yaml:"version"`
	Keys    Keys   `yaml:"keys"`
	Tools   []Tool `yaml:"tools"`
}

// Keys holds the key bindings.
type Keys struct {
	// Release is a normalized Bubble Tea keystroke string.
	Release string `yaml:"release"`
}

// Tool is one entry of the sidebar.
type Tool struct {
	Name        string `yaml:"name"`
	Command     string `yaml:"command"`
	Description string `yaml:"description,omitempty"`
	Dir         Dir    `yaml:"dir"`
}

// Dir is the working folder setting of a tool.
type Dir struct {
	Mode DirMode `yaml:"mode"`
	// Path is required for ModeFixed and kept untouched for the other modes.
	Path string `yaml:"path,omitempty"`
}

// Empty returns the config used when the file is missing: current version,
// default release key and no tools.
func Empty() Config {
	return Config{Version: CurrentVersion, Keys: Keys{Release: DefaultReleaseKey}, Tools: []Tool{}}
}

// Clone returns a copy that shares no memory with c.
func (c Config) Clone() Config {
	c.Tools = slices.Clone(c.Tools)
	if c.Tools == nil {
		c.Tools = []Tool{}
	}
	return c
}
