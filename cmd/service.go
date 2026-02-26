package cmd

import (
	"fmt"
	"strings"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	serviceAddName     string
	serviceAddPlatform string
	serviceAddID       string
	serviceRemoveName  string
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage services within a project",
	Long: `Add or remove services from a project.

  orbit service add <project> --name X --platform Y --id Z
  orbit service remove <project> --name X`,
}

var serviceAddCmd = &cobra.Command{
	Use:   "add <project>",
	Short: "Add a service to a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceAdd,
}

var serviceRemoveCmd = &cobra.Command{
	Use:   "remove <project>",
	Short: "Remove a service from a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runServiceRemove,
}

func init() {
	serviceAddCmd.Flags().StringVar(&serviceAddName, "name", "", "Service name")
	serviceAddCmd.Flags().StringVar(&serviceAddPlatform, "platform", "", "Platform (vercel, koyeb, supabase)")
	serviceAddCmd.Flags().StringVar(&serviceAddID, "id", "", "Service ID on the platform")
	serviceAddCmd.MarkFlagRequired("name")
	serviceAddCmd.MarkFlagRequired("platform")
	serviceAddCmd.MarkFlagRequired("id")

	serviceRemoveCmd.Flags().StringVar(&serviceRemoveName, "name", "", "Service name to remove")
	serviceRemoveCmd.MarkFlagRequired("name")

	serviceCmd.AddCommand(serviceAddCmd)
	serviceCmd.AddCommand(serviceRemoveCmd)
	rootCmd.AddCommand(serviceCmd)
}

func runServiceAdd(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	platName := strings.ToLower(serviceAddPlatform)

	if !platform.IsSupported(platName) {
		return fmt.Errorf("unsupported platform: %s\nSupported: vercel, koyeb, supabase", platName)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Check platform is connected
	if _, ok := cfg.Platforms[platName]; !ok {
		return fmt.Errorf("platform %q not connected\nRun: orbit connect %s", platName, platName)
	}

	proj, ok := cfg.Projects[projectName]
	if !ok {
		return fmt.Errorf("project %q not found\nAvailable projects: %s", projectName, projectNames(cfg))
	}

	// Check for duplicate service name
	for _, svc := range proj.Topology {
		if svc.Name == serviceAddName {
			return fmt.Errorf("service %q already exists in project %q", serviceAddName, projectName)
		}
	}

	proj.Topology = append(proj.Topology, config.ServiceEntry{
		Name:     serviceAddName,
		Platform: platName,
		ID:       serviceAddID,
	})

	cfg.Projects[projectName] = proj

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("  %s Service %s added to %s\n",
		ui.IconSuccess,
		ui.HealthyStyle.Render(serviceAddName),
		ui.ProjectTitleStyle.Render(projectName))
	return nil
}

func runServiceRemove(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	proj, ok := cfg.Projects[projectName]
	if !ok {
		return fmt.Errorf("project %q not found\nAvailable projects: %s", projectName, projectNames(cfg))
	}

	found := false
	filtered := make([]config.ServiceEntry, 0, len(proj.Topology))
	for _, svc := range proj.Topology {
		if svc.Name == serviceRemoveName {
			found = true
			continue
		}
		filtered = append(filtered, svc)
	}

	if !found {
		var svcNames []string
		for _, svc := range proj.Topology {
			svcNames = append(svcNames, svc.Name)
		}
		return fmt.Errorf("service %q not found in project %q\nAvailable services: %s",
			serviceRemoveName, projectName, joinNames(svcNames))
	}

	proj.Topology = filtered
	cfg.Projects[projectName] = proj

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("  %s Service %s removed from %s\n",
		ui.IconSuccess,
		serviceRemoveName,
		ui.ProjectTitleStyle.Render(projectName))
	return nil
}
