package tui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent = lipgloss.AdaptiveColor{Light: "#D35400", Dark: "#F39C12"}
	colMuted  = lipgloss.AdaptiveColor{Light: "#7F8C8D", Dark: "#95A5A6"}
	colBorder = lipgloss.AdaptiveColor{Light: "#BDC3C7", Dark: "#4A4A4A"}
	colOK     = lipgloss.AdaptiveColor{Light: "#1E8449", Dark: "#2ECC71"}
	colErr    = lipgloss.AdaptiveColor{Light: "#C0392B", Dark: "#E74C3C"}
	colWarn   = lipgloss.AdaptiveColor{Light: "#B9770E", Dark: "#F1C40F"}
	colCursor = lipgloss.AdaptiveColor{Light: "#FDEBD0", Dark: "#3A2E1E"}

	styTitle    = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styHeading  = lipgloss.NewStyle().Bold(true)
	styMuted    = lipgloss.NewStyle().Foreground(colMuted)
	styOK       = lipgloss.NewStyle().Foreground(colOK)
	styErr      = lipgloss.NewStyle().Foreground(colErr)
	styWarn     = lipgloss.NewStyle().Foreground(colWarn)
	styCursor   = lipgloss.NewStyle().Background(colCursor).Bold(true)
	styCursorNF = lipgloss.NewStyle().Bold(true)
	styChosen   = lipgloss.NewStyle().Foreground(colAccent)
	styHelp     = lipgloss.NewStyle().Foreground(colMuted)

	styPane        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBorder)
	styPaneFocused = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colAccent)

	styButton        = lipgloss.NewStyle().Padding(0, 2).Foreground(colMuted).Border(lipgloss.RoundedBorder()).BorderForeground(colBorder)
	styButtonFocused = lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(colAccent).Border(lipgloss.RoundedBorder()).BorderForeground(colAccent)
	styButtonDanger  = lipgloss.NewStyle().Padding(0, 2).Bold(true).Foreground(colErr).Border(lipgloss.RoundedBorder()).BorderForeground(colErr)
)
