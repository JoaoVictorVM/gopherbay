// Package version holds the build metadata injected with -ldflags -X.
package version

import (
	"fmt"
	"strings"
	"time"
)

// Set at build time with -X github.com/JoaoVictorVM/gopherbay/internal/version.<Name>=<value>.
var (
	Version = "dev"
	Commit  = "none"
	Date    = ""
)

// String returns the line printed by gbay --version.
func String() string {
	return format(Version, Commit, Date)
}

func format(v, commit, date string) string {
	return fmt.Sprintf("gbay %s (commit %s, %s)", normalizeVersion(v), normalizeCommit(commit), normalizeDate(date))
}

// GoReleaser passes the tag without the leading "v", while git describe keeps it.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "dev"
	}
	if v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
}

func normalizeCommit(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return "none"
	}
	return c
}

// The Makefile injects YYYY-MM-DD and GoReleaser injects RFC 3339, both print as YYYY-MM-DD.
func normalizeDate(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return "data desconhecida"
	}
	if t, err := time.Parse(time.RFC3339, d); err == nil {
		return t.UTC().Format(time.DateOnly)
	}
	if t, err := time.Parse(time.DateOnly, d); err == nil {
		return t.Format(time.DateOnly)
	}
	return "data desconhecida"
}
