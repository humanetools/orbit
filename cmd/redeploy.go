package cmd

import (
	"fmt"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var redeployService string

var redeployCmd = &cobra.Command{
	Use:   "redeploy <project>",
	Short: "Redeploy a service",
	Long: `Trigger a redeployment for a service.

  orbit redeploy myshop --service api`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRedeploy,
}

func init() {
	redeployCmd.Flags().StringVar(&redeployService, "service", "", "Service name (required)")
	redeployCmd.MarkFlagRequired("service")
	rootCmd.AddCommand(redeployCmd)
}

func runRedeploy(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	key, err := config.LoadOrCreateKey()
	if err != nil {
		return fmt.Errorf("load encryption key: %w", err)
	}

	projectName := ""
	if len(args) > 0 {
		projectName = args[0]
	} else {
		projectName = cfg.DefaultProject
	}

	resolved, err := resolveService(cfg, key, projectName, redeployService)
	if err != nil {
		return err
	}

	fmt.Printf("  Redeploying %s/%s (%s)... ", projectName, resolved.Entry.Name, resolved.Entry.Platform)

	deploy, err := resolved.Platform.Redeploy(resolved.Entry.ID)
	if err != nil {
		fmt.Println(ui.ErrorStyle.Render("failed"))
		return fmt.Errorf("redeploy failed: %w", err)
	}

	fmt.Println(ui.HealthyStyle.Render("triggered"))
	fmt.Printf("\n  %s Redeployment started\n", ui.IconDeploy)
	fmt.Printf("  Deploy ID: %s\n", deploy.ID)
	fmt.Printf("  Status:    %s\n", ui.FormatStatus(deploy.Status))
	fmt.Printf("\n  Track progress: orbit watch %s --service %s\n", projectName, redeployService)

	return nil
}
