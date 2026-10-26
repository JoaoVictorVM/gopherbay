package app

import "charm.land/lipgloss/v2"

type styles struct {
	title    lipgloss.Style
	selected lipgloss.Style
	text     lipgloss.Style
	border   lipgloss.Style
	error    lipgloss.Style
	status   lipgloss.Style
}

func newStyles() styles {
	return styles{
		title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0")),
		selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#67D4DF")),
		text:     lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")),
		border:   lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#64748B")),
		error:    lipgloss.NewStyle().Foreground(lipgloss.Color("#F4C572")),
		status:   lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8")),
	}
}
