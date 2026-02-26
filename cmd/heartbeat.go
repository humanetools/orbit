package cmd

import (
	"fmt"
	"net/http"
	"time"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	heartbeatService  string
	heartbeatURL      string
	heartbeatInterval string
	heartbeatRemove   bool
)

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat <project>",
	Short: "Manage heartbeat pings to prevent cold starts",
	Long: `Register, view, or remove heartbeat health checks for services.

  orbit heartbeat myshop                                        Show heartbeat status
  orbit heartbeat myshop --service api --url https://url/health  Register heartbeat
  orbit heartbeat myshop --service api --interval 5m             Set interval (default 5m)
  orbit heartbeat myshop --service api --remove                  Remove heartbeat

When viewing, each configured URL is pinged to show current response time.`,
	Args: cobra.ExactArgs(1),
	RunE: runHeartbeat,
}

func init() {
	heartbeatCmd.Flags().StringVar(&heartbeatService, "service", "", "Service name")
	heartbeatCmd.Flags().StringVar(&heartbeatURL, "url", "", "Health check URL")
	heartbeatCmd.Flags().StringVar(&heartbeatInterval, "interval", "5m", "Ping interval (e.g. 5m, 30s)")
	heartbeatCmd.Flags().BoolVar(&heartbeatRemove, "remove", false, "Remove heartbeat for a service")
	rootCmd.AddCommand(heartbeatCmd)
}

func runHeartbeat(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	proj, ok := cfg.Projects[projectName]
	if !ok {
		return fmt.Errorf("project %q not found\nAvailable projects: %s", projectName, projectNames(cfg))
	}

	// Remove heartbeat
	if heartbeatRemove {
		if heartbeatService == "" {
			return fmt.Errorf("--service is required with --remove")
		}
		return removeHeartbeat(cfg, projectName, &proj)
	}

	// Register heartbeat
	if heartbeatURL != "" {
		if heartbeatService == "" {
			return fmt.Errorf("--service is required with --url")
		}
		return registerHeartbeat(cfg, projectName, &proj)
	}

	// Show heartbeat status
	return showHeartbeats(projectName, &proj)
}

func registerHeartbeat(cfg *config.Config, projectName string, proj *config.ProjectConfig) error {
	found := false
	for i := range proj.Topology {
		if proj.Topology[i].Name == heartbeatService {
			proj.Topology[i].HeartbeatURL = heartbeatURL
			proj.Topology[i].HeartbeatInterval = heartbeatInterval
			found = true
			break
		}
	}

	if !found {
		var svcNames []string
		for _, svc := range proj.Topology {
			svcNames = append(svcNames, svc.Name)
		}
		return fmt.Errorf("service %q not found in project %q\nAvailable services: %s",
			heartbeatService, projectName, joinNames(svcNames))
	}

	cfg.Projects[projectName] = *proj
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("  %s Heartbeat registered for %s/%s\n", ui.IconSuccess,
		ui.ProjectTitleStyle.Render(projectName),
		ui.HealthyStyle.Render(heartbeatService))
	fmt.Printf("  URL:      %s\n", heartbeatURL)
	fmt.Printf("  Interval: %s\n", heartbeatInterval)
	return nil
}

func removeHeartbeat(cfg *config.Config, projectName string, proj *config.ProjectConfig) error {
	found := false
	for i := range proj.Topology {
		if proj.Topology[i].Name == heartbeatService {
			if proj.Topology[i].HeartbeatURL == "" {
				return fmt.Errorf("no heartbeat configured for service %q", heartbeatService)
			}
			proj.Topology[i].HeartbeatURL = ""
			proj.Topology[i].HeartbeatInterval = ""
			found = true
			break
		}
	}

	if !found {
		var svcNames []string
		for _, svc := range proj.Topology {
			svcNames = append(svcNames, svc.Name)
		}
		return fmt.Errorf("service %q not found in project %q\nAvailable services: %s",
			heartbeatService, projectName, joinNames(svcNames))
	}

	cfg.Projects[projectName] = *proj
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("  %s Heartbeat removed for %s/%s\n", ui.IconSuccess,
		projectName, heartbeatService)
	return nil
}

func showHeartbeats(projectName string, proj *config.ProjectConfig) error {
	fmt.Printf("\n  %s %s\n\n", ui.ProjectTitleStyle.Render(projectName), ui.MutedStyle.Render("heartbeats"))

	hasAny := false
	for _, svc := range proj.Topology {
		if svc.HeartbeatURL == "" {
			continue
		}
		hasAny = true

		interval := svc.HeartbeatInterval
		if interval == "" {
			interval = "5m"
		}

		// Ping the URL
		respTime, err := pingURL(svc.HeartbeatURL)

		statusStr := ""
		if err != nil {
			statusStr = ui.ErrorStyle.Render(fmt.Sprintf("✗ %s", err))
		} else {
			statusStr = ui.HealthyStyle.Render(fmt.Sprintf("✓ %dms", respTime))
		}

		fmt.Printf("  %-12s  %-40s  %s  %s\n",
			ui.HealthyStyle.Render(svc.Name),
			ui.MutedStyle.Render(svc.HeartbeatURL),
			ui.MutedStyle.Render(fmt.Sprintf("every %s", interval)),
			statusStr)
	}

	if !hasAny {
		fmt.Println(ui.MutedStyle.Render("  No heartbeats configured."))
		fmt.Println(ui.MutedStyle.Render("  Register: orbit heartbeat " + projectName + " --service <name> --url <health-url>"))
	}

	fmt.Println()
	return nil
}

func pingURL(url string) (int64, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	resp, err := client.Get(url)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return 0, fmt.Errorf("unreachable")
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return elapsed, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return elapsed, nil
}
