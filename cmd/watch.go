package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

// Watch exit codes
const (
	exitSuccess      = 0
	exitFailed       = 1
	exitNoDeployment = 2
	exitTimeout      = 3
)

// Detection phase timeout — how long to wait for a new deployment before giving up.
const detectTimeout = 60 * time.Second

var (
	watchService string
	watchAll     bool
	watchTimeout int
	watchFormat  string
)

var watchCmd = &cobra.Command{
	Use:   "watch <project>",
	Short: "Watch for new deployments and track their progress",
	Long: `Watch a service for new deployments after a git push.

  orbit watch myshop --service api
  orbit watch myshop --service api --timeout 300
  orbit watch myshop --service api --format json
  orbit watch myshop --service api,frontend
  orbit watch myshop --all

Exit codes:
  0  Deploy successful (healthy)
  1  Build/deploy failed
  2  No new deployment detected
  3  Timeout (deploy still in progress)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().StringVar(&watchService, "service", "", "Service name(s), comma-separated")
	watchCmd.Flags().BoolVar(&watchAll, "all", false, "Watch all services in the project")
	watchCmd.Flags().IntVar(&watchTimeout, "timeout", 300, "Maximum wait time in seconds")
	watchCmd.Flags().StringVar(&watchFormat, "format", "", "Output format (json)")
	rootCmd.AddCommand(watchCmd)
}

type serviceContext struct {
	resolved *resolvedService
	name     string
}

// watchResult holds the outcome of watching a single service.
type watchResult struct {
	ServiceName string
	Platform    string
	ExitCode    int
	DeployID    string
	Commit      string
	Message     string
	Duration    time.Duration
	Status      string
	Phase       string
	URL         string
	Error       string
	Logs        []string
	WaitedSec   int
}

func runWatch(cmd *cobra.Command, args []string) error {
	if watchService == "" && !watchAll {
		return fmt.Errorf("specify --service <name> or --all")
	}

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

	proj, err := resolveProject(cfg, projectName)
	if err != nil {
		return err
	}

	// Determine which services to watch
	var serviceNames []string
	if watchAll {
		for _, e := range proj.Topology {
			serviceNames = append(serviceNames, e.Name)
		}
	} else {
		serviceNames = strings.Split(watchService, ",")
		for i := range serviceNames {
			serviceNames[i] = strings.TrimSpace(serviceNames[i])
		}
	}

	if len(serviceNames) == 0 {
		return fmt.Errorf("no services to watch")
	}

	// Resolve all services upfront
	var contexts []serviceContext
	for _, name := range serviceNames {
		r, err := resolveService(cfg, key, projectName, name)
		if err != nil {
			return err
		}
		contexts = append(contexts, serviceContext{resolved: r, name: name})
	}

	// Single service — simple path
	if len(contexts) == 1 {
		result := watchSingleService(contexts[0].resolved, projectName, time.Duration(watchTimeout)*time.Second)
		if watchFormat == "json" {
			printWatchJSON(result)
		}
		return exitCodeFromResult(result)
	}

	// Multiple services — parallel watch
	results := watchMultipleServices(contexts, projectName, time.Duration(watchTimeout)*time.Second)

	if watchFormat == "json" {
		printWatchMultiJSON(results)
	}

	// Determine overall exit code: failed > timeout > no_deployment > success
	worstCode := exitSuccess
	for _, r := range results {
		if r.ExitCode > worstCode {
			worstCode = r.ExitCode
		}
	}
	// Spec: if any failed → exit 1 (takes priority)
	for _, r := range results {
		if r.ExitCode == exitFailed {
			worstCode = exitFailed
			break
		}
	}

	if worstCode == exitSuccess {
		return nil
	}
	// Suppress Cobra's error printing — we already printed output
	cmd.SilenceErrors = true
	return &ExitCodeError{Code: worstCode, Msg: ""}
}

func watchSingleService(resolved *resolvedService, projectName string, timeout time.Duration) watchResult {
	result := watchResult{
		ServiceName: resolved.Entry.Name,
		Platform:    resolved.Entry.Platform,
	}

	isJSON := watchFormat == "json"

	// Get current deployment ID
	deploys, err := resolved.Platform.ListDeployments(resolved.Entry.ID, 1)
	if err != nil {
		result.ExitCode = exitFailed
		result.Error = fmt.Sprintf("list deployments: %s", err)
		if !isJSON {
			fmt.Printf("%s Error: %s\n", ui.IconFailed, result.Error)
		}
		return result
	}

	currentDeployID := ""
	if len(deploys) > 0 {
		currentDeployID = deploys[0].ID
	}

	if !isJSON {
		fmt.Printf("%s Watching %s (%s)...", ui.IconWatch, resolved.Entry.Name, resolved.Entry.Platform)
		if currentDeployID != "" {
			fmt.Printf(" (current: %s)", shortID(currentDeployID))
		}
		fmt.Println()
	}

	// Start watching
	ch, err := resolved.Platform.WatchDeployment(resolved.Entry.ID, currentDeployID)
	if err != nil {
		result.ExitCode = exitFailed
		result.Error = fmt.Sprintf("watch: %s", err)
		if !isJSON {
			fmt.Printf("%s Error: %s\n", ui.IconFailed, result.Error)
		}
		return result
	}

	overallDeadline := time.After(timeout)
	detectDeadline := time.After(detectTimeout)
	detected := false
	startTime := time.Now()

	for {
		select {
		case <-detectDeadline:
			if !detected {
				elapsed := int(time.Since(startTime).Seconds())
				result.ExitCode = exitNoDeployment
				result.WaitedSec = elapsed
				result.Error = "No new deployment detected"
				if currentDeployID != "" {
					result.DeployID = currentDeployID
				}
				if !isJSON {
					fmt.Printf("\n%s No new deployment detected after %ds.\n", ui.IconWarning, elapsed)
					if currentDeployID != "" {
						fmt.Printf("\n  Current: %s\n", shortID(currentDeployID))
					}
					fmt.Printf("\n  Possible reasons:\n")
					fmt.Printf("  - Push didn't trigger auto-deploy (check branch settings)\n")
					fmt.Printf("  - Build queue is backed up\n")
					fmt.Printf("  - Auto-deploy is disabled for this service\n")
				}
				return result
			}

		case <-overallDeadline:
			elapsed := int(time.Since(startTime).Seconds())
			if !detected {
				result.ExitCode = exitNoDeployment
				result.WaitedSec = elapsed
				result.Error = "No new deployment detected"
			} else {
				result.ExitCode = exitTimeout
				result.Error = fmt.Sprintf("Deploy still in progress after %ds", elapsed)
			}
			if !isJSON {
				if !detected {
					fmt.Printf("\n%s No new deployment detected after %ds.\n", ui.IconWarning, elapsed)
				} else {
					fmt.Printf("\n%s Timeout! Deploy still in progress after %ds.\n", "⏰", elapsed)
					if result.DeployID != "" {
						fmt.Printf("\n  Deploy:  %s\n", shortID(result.DeployID))
						fmt.Printf("  Phase:   %s (still running)\n", result.Phase)
						fmt.Printf("\n  Continue watching: orbit watch %s --service %s\n", projectName, resolved.Entry.Name)
					}
				}
			}
			return result

		case event, ok := <-ch:
			if !ok {
				// Channel closed — should not happen without a terminal event
				if result.ExitCode == 0 && !detected {
					result.ExitCode = exitNoDeployment
					result.Error = "Watch ended unexpectedly"
				}
				return result
			}

			switch event.Phase {
			case "waiting":
				elapsed := int(time.Since(startTime).Seconds())
				if !isJSON && elapsed > 0 && elapsed%15 == 0 {
					fmt.Printf("%s Waiting... (%ds)\n", ui.IconWatch, elapsed)
				}

			case "detected":
				detected = true
				if event.Deploy != nil {
					result.DeployID = event.Deploy.ID
					result.Commit = event.Deploy.Commit
					result.Message = event.Deploy.Message
				}
				if !isJSON {
					fmt.Printf("%s New deployment detected! (%s)\n", ui.IconBuilding, shortID(result.DeployID))
					if result.Commit != "" {
						msg := result.Message
						if msg == "" {
							msg = ""
						}
						commitStr := ui.FormatCommit(result.Commit)
						if msg != "" {
							fmt.Printf("   Commit: %s %q\n", commitStr, msg)
						} else {
							fmt.Printf("   Commit: %s\n", commitStr)
						}
					}
				}

			case "building":
				result.Phase = "building"
				if !isJSON {
					elapsed := int(time.Since(startTime).Seconds())
					fmt.Printf("%s Building... (%ds)\n", ui.IconBuilding, elapsed)
				}

			case "deploying":
				result.Phase = "deploying"
				if !isJSON {
					elapsed := int(time.Since(startTime).Seconds())
					fmt.Printf("%s Deploying... (%ds)\n", ui.IconDeploy, elapsed)
				}

			case "healthcheck":
				result.Phase = "healthcheck"
				if !isJSON {
					fmt.Printf("%s Health check...\n", ui.IconHealth)
				}

			case "done":
				result.ExitCode = exitSuccess
				result.Phase = "done"
				result.Duration = time.Since(startTime)
				if event.Deploy != nil {
					result.Status = event.Deploy.Status
					result.URL = event.Deploy.URL
					if result.DeployID == "" {
						result.DeployID = event.Deploy.ID
					}
				}
				if !isJSON {
					fmt.Printf("%s Deploy successful!\n", ui.IconSuccess)
					fmt.Println()
					fmt.Printf("  Deploy:   %s\n", shortID(result.DeployID))
					if result.Commit != "" {
						fmt.Printf("  Commit:   %s\n", ui.FormatCommit(result.Commit))
					}
					fmt.Printf("  Duration: %ds\n", int(result.Duration.Seconds()))
					fmt.Printf("  Status:   %s\n", ui.FormatStatus("healthy"))
					if result.URL != "" {
						fmt.Printf("  URL:      %s\n", result.URL)
					}
				}
				return result

			case "failed":
				result.ExitCode = exitFailed
				result.Phase = event.Phase
				result.Duration = time.Since(startTime)
				if event.Error != nil {
					result.Error = event.Error.Error()
				}
				result.Logs = event.Logs
				if event.Deploy != nil {
					result.Status = event.Deploy.Status
					if result.DeployID == "" {
						result.DeployID = event.Deploy.ID
					}
				}
				if !isJSON {
					fmt.Printf("%s Build failed! (%ds)\n", ui.IconFailed, int(result.Duration.Seconds()))
					fmt.Println()
					fmt.Printf("  Deploy:  %s\n", shortID(result.DeployID))
					if result.Commit != "" {
						fmt.Printf("  Commit:  %s\n", ui.FormatCommit(result.Commit))
					}
					fmt.Printf("  Phase:   %s\n", result.Phase)
					if len(result.Logs) > 0 {
						fmt.Println()
						fmt.Println("  ── Error Log ──────────────────────────────────")
						for _, l := range result.Logs {
							fmt.Printf("  %s\n", l)
						}
						fmt.Println("  ────────────────────────────────────────────────")
					}
					fmt.Printf("\n  Full logs: orbit logs %s --service %s\n", projectName, resolved.Entry.Name)
				}
				return result
			}
		}
	}
}

func watchMultipleServices(contexts []serviceContext, projectName string, timeout time.Duration) []watchResult {
	results := make([]watchResult, len(contexts))
	var wg sync.WaitGroup

	isJSON := watchFormat == "json"
	var mu sync.Mutex // protects stdout for text mode

	for i, ctx := range contexts {
		wg.Add(1)
		go func(idx int, r *resolvedService, svcName string) {
			defer wg.Done()
			res := watchSingleServiceQuiet(r, timeout)
			results[idx] = res

			if !isJSON {
				mu.Lock()
				printServiceResult(projectName, svcName, res)
				mu.Unlock()
			}
		}(i, ctx.resolved, ctx.name)
	}

	wg.Wait()
	return results
}

// watchSingleServiceQuiet watches without printing — for parallel use.
func watchSingleServiceQuiet(resolved *resolvedService, timeout time.Duration) watchResult {
	result := watchResult{
		ServiceName: resolved.Entry.Name,
		Platform:    resolved.Entry.Platform,
	}

	deploys, err := resolved.Platform.ListDeployments(resolved.Entry.ID, 1)
	if err != nil {
		result.ExitCode = exitFailed
		result.Error = fmt.Sprintf("list deployments: %s", err)
		return result
	}

	currentDeployID := ""
	if len(deploys) > 0 {
		currentDeployID = deploys[0].ID
	}

	ch, err := resolved.Platform.WatchDeployment(resolved.Entry.ID, currentDeployID)
	if err != nil {
		result.ExitCode = exitFailed
		result.Error = fmt.Sprintf("watch: %s", err)
		return result
	}

	overallDeadline := time.After(timeout)
	detectDeadline := time.After(detectTimeout)
	detected := false
	startTime := time.Now()

	for {
		select {
		case <-detectDeadline:
			if !detected {
				result.ExitCode = exitNoDeployment
				result.WaitedSec = int(time.Since(startTime).Seconds())
				result.Error = "No new deployment detected"
				return result
			}

		case <-overallDeadline:
			elapsed := int(time.Since(startTime).Seconds())
			if !detected {
				result.ExitCode = exitNoDeployment
				result.WaitedSec = elapsed
				result.Error = "No new deployment detected"
			} else {
				result.ExitCode = exitTimeout
				result.Error = fmt.Sprintf("Deploy still in progress after %ds", elapsed)
			}
			return result

		case event, ok := <-ch:
			if !ok {
				if result.ExitCode == 0 && !detected {
					result.ExitCode = exitNoDeployment
					result.Error = "Watch ended unexpectedly"
				}
				return result
			}

			switch event.Phase {
			case "detected":
				detected = true
				if event.Deploy != nil {
					result.DeployID = event.Deploy.ID
					result.Commit = event.Deploy.Commit
					result.Message = event.Deploy.Message
				}
			case "building":
				result.Phase = "building"
			case "deploying":
				result.Phase = "deploying"
			case "healthcheck":
				result.Phase = "healthcheck"
			case "done":
				result.ExitCode = exitSuccess
				result.Phase = "done"
				result.Duration = time.Since(startTime)
				if event.Deploy != nil {
					result.Status = event.Deploy.Status
					result.URL = event.Deploy.URL
				}
				return result
			case "failed":
				result.ExitCode = exitFailed
				result.Phase = event.Phase
				result.Duration = time.Since(startTime)
				if event.Error != nil {
					result.Error = event.Error.Error()
				}
				result.Logs = event.Logs
				return result
			}
		}
	}
}

func printServiceResult(projectName, svcName string, r watchResult) {
	fmt.Printf("\n── %s/%s (%s) ", projectName, svcName, r.Platform)
	switch r.ExitCode {
	case exitSuccess:
		fmt.Println(ui.HealthyStyle.Render("SUCCESS"))
		fmt.Printf("  Deploy: %s  Duration: %ds\n", shortID(r.DeployID), int(r.Duration.Seconds()))
	case exitFailed:
		fmt.Println(ui.ErrorStyle.Render("FAILED"))
		if r.Error != "" {
			fmt.Printf("  %s\n", r.Error)
		}
	case exitNoDeployment:
		fmt.Println(ui.WarningStyle.Render("NO DEPLOYMENT"))
		fmt.Printf("  Waited %ds, no new deployment detected\n", r.WaitedSec)
	case exitTimeout:
		fmt.Println(ui.WarningStyle.Render("TIMEOUT"))
		fmt.Printf("  Phase: %s (still running)\n", r.Phase)
	}
}

// --- JSON output ---

type watchJSON struct {
	Result          string   `json:"result"`
	Service         string   `json:"service,omitempty"`
	Platform        string   `json:"platform,omitempty"`
	DeployID        string   `json:"deploy_id,omitempty"`
	Commit          string   `json:"commit,omitempty"`
	DurationSec     int      `json:"duration_sec,omitempty"`
	Status          string   `json:"status,omitempty"`
	Phase           string   `json:"phase,omitempty"`
	URL             string   `json:"url,omitempty"`
	Error           string   `json:"error,omitempty"`
	Logs            []string `json:"logs,omitempty"`
	CurrentDeployID string   `json:"current_deploy_id,omitempty"`
	WaitedSec       int      `json:"waited_sec,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	ElapsedSec      int      `json:"elapsed_sec,omitempty"`
}

