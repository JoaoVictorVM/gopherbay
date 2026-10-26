package app

const (
	minimumWidth  = 60
	minimumHeight = 16
	sidebarWidth  = 28
)

type region struct {
	x, y, width, height int
}

type layout struct {
	valid        bool
	header       region
	sidebar      region
	sidebarInner region
	pane         region
	paneInner    region
	status       region
	footer       region
}

func calculateLayout(width, height int) layout {
	if width < minimumWidth || height < minimumHeight {
		return layout{}
	}
	return layout{
		valid:        true,
		header:       region{0, 0, width, 1},
		sidebar:      region{0, 1, sidebarWidth, height - 3},
		sidebarInner: region{1, 2, sidebarWidth - 2, height - 5},
		pane:         region{sidebarWidth, 1, width - sidebarWidth, height - 3},
		paneInner:    region{sidebarWidth + 1, 2, width - sidebarWidth - 2, height - 5},
		status:       region{0, height - 2, width, 1},
		footer:       region{0, height - 1, width, 1},
	}
}

func (m *model) paneSize() (int, int, bool) {
	l := calculateLayout(m.width, m.height)
	return l.paneInner.width, l.paneInner.height, l.valid
}
