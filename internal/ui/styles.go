package ui

import "github.com/charmbracelet/lipgloss"

// Status icons
const (
	IconHealthy  = "✓"
	IconWarning  = "⚠"
	IconError    = "✗"
	IconSleeping = "⏳"
	IconBuilding = "🔨"
	IconDeploy   = "📦"
	IconWatch    = "⏳"
	IconSuccess  = "✅"
	IconFailed   = "❌"
	IconRocket   = "🚀"
	IconHealth   = "🏥"
)

// Status colors
var (
	ColorHealthy  = lipgloss.Color("#22c55e") // green
	ColorWarning  = lipgloss.Color("#eab308") // yellow
	ColorError    = lipgloss.Color("#ef4444") // red
	ColorSleeping = lipgloss.Color("#6b7280") // gray
	ColorPrimary  = lipgloss.Color("#818cf8") // indigo
	ColorMuted    = lipgloss.Color("#9ca3af") // gray-400
)

// Status text styles
var (
	HealthyStyle  = lipgloss.NewStyle().Foreground(ColorHealthy).Bold(true)
	WarningStyle  = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	ErrorStyle    = lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	SleepingStyle = lipgloss.NewStyle().Foreground(ColorSleeping)
	MutedStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
)

// Table styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			PaddingRight(2)

	CellStyle = lipgloss.NewStyle().
			PaddingRight(2)

	ViolationStyle = lipgloss.NewStyle().
			Foreground(ColorWarning).
			PaddingLeft(1)
)

// Box style for project groups
var ProjectBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(ColorPrimary).
	Padding(0, 1)

// Title style for project names
var ProjectTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(ColorPrimary)

// FormatStatus returns a styled status string with icon.
func FormatStatus(status string) string {
	switch status {
	case "healthy":
		return HealthyStyle.Render(IconHealthy + " healthy")
	case "warning", "degraded", "warn":
		return WarningStyle.Render(IconWarning + " warn")
	case "unhealthy", "error", "failed":
		return ErrorStyle.Render(IconError + " error")
	case "sleeping", "paused":
		return SleepingStyle.Render(IconSleeping + " sleep")
	default:
		return MutedStyle.Render(status)
	}
}
