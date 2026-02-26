package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	deployID      string
	deployService string
	deployFormat  string
)

var deployCmd = &cobra.Command{
	Use:   "deploy <project>",
	Short: "Show details of a specific deployment",
	Args:  cobra.ExactArgs(1),
	RunE:  runDeploy,
}

func init() {
	deployCmd.Flags().StringVar(&deployID, "id", "", "Deployment ID (required)")
	deployCmd.Flags().StringVar(&deployService, "service", "", "Service name (required)")
	deployCmd.Flags().StringVar(&deployFormat, "format", "", "Output format (json)")
	deployCmd.MarkFlagRequired("id")
	deployCmd.MarkFlagRequired("service")
	rootCmd.AddCommand(deployCmd)
}

func runDeploy(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	key, err := config.LoadOrCreateKey()
	if err != nil {
		return fmt.Errorf("load encryption key: %w", err)
	}

	resolved, err := resolveService(cfg, key, args[0], deployService)
	if err != nil {
		return err
	}

	deploy, err := resolved.Platform.GetDeployment(deployID)
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}

	if deployFormat == "json" {
		data, err := json.MarshalIndent(deploy, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println(ui.ProjectTitleStyle.Render(fmt.Sprintf("%s/%s", args[0], deployService)))
	fmt.Println()
	fmt.Printf("  Deploy ID:  %s\n", deploy.ID)
	fmt.Printf("  Status:     %s\n", ui.FormatStatus(deploy.Status))
	if deploy.Commit != "" {
		fmt.Printf("  Commit:     %s\n", ui.FormatCommit(deploy.Commit))
	}
	if deploy.Message != "" {
		fmt.Printf("  Message:    %s\n", deploy.Message)
	}
	if !deploy.CreatedAt.IsZero() {
		fmt.Printf("  Created:    %s (%s)\n", deploy.CreatedAt.Format("2006-01-02 15:04:05"), ui.TimeAgo(deploy.CreatedAt))
	}
	if deploy.URL != "" {
		fmt.Printf("  URL:        %s\n", deploy.URL)
	}
	fmt.Println()

	return nil
}
