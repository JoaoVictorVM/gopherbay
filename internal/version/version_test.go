package version

import "testing"

func TestString(t *testing.T) {
	tests := []struct {
		name    string
		version string
		commit  string
		date    string
		want    string
	}{
		{"defaults", "dev", "none", "", "gbay dev (commit none, data desconhecida)"},
		{"injected", "v0.1.0", "abc1234", "2026-11-01", "gbay v0.1.0 (commit abc1234, 2026-11-01)"},
		{"version without v", "0.1.0", "abc1234", "2026-11-01", "gbay v0.1.0 (commit abc1234, 2026-11-01)"},
		{"git describe dirty", "v0.1.0-3-gabc1234-dirty", "abc1234", "2026-11-01", "gbay v0.1.0-3-gabc1234-dirty (commit abc1234, 2026-11-01)"},
		{"rfc3339 date", "v0.1.0", "abc1234", "2026-11-01T10:15:02Z", "gbay v0.1.0 (commit abc1234, 2026-11-01)"},
		{"rfc3339 date with offset", "v0.1.0", "abc1234", "2026-11-01T23:30:00-03:00", "gbay v0.1.0 (commit abc1234, 2026-11-02)"},
		{"empty version", "", "abc1234", "2026-11-01", "gbay dev (commit abc1234, 2026-11-01)"},
		{"empty commit", "v0.1.0", "", "2026-11-01", "gbay v0.1.0 (commit none, 2026-11-01)"},
		{"empty date", "v0.1.0", "abc1234", "", "gbay v0.1.0 (commit abc1234, data desconhecida)"},
		{"unparseable date", "v0.1.0", "abc1234", "ontem", "gbay v0.1.0 (commit abc1234, data desconhecida)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version, Commit, Date = tt.version, tt.commit, tt.date
			t.Cleanup(func() { Version, Commit, Date = "dev", "none", "" })

			if got := String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
