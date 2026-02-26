package cmd

import (
	"fmt"
	"strings"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var topologySet string

var topologyCmd = &cobra.Command{
	Use:   "topology <project>",
	Short: "View or set service topology order",
	Long: `View or reorder the service topology for a project.

  orbit topology <project>                          Show current topology
  orbit topology <project> --set "frontend → api → db"  Set topology order

The --set flag accepts service names separated by "→" or "->".`,
	Args: cobra.ExactArgs(1),
	RunE: runTopology,
}

func init() {
	topologyCmd.Flags().StringVar(&topologySet, "set", "", `Topology order (e.g. "frontend → api → db")`)
	rootCmd.AddCommand(topologyCmd)
}

func runTopology(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	proj, ok := cfg.Projects[projectName]
	if !ok {
		return fmt.Errorf("project %q not found\nAvailable projects: %s", projectName, projectNames(cfg))
	}

	if topologySet != "" {
		return setTopologyOrder(cfg, projectName, &proj)
	}

	return showTopology(projectName, &proj)
}

func showTopology(projectName string, proj *config.ProjectConfig) error {
	fmt.Printf("\n  %s %s\n\n", ui.ProjectTitleStyle.Render(projectName), ui.MutedStyle.Render("topology"))

	if len(proj.Topology) == 0 {
		fmt.Println(ui.MutedStyle.Render("  No services configured."))
		fmt.Println()
		return nil
	}

	for i, svc := range proj.Topology {
		connector := "  "
		if i < len(proj.Topology)-1 {
			connector = " →"
		}

		fmt.Printf("  %s %s %s\n",
			ui.HealthyStyle.Render(svc.Name),
			ui.MutedStyle.Render(fmt.Sprintf("[%s]", svc.Platform)),
			ui.MutedStyle.Render(connector))
	}

	fmt.Println()
	return nil
}

func setTopologyOrder(cfg *config.Config, projectName string, proj *config.ProjectConfig) error {
	// Parse: split by "→" or "->"
	input := topologySet
	input = strings.ReplaceAll(input, "→", "->")
	parts := strings.Split(input, "->")

	names := make([]string, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name != "" {
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		return fmt.Errorf("no service names provided in --set value")
	}

	// Build lookup from existing services
	svcMap := make(map[string]config.ServiceEntry)
	for _, svc := range proj.Topology {
		svcMap[svc.Name] = svc
	}

	// Validate all names exist
	for _, name := range names {
		if _, ok := svcMap[name]; !ok {
			var existing []string
			for _, svc := range proj.Topology {
				existing = append(existing, svc.Name)
			}
			return fmt.Errorf("service %q not found in project %q\nAvailable services: %s",
				name, projectName, joinNames(existing))
		}
	}

	// Rebuild topology in the specified order
	reordered := make([]config.ServiceEntry, 0, len(names))
	used := make(map[string]bool)
	for _, name := range names {
		if used[name] {
			return fmt.Errorf("duplicate service %q in --set value", name)
		}
		reordered = append(reordered, svcMap[name])
		used[name] = true
	}

	// Append any services not mentioned (preserve them at the end)
	for _, svc := range proj.Topology {
		if !used[svc.Name] {
			reordered = append(reordered, svc)
		}
	}

	proj.Topology = reordered
	cfg.Projects[projectName] = *proj

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("  %s Topology updated for %s\n", ui.IconSuccess, ui.ProjectTitleStyle.Render(projectName))
	return showTopology(projectName, proj)
}
