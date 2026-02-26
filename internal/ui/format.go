package ui

import (
	"fmt"
	"time"
)

// Dash is the placeholder for missing or unavailable values.
const Dash = "—"

// TimeAgo returns a human-readable relative time string.
func TimeAgo(t time.Time) string {
	if t.IsZero() {
		return Dash
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// FormatCommit returns the first 7 characters of a commit SHA, or Dash if empty.
func FormatCommit(sha string) string {
	if sha == "" {
		return Dash
	}
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// FormatResponseTime formats a response time in milliseconds.
func FormatResponseTime(ms int) string {
	if ms <= 0 {
		return Dash
	}
	return fmt.Sprintf("%dms", ms)
}

// FormatCPU formats a CPU usage percentage.
func FormatCPU(pct float64) string {
	if pct < 0 {
		return Dash
	}
	return fmt.Sprintf("%.1f%%", pct)
}

// FormatMemory formats a memory usage percentage.
func FormatMemory(pct float64) string {
	if pct < 0 {
		return Dash
	}
	return fmt.Sprintf("%.1f%%", pct)
}

// FormatInstances formats current/max instance counts.
func FormatInstances(current, max int) string {
	if current < 0 && max < 0 {
		return Dash
	}
	if max <= 0 {
		return fmt.Sprintf("%d/auto", current)
	}
	return fmt.Sprintf("%d/%d", current, max)
}
