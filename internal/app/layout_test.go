package app

import "testing"

func TestLayout_PaneSize(t *testing.T) {
	for _, tt := range []struct{ width, height, cols, rows int }{
		{120, 30, 90, 25}, {80, 20, 50, 15}, {60, 16, 30, 11},
	} {
		m := &model{width: tt.width, height: tt.height}
		cols, rows, ok := m.paneSize()
		if !ok || cols != tt.cols || rows != tt.rows {
			t.Errorf("paneSize(%d,%d) = %d,%d,%v", tt.width, tt.height, cols, rows, ok)
		}
	}
}

func TestLayout_MinimumBoundary(t *testing.T) {
	for _, size := range [][2]int{{59, 20}, {80, 15}, {0, 0}, {-1, -1}} {
		m := &model{width: size[0], height: size[1]}
		if cols, rows, ok := m.paneSize(); cols != 0 || rows != 0 || ok {
			t.Errorf("paneSize(%v) = %d,%d,%v", size, cols, rows, ok)
		}
	}
}

func TestLayout_RegionsDoNotOverlap(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {80, 20}, {120, 30}} {
		l := calculateLayout(size[0], size[1])
		regions := []region{l.header, l.sidebar, l.pane, l.status, l.footer}
		for i, a := range regions {
			if a.x < 0 || a.y < 0 || a.width < 1 || a.height < 1 || a.x+a.width > size[0] || a.y+a.height > size[1] {
				t.Fatalf("region outside terminal: %+v", a)
			}
			for _, b := range regions[i+1:] {
				if a.x < b.x+b.width && b.x < a.x+a.width && a.y < b.y+b.height && b.y < a.y+a.height {
					t.Fatalf("overlapping regions: %+v %+v", a, b)
				}
			}
		}
		if l.sidebarInner.width+2 != l.sidebar.width || l.paneInner.height+2 != l.pane.height {
			t.Fatal("internal region does not exclude borders")
		}
	}
}
