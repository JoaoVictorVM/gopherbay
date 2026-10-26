package app

import (
	"strings"
	"testing"
)

func TestStyles_PreserveTextAndSelection(t *testing.T) {
	s := newStyles()
	for name, got := range map[string]string{
		"title":    s.title.Render("gopherbay"),
		"selected": s.selected.Render("gopherbay"),
		"text":     s.text.Render("gopherbay"),
		"border":   s.border.Render("gopherbay"),
		"error":    s.error.Render("gopherbay"),
		"status":   s.status.Render("gopherbay"),
	} {
		if !strings.Contains(got, "gopherbay") {
			t.Errorf("%s style lost content: %q", name, got)
		}
	}
	if s.title.Render("gopherbay") == s.status.Render("gopherbay") {
		t.Fatal("title must remain visually distinct from status")
	}
}
