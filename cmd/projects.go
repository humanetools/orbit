package cmd

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/lipgloss"
	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List all projects",
	RunE:  runProjects,
}

func init() {
	rootCmd.AddCommand(projectsCmd)
}

func runProjects(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(cfg.Projects) == 0 {
		fmt.Println(ui.MutedStyle.Render("No projects configured. Run 'orbit init' to get started."))
		return nil
	}

	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	defaultMark := lipgloss.NewStyle().Foreground(ui.ColorHealthy).Render(" (default)")

	for _, name := range names {
		proj := cfg.Projects[name]
		label := ui.ProjectTitleStyle.Render(name)
		if name == cfg.DefaultProject {
			label += defaultMark
		}

		svcCount := fmt.Sprintf("%d services", len(proj.Topology))

		platforms := make(map[string]bool)
		for _, svc := range proj.Topology {
			platforms[svc.Platform] = true
		}
		platList := make([]string, 0, len(platforms))
		for p := range platforms {
			platList = append(platList, p)
		}
		sort.Strings(platList)

		fmt.Printf("  %s  %s  %s\n", label, ui.MutedStyle.Render(svcCount), ui.MutedStyle.Render(fmt.Sprintf("%v", platList)))
	}

	return nil
}
