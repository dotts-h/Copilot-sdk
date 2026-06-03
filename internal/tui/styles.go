package tui

import "github.com/charmbracelet/lipgloss"

// Palette holds the colors my-orchestra renders with. The fusion aesthetic
// borrows Copilot's calm slate chrome and Claude's warm accent.
type Palette struct {
	Accent    lipgloss.Color
	Accent2   lipgloss.Color
	Fg        lipgloss.Color
	Dim       lipgloss.Color
	Subtle    lipgloss.Color
	Bg        lipgloss.Color
	Panel     lipgloss.Color
	Good      lipgloss.Color
	Warn      lipgloss.Color
	Bad       lipgloss.Color
	UserText  lipgloss.Color
	AgentText lipgloss.Color
}

// DarkPalette is the default dark theme.
func DarkPalette() Palette {
	return Palette{
		Accent:    lipgloss.Color("#d98c5f"), // warm terracotta (Claude-ish)
		Accent2:   lipgloss.Color("#6ea8fe"), // copilot blue
		Fg:        lipgloss.Color("#e6e6e6"),
		Dim:       lipgloss.Color("#9aa0a6"),
		Subtle:    lipgloss.Color("#5f6368"),
		Bg:        lipgloss.Color("#1b1d1e"),
		Panel:     lipgloss.Color("#26282a"),
		Good:      lipgloss.Color("#7ee787"),
		Warn:      lipgloss.Color("#e3b341"),
		Bad:       lipgloss.Color("#f85149"),
		UserText:  lipgloss.Color("#6ea8fe"),
		AgentText: lipgloss.Color("#d98c5f"),
	}
}

// Styles is the precomputed set of lipgloss styles derived from a Palette.
type Styles struct {
	P Palette

	Title       lipgloss.Style
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	Panel       lipgloss.Style
	Footer      lipgloss.Style
	User        lipgloss.Style
	Agent       lipgloss.Style
	Reasoning   lipgloss.Style
	Tool        lipgloss.Style
	Dim         lipgloss.Style
	Good        lipgloss.Style
	Warn        lipgloss.Style
	Bad         lipgloss.Style
	Key         lipgloss.Style
	Selected    lipgloss.Style
}

// NewStyles builds styles for a palette.
func NewStyles(p Palette) Styles {
	return Styles{
		P:     p,
		Title: lipgloss.NewStyle().Bold(true).Foreground(p.Accent),
		TabActive: lipgloss.NewStyle().Bold(true).
			Foreground(p.Bg).Background(p.Accent).Padding(0, 1),
		TabInactive: lipgloss.NewStyle().Foreground(p.Dim).Padding(0, 1),
		Panel: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Subtle).Padding(0, 1),
		Footer:    lipgloss.NewStyle().Foreground(p.Dim),
		User:      lipgloss.NewStyle().Bold(true).Foreground(p.UserText),
		Agent:     lipgloss.NewStyle().Bold(true).Foreground(p.AgentText),
		Reasoning: lipgloss.NewStyle().Italic(true).Foreground(p.Subtle),
		Tool:      lipgloss.NewStyle().Foreground(p.Accent2),
		Dim:       lipgloss.NewStyle().Foreground(p.Dim),
		Good:      lipgloss.NewStyle().Foreground(p.Good),
		Warn:      lipgloss.NewStyle().Foreground(p.Warn),
		Bad:       lipgloss.NewStyle().Foreground(p.Bad),
		Key:       lipgloss.NewStyle().Bold(true).Foreground(p.Accent2),
		Selected:  lipgloss.NewStyle().Bold(true).Foreground(p.Bg).Background(p.Accent2).Padding(0, 1),
	}
}

// bar renders a horizontal progress/usage bar of the given width in [0,1].
func bar(s Styles, frac float64, width int) string {
	if width < 4 {
		width = 4
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	col := s.Good
	switch {
	case frac >= 1:
		col = s.Bad
	case frac >= 0.8:
		col = s.Warn
	}
	out := col.Render(repeat("█", filled))
	out += s.Dim.Render(repeat("░", width-filled))
	return out
}

func repeat(r string, n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, 0, n*len(r))
	for i := 0; i < n; i++ {
		b = append(b, r...)
	}
	return string(b)
}
