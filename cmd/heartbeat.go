package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	heartbeatRunSvc   string
	heartbeatDaemon   bool
)

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat <project>",
	Short: "Manage heartbeat pings to prevent cold starts",
	Long: `Register, view, or remove heartbeat health checks for services.

  orbit heartbeat myshop                                        Show heartbeat status
  orbit heartbeat myshop --service api --url https://url/health  Register heartbeat
  orbit heartbeat myshop --service api --interval 10s-40s        Set random interval range
  orbit heartbeat myshop --service api --remove                  Remove heartbeat

When viewing, each configured URL is pinged to show current response time.`,
	Args: cobra.ExactArgs(1),
	RunE: runHeartbeat,
}

var heartbeatRunCmd = &cobra.Command{
	Use:   "run <project>",
	Short: "Run heartbeat daemon (Ctrl+C to stop)",
	Long: `Start a daemon that periodically pings registered heartbeat URLs.

  orbit heartbeat run myshop                  Ping all services
  orbit heartbeat run myshop --service api    Ping specific service only
  orbit heartbeat run myshop --daemon         Run in background

Interval supports random ranges (e.g. 10s-40s) for bot detection avoidance.`,
	Args: cobra.ExactArgs(1),
	RunE: runHeartbeatDaemon,
}

var heartbeatStopCmd = &cobra.Command{
	Use:   "stop <project>",
	Short: "Stop a background heartbeat daemon",
	Args:  cobra.ExactArgs(1),
	RunE:  stopHeartbeatDaemon,
}

func init() {
	heartbeatCmd.Flags().StringVar(&heartbeatService, "service", "", "Service name")
	heartbeatCmd.Flags().StringVar(&heartbeatURL, "url", "", "Health check URL")
	heartbeatCmd.Flags().StringVar(&heartbeatInterval, "interval", "5m", "Ping interval (e.g. 5m, 30s, 10s-40s)")
	heartbeatCmd.Flags().BoolVar(&heartbeatRemove, "remove", false, "Remove heartbeat for a service")

	heartbeatRunCmd.Flags().StringVar(&heartbeatRunSvc, "service", "", "Ping specific service only")
	heartbeatRunCmd.Flags().BoolVarP(&heartbeatDaemon, "daemon", "d", false, "Run in background")
	heartbeatCmd.AddCommand(heartbeatRunCmd)
	heartbeatCmd.AddCommand(heartbeatStopCmd)

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

// parseInterval parses interval strings like "5m" (fixed) or "10s-40s" (random range).
func parseInterval(s string) (min, max time.Duration, err error) {
	if idx := strings.Index(s, "-"); idx > 0 {
		minStr := s[:idx]
		maxStr := s[idx+1:]
		min, err = time.ParseDuration(minStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid min duration %q: %w", minStr, err)
		}
		max, err = time.ParseDuration(maxStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid max duration %q: %w", maxStr, err)
		}
		if min > max {
			return 0, 0, fmt.Errorf("min (%s) must be <= max (%s)", min, max)
		}
		return min, max, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, d, nil
}

func randomDuration(min, max time.Duration) time.Duration {
	if min == max {
		return min
	}
	return min + time.Duration(rand.Int63n(int64(max-min)))
}

func heartbeatPidPath(project string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".orbit", fmt.Sprintf("heartbeat-%s.pid", project))
}

func heartbeatLogPath(project string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".orbit", fmt.Sprintf("heartbeat-%s.log", project))
}

func stopHeartbeatDaemon(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	pidFile := heartbeatPidPath(projectName)

	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("no running daemon for project %q", projectName)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(pidFile)
		return fmt.Errorf("invalid PID file, removed")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidFile)
		return fmt.Errorf("process %d not found, removed stale PID file", pid)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		os.Remove(pidFile)
		return fmt.Errorf("process %d not running, removed stale PID file", pid)
	}

	os.Remove(pidFile)
	fmt.Printf("  %s Heartbeat daemon stopped (PID %d)\n", ui.IconSuccess, pid)
	return nil
}

func runHeartbeatDaemon(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	// --daemon: fork self in background
	if heartbeatDaemon {
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable: %w", err)
		}

		forkArgs := []string{"heartbeat", "run", projectName}
		if heartbeatRunSvc != "" {
			forkArgs = append(forkArgs, "--service", heartbeatRunSvc)
		}

		logFile, err := os.OpenFile(heartbeatLogPath(projectName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}

		child := exec.Command(exePath, forkArgs...)
		child.Stdout = logFile
		child.Stderr = logFile
		setSysProcAttr(child)

		if err := child.Start(); err != nil {
			logFile.Close()
			return fmt.Errorf("start daemon: %w", err)
		}
		logFile.Close()

		if err := os.WriteFile(heartbeatPidPath(projectName), []byte(strconv.Itoa(child.Process.Pid)), 0644); err != nil {
			return fmt.Errorf("write PID file: %w", err)
		}

		fmt.Printf("  %s Heartbeat daemon started in background (PID %d)\n", ui.IconSuccess, child.Process.Pid)
		fmt.Printf("  Log: %s\n", heartbeatLogPath(projectName))
		fmt.Printf("  Stop: orbit heartbeat stop %s\n", projectName)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	proj, ok := cfg.Projects[projectName]
	if !ok {
		return fmt.Errorf("project %q not found\nAvailable projects: %s", projectName, projectNames(cfg))
	}

	type target struct {
		name     string
		url      string
		min, max time.Duration
	}

	var targets []target
	for _, svc := range proj.Topology {
		if svc.HeartbeatURL == "" {
			continue
		}
		if heartbeatRunSvc != "" && svc.Name != heartbeatRunSvc {
			continue
		}
		interval := svc.HeartbeatInterval
		if interval == "" {
			interval = "5m"
		}
		mn, mx, err := parseInterval(interval)
		if err != nil {
			return fmt.Errorf("service %q: %w", svc.Name, err)
		}
		targets = append(targets, target{name: svc.Name, url: svc.HeartbeatURL, min: mn, max: mx})
	}

	if len(targets) == 0 {
		if heartbeatRunSvc != "" {
			return fmt.Errorf("no heartbeat configured for service %q in project %q", heartbeatRunSvc, projectName)
		}
		return fmt.Errorf("no heartbeats configured in project %q\nRegister: orbit heartbeat %s --service <name> --url <health-url>", projectName, projectName)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("\n  %s Heartbeat daemon started for %s (%d services)\n",
		ui.IconSuccess, ui.ProjectTitleStyle.Render(projectName), len(targets))
	fmt.Printf("  Press Ctrl+C to stop.\n\n")

	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			for {
				respTime, err := pingURL(t.url)
				now := time.Now().Format("15:04:05")
				if err != nil {
					fmt.Printf("  [%s] %-12s  %s %s\n", now,
						t.name, ui.ErrorStyle.Render("✗"), ui.ErrorStyle.Render(err.Error()))
				} else {
					fmt.Printf("  [%s] %-12s  %s %dms\n", now,
						t.name, ui.HealthyStyle.Render("✓"), respTime)
				}

				wait := randomDuration(t.min, t.max)
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return
				}
			}
		}(t)
	}

	<-ctx.Done()
	fmt.Printf("\n  Stopping...\n")
	wg.Wait()
	fmt.Printf("  %s Heartbeat daemon stopped.\n\n", ui.IconSuccess)
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
