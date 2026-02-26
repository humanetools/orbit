package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var projectAutoDiscover bool

var projectCmd = &cobra.Command{
	Use:   "project [name]",
	Short: "Manage projects (create, show, delete)",
	Long: `Manage Orbit projects.

  orbit project <name>                Show project details
  orbit project create <name>         Create a new project
  orbit project create <name> --auto  Create and auto-discover services
  orbit project delete <name>         Delete a project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProjectShow,
}

var projectCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectCreate,
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectDelete,
}

func init() {
	projectCreateCmd.Flags().BoolVar(&projectAutoDiscover, "auto", false, "Auto-discover services from connected platforms")
	projectCmd.AddCommand(projectCreateCmd)
	projectCmd.AddCommand(projectDeleteCmd)
	rootCmd.AddCommand(projectCmd)
}

func runProjectCreate(cmd *cobra.Command, args []string) error {
	name := strings.ToLower(args[0])

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if _, exists := cfg.Projects[name]; exists {
		return fmt.Errorf("project %q already exists", name)
	}

	proj := config.ProjectConfig{
		Topology: []config.ServiceEntry{},
	}

	if projectAutoDiscover {
		key, err := config.LoadOrCreateKey()
		if err != nil {
			return fmt.Errorf("load encryption key: %w", err)
		}

		tokens := make(map[string]string)
		for pName, pc := range cfg.Platforms {
			token, err := config.Decrypt(key, pc.Token)
			if err != nil {
				fmt.Printf("  %s skipping %s: %s\n", ui.IconWarning, pName, err)
				continue
			}
			tokens[pName] = token
		}

		if len(tokens) == 0 {
			return fmt.Errorf("no connected platforms\nRun: orbit connect <platform>")
		}

		fmt.Printf("  Discovering services... ")
		discovered, errMap := platform.DiscoverAll(tokens)
		for pName, dErr := range errMap {
			fmt.Printf("\n  %s %s: %s", ui.IconWarning, pName, dErr)
		}

		if len(discovered) == 0 {
			fmt.Println(ui.MutedStyle.Render("none found"))
		} else {
			fmt.Println(ui.HealthyStyle.Render(fmt.Sprintf("%d found", len(discovered))))
			for _, svc := range discovered {
				proj.Topology = append(proj.Topology, config.ServiceEntry{
					Name:     svc.Name,
					Platform: svc.Platform,
					ID:       svc.ID,
				})
			}
		}
	}

	cfg.Projects[name] = proj

	// Set as default if it's the only project
	if len(cfg.Projects) == 1 {
		cfg.DefaultProject = name
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("\n%s Project %s created", ui.IconSuccess, ui.ProjectTitleStyle.Render(name))
	if len(proj.Topology) > 0 {
		fmt.Printf(" with %d services", len(proj.Topology))
	}
	fmt.Println()

	return nil
}

func runProjectShow(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	proj, err := resolveProject(cfg, name)
	if err != nil {
		return err
	}

	// Project name
	label := ui.ProjectTitleStyle.Render(name)
	if name == cfg.DefaultProject {
		label += ui.HealthyStyle.Render(" (default)")
	}
	fmt.Printf("\n  %s\n", label)

	// Service count
	fmt.Printf("  %s\n", ui.MutedStyle.Render(fmt.Sprintf("%d services", len(proj.Topology))))

	// Platforms used
	platforms := make(map[string]bool)
	for _, svc := range proj.Topology {
		platforms[svc.Platform] = true
	}
	platList := make([]string, 0, len(platforms))
	for p := range platforms {
		platList = append(platList, p)
	}
	sort.Strings(platList)
	if len(platList) > 0 {
		fmt.Printf("  Platforms: %s\n", ui.MutedStyle.Render(strings.Join(platList, ", ")))
	}

	// Topology
	if len(proj.Topology) > 0 {
		fmt.Printf("\n  Topology:\n")
		for i, svc := range proj.Topology {
			arrow := ""
			if i < len(proj.Topology)-1 {
				arrow = " →"
			}
			fmt.Printf("    %s %s %s%s\n",
				ui.HealthyStyle.Render(svc.Name),
				ui.MutedStyle.Render(fmt.Sprintf("(%s: %s)", svc.Platform, svc.ID)),
				"",
				ui.MutedStyle.Render(arrow))
		}
	}

	fmt.Println()
	return nil
}

func runProjectDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if _, ok := cfg.Projects[name]; !ok {
		return fmt.Errorf("project %q not found\nAvailable projects: %s", name, projectNames(cfg))
	}

	// Confirmation prompt
	fmt.Printf("  Delete project %s? This cannot be undone. [y/N] ", ui.ProjectTitleStyle.Render(name))
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		fmt.Println("  Cancelled.")
		return nil
	}

	delete(cfg.Projects, name)

	if cfg.DefaultProject == name {
		cfg.DefaultProject = ""
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("  %s Project %s deleted.\n", ui.IconSuccess, name)
	return nil
}