func resultToJSON(r watchResult) watchJSON {
	j := watchJSON{
		Service:  r.ServiceName,
		Platform: r.Platform,
		DeployID: r.DeployID,
		Commit:   r.Commit,
		Status:   r.Status,
		URL:      r.URL,
	}

	switch r.ExitCode {
	case exitSuccess:
		j.Result = "success"
		j.DurationSec = int(r.Duration.Seconds())
		if j.Status == "" {
			j.Status = "healthy"
		}
	case exitFailed:
		j.Result = "failed"
		j.DurationSec = int(r.Duration.Seconds())
		j.Phase = r.Phase
		j.Error = r.Error
		j.Logs = r.Logs
	case exitNoDeployment:
		j.Result = "no_deployment"
		j.CurrentDeployID = r.DeployID
		j.DeployID = ""
		j.WaitedSec = r.WaitedSec
		j.Reason = "No new deployment detected"
	case exitTimeout:
		j.Result = "timeout"
		j.Phase = r.Phase
		j.ElapsedSec = int(r.Duration.Seconds())
		if j.ElapsedSec == 0 {
			j.ElapsedSec = r.WaitedSec
		}
	}

	return j
}

func printWatchJSON(r watchResult) {
	j := resultToJSON(r)
	data, _ := json.MarshalIndent(j, "", "  ")
	fmt.Println(string(data))
}

func printWatchMultiJSON(results []watchResult) {
	var out []watchJSON
	for _, r := range results {
		out = append(out, resultToJSON(r))
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(data))
}

// --- Helpers ---

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func exitCodeFromResult(r watchResult) error {
	if r.ExitCode == exitSuccess {
		return nil
	}
	return &ExitCodeError{Code: r.ExitCode, Msg: ""}
}
