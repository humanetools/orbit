package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	scaleService string
	scaleMin     int
	scaleMax     int
	scaleType    string
)

var scaleCmd = &cobra.Command{
	Use:   "scale <project>",
	Short: "View or change service scaling",
	Long: `View or change scaling configuration for a service.

  orbit scale myshop --service api                  Show current scale
  orbit scale myshop --service api --min 3           Scale out (min instances)
  orbit scale myshop --service api --min 2 --max 8   Set min and max instances
  orbit scale myshop --service api --type small       Scale up (instance type, triggers redeploy)

Scaling is only supported for backend platforms (Koyeb).
Vercel uses automatic scaling. Supabase does not support scaling via API.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScale,
}

func init() {
	scaleCmd.Flags().StringVar(&scaleService, "service", "", "Service name (required)")
	scaleCmd.Flags().IntVar(&scaleMin, "min", 0, "Minimum number of instances")
	scaleCmd.Flags().IntVar(&scaleMax, "max", 0, "Maximum number of instances")
	scaleCmd.Flags().StringVar(&scaleType, "type", "", "Instance type (e.g. eco, small, medium, large)")
	scaleCmd.MarkFlagRequired("service")
	rootCmd.AddCommand(scaleCmd)
}

func runScale(cmd *cobra.Command, args []string) error {
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

	resolved, err := resolveService(cfg, key, projectName, scaleService)
	if err != nil {
		return err
	}

	// No flags given → show current scale info
	if scaleMin == 0 && scaleMax == 0 && scaleType == "" {
		return showScaleInfo(resolved)
	}

	// Instance type change triggers a redeploy — confirm with user
	if scaleType != "" {
		fmt.Printf("  %s Instance type change will trigger a redeployment.\n", ui.IconWarning)

		// Show current → new if we can
		if provider, ok := resolved.Platform.(platform.ScaleInfoProvider); ok {
			_, _, currentType, err := provider.GetCurrentScale(resolved.Entry.ID)
			if err == nil && currentType != "" {
				fmt.Printf("  Current: %s → New: %s\n", currentType, scaleType)
			}
		}

		fmt.Printf("  Proceed? (y/N) ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("  Cancelled.")
			return nil
		}
	}

	opts := platform.ScaleOptions{
		MinInstances: scaleMin,
		MaxInstances: scaleMax,
		InstanceType: scaleType,
	}

	fmt.Printf("  Scaling %s/%s... ", projectName, resolved.Entry.Name)

	if err := resolved.Platform.Scale(resolved.Entry.ID, opts); err != nil {
		fmt.Println(ui.ErrorStyle.Render("failed"))
		return fmt.Errorf("scale failed: %w", err)
	}

	fmt.Println(ui.HealthyStyle.Render("done"))

	// Show updated scale info
	if scaleMin > 0 || scaleMax > 0 {
		fmt.Printf("  Instances: min=%d", scaleMin)
		if scaleMax > 0 {
			fmt.Printf(", max=%d", scaleMax)
		}
		fmt.Println()
	}
	if scaleType != "" {
		fmt.Printf("  Instance type: %s\n", scaleType)
		fmt.Printf("\n  Track redeployment: orbit watch %s --service %s\n", projectName, scaleService)
	}

	return nil
}

func showScaleInfo(resolved *resolvedService) error {
	provider, ok := resolved.Platform.(platform.ScaleInfoProvider)
	if !ok {
		return fmt.Errorf("scaling info not available for %s", resolved.Entry.Platform)
	}

	min, max, instanceType, err := provider.GetCurrentScale(resolved.Entry.ID)
	if err != nil {
		return fmt.Errorf("get scale info: %w", err)
	}

	fmt.Printf("\n  %s Scale info for %s (%s)\n\n", ui.IconRocket, resolved.Entry.Name, resolved.Entry.Platform)
	if instanceType != "" {
		fmt.Printf("  Instance:  %s\n", instanceType)
	}
	fmt.Printf("  Min:       %d\n", min)
	fmt.Printf("  Max:       %d\n", max)
	fmt.Println()

	return nil
}
