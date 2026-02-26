package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
)

// ServiceResult pairs a service entry with its fetched status or error.
type ServiceResult struct {
	Entry  config.ServiceEntry
	Status *platform.ServiceStatus
	Err    error
}

// ThresholdViolation describes a metric that exceeds its threshold.
type ThresholdViolation struct {
	ServiceName string
	Metric      string
	Value       string
	Threshold   string
}

// Column widths for table rendering.
const (
	colName     = 18
	colPlatform = 10
	colStatus   = 16
	colTime     = 10
	colCommit   = 9
	colResp     = 10
	colCPU      = 8
	colMem      = 8
	colInst     = 10
)

func pad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func headerRow(cols ...string) string {
	var parts []string
	widths := []int{colName, colPlatform, colStatus, colTime, colCommit, colResp, colCPU, colMem, colInst}
	for i, c := range cols {
		w := colName
		if i < len(widths) {
			w = widths[i]
		}
		parts = append(parts, HeaderStyle.Render(pad(c, w)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func cellRow(widths []int, cells ...string) string {
	var parts []string
	for i, c := range cells {
		w := colName
		if i < len(widths) {
			w = widths[i]
		}
		parts = append(parts, CellStyle.Render(pad(c, w)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// RenderOverviewTable renders the L0 overview: all projects, all services.
func RenderOverviewTable(projectName string, results []ServiceResult) string {
	var rows []string

	header := headerRow("Service", "Platform", "Status", "Deployed", "Commit")
	rows = append(rows, header)

	for _, r := range results {
		if r.Err != nil {
			row := cellRow(
				[]int{colName, colPlatform, colStatus, colTime, colCommit},
				r.Entry.Name,
				r.Entry.Platform,
				ErrorStyle.Render(IconError+" error"),
				Dash,
				Dash,
			)
			rows = append(rows, row)
			continue
		}

		status := FormatStatus(r.Status.Status)
		deployTime := Dash
		commit := Dash
		if r.Status.LastDeploy != nil {
			deployTime = TimeAgo(r.Status.LastDeploy.CreatedAt)
			commit = FormatCommit(r.Status.LastDeploy.Commit)
		}

		row := cellRow(
			[]int{colName, colPlatform, colStatus, colTime, colCommit},
			r.Entry.Name,
			r.Entry.Platform,
			status,
			deployTime,
			commit,
		)
		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	title := ProjectTitleStyle.Render(projectName)
	box := ProjectBoxStyle.Render(content)
	return title + "\n" + box
}

// RenderDetailTable renders the L1 detail: single project with metrics.
func RenderDetailTable(projectName string, results []ServiceResult, t config.ThresholdConfig) (string, []ThresholdViolation) {
	var rows []string
	var violations []ThresholdViolation

	header := headerRow("Service", "Platform", "Status", "Response", "CPU", "Memory", "Instances")
	rows = append(rows, header)

	for _, r := range results {
		if r.Err != nil {
			row := cellRow(
				[]int{colName, colPlatform, colStatus, colResp, colCPU, colMem, colInst},
				r.Entry.Name,
				r.Entry.Platform,
				ErrorStyle.Render(IconError+" error"),
				Dash, Dash, Dash, Dash,
			)
			rows = append(rows, row)
			continue
		}

		violations = append(violations, checkThresholds(r.Entry.Name, r.Status, t)...)

		status := FormatStatus(r.Status.Status)
		resp := FormatResponseTime(r.Status.ResponseMs)
		cpu := FormatCPU(r.Status.CPU)
		mem := FormatMemory(r.Status.Memory)
		inst := FormatInstances(r.Status.Instances, r.Status.MaxInstances)

		row := cellRow(
			[]int{colName, colPlatform, colStatus, colResp, colCPU, colMem, colInst},
			r.Entry.Name,
			r.Entry.Platform,
			status,
			resp, cpu, mem, inst,
		)
		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	title := ProjectTitleStyle.Render(projectName)
	box := ProjectBoxStyle.Render(content)
	output := title + "\n" + box
	return output, violations
}

// RenderServiceDetail renders the L2 detail card for a single service.
func RenderServiceDetail(projectName string, entry config.ServiceEntry, status *platform.ServiceStatus, t config.ThresholdConfig) (string, []ThresholdViolation) {
	violations := checkThresholds(entry.Name, status, t)

	kv := func(key, value string) string {
		return HeaderStyle.Render(pad(key, 16)) + CellStyle.Render(value)
	}

	var rows []string
	rows = append(rows, kv("Service", entry.Name))
	rows = append(rows, kv("Platform", entry.Platform))
	rows = append(rows, kv("ID", entry.ID))
	rows = append(rows, kv("Status", FormatStatus(status.Status)))
	rows = append(rows, kv("Response", FormatResponseTime(status.ResponseMs)))
	rows = append(rows, kv("CPU", FormatCPU(status.CPU)))
	rows = append(rows, kv("Memory", FormatMemory(status.Memory)))
	rows = append(rows, kv("Instances", FormatInstances(status.Instances, status.MaxInstances)))

	if status.LastDeploy != nil {
		d := status.LastDeploy
		rows = append(rows, "")
		rows = append(rows, HeaderStyle.Render("Last Deploy"))
		rows = append(rows, kv("  Deploy ID", d.ID))
		rows = append(rows, kv("  Status", FormatStatus(d.Status)))
		rows = append(rows, kv("  Commit", FormatCommit(d.Commit)))
		if d.Message != "" {
			rows = append(rows, kv("  Message", d.Message))
		}
		rows = append(rows, kv("  Created", TimeAgo(d.CreatedAt)))
		if d.URL != "" {
			rows = append(rows, kv("  URL", d.URL))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	title := ProjectTitleStyle.Render(projectName + " / " + entry.Name)
	box := ProjectBoxStyle.Render(content)
	return title + "\n" + box, violations
}

// RenderViolations renders threshold violation warnings.
func RenderViolations(violations []ThresholdViolation) string {
	if len(violations) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, WarningStyle.Render(IconWarning+" Threshold Warnings"))
	for _, v := range violations {
		line := fmt.Sprintf("  %s %s: %s (threshold: %s)",
			IconWarning, v.ServiceName, v.Metric+" = "+v.Value, v.Threshold)
		lines = append(lines, ViolationStyle.Render(line))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// checkThresholds compares service metrics against configured thresholds.
func checkThresholds(name string, status *platform.ServiceStatus, t config.ThresholdConfig) []ThresholdViolation {
	var violations []ThresholdViolation

	if t.ResponseTimeMs > 0 && status.ResponseMs > t.ResponseTimeMs {
		violations = append(violations, ThresholdViolation{
			ServiceName: name,
			Metric:      "response_time",
			Value:       FormatResponseTime(status.ResponseMs),
			Threshold:   FormatResponseTime(t.ResponseTimeMs),
		})
	}
	if t.CPUPercent > 0 && status.CPU > float64(t.CPUPercent) {
		violations = append(violations, ThresholdViolation{
			ServiceName: name,
			Metric:      "cpu",
			Value:       FormatCPU(status.CPU),
			Threshold:   FormatCPU(float64(t.CPUPercent)),
		})
	}
	if t.MemoryPercent > 0 && status.Memory > float64(t.MemoryPercent) {
		violations = append(violations, ThresholdViolation{
			ServiceName: name,
			Metric:      "memory",
			Value:       FormatMemory(status.Memory),
			Threshold:   FormatMemory(float64(t.MemoryPercent)),
		})
	}
	return violations
}
