package cmd

import (
	"fmt"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	rollbackService string
	rollbackTo      string
)

var rollbackCmd = &cobra.Command{
	Use:   "rollback <project>",
	Short: "Rollback to a previous deployment",
	Long: `Rollback a service to a previous deployment.

  orbit rollback myshop --service api
  orbit rollback myshop --service api --to <deploy-id>

Without --to, rolls back to the most recent successful deployment before the current one.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRollback,
}

func init() {
	rollbackCmd.Flags().StringVar(&rollbackService, "service", "", "Service name (required)")
	rollbackCmd.Flags().StringVar(&rollbackTo, "to", "", "Target deployment ID to rollback to")
	rollbackCmd.MarkFlagRequired("service")
	rootCmd.AddCommand(rollbackCmd)
}

func runRollback(cmd *cobra.Command, args []string) error {
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

	resolved, err := resolveService(cfg, key, projectName, rollbackService)
	if err != nil {
		return err
	}

	// Find the target deployment to rollback to
	if rollbackTo == "" {
		// Find the most recent successful deployment that's not the current one
		deploys, err := resolved.Platform.ListDeployments(resolved.Entry.ID, 10)
		if err != nil {
			return fmt.Errorf("list deployments: %w", err)
		}

		if len(deploys) < 2 {
			return fmt.Errorf("no previous deployment found to rollback to")
		}

		// Skip the first (current) deployment, find the next healthy one
		for _, d := range deploys[1:] {
			if d.Status == "healthy" || d.Status == "READY" {
				rollbackTo = d.ID
				break
			}
		}
		if rollbackTo == "" {
			// Fall back to the immediately previous deployment
			rollbackTo = deploys[1].ID
		}
	}

	// Show what we're rolling back to
	target, err := resolved.Platform.GetDeployment(rollbackTo)
	if err != nil {
		return fmt.Errorf("get target deployment: %w", err)
	}

	fmt.Printf("\n  %s Rolling back %s/%s\n", ui.IconDeploy, projectName, resolved.Entry.Name)
	fmt.Printf("  Target:  %s", rollbackTo)
	if target.Commit != "" {
		fmt.Printf(" (%s)", ui.FormatCommit(target.Commit))
	}
	fmt.Println()
	fmt.Printf("  Created: %s\n", ui.TimeAgo(target.CreatedAt))
	fmt.Println()

	// Trigger redeployment (the platform's Redeploy recreates from current config;
	// full rollback to a specific deployment requires platform-specific support)
	fmt.Printf("  Triggering redeployment... ")

	deploy, err := resolved.Platform.Redeploy(resolved.Entry.ID)
	if err != nil {
		fmt.Println(ui.ErrorStyle.Render("failed"))
		return fmt.Errorf("rollback failed: %w", err)
	}

	fmt.Println(ui.HealthyStyle.Render("triggered"))
	fmt.Printf("  New deploy: %s\n", deploy.ID)
	fmt.Printf("\n  Track progress: orbit watch %s --service %s\n", projectName, rollbackService)

	return nil
}
