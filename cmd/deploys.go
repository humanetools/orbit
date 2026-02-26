package cmd

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	deploysService string
	deploysLimit   int
	deploysFormat  string
)

var deploysCmd = &cobra.Command{
	Use:   "deploys <project>",
	Short: "Show deployment history",
	Long: `Show deployment history for services in a project.

  orbit deploys myshop
  orbit deploys myshop --service api
  orbit deploys myshop --service api --limit 20
  orbit deploys myshop --format json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeploys,
}

func init() {
	deploysCmd.Flags().StringVar(&deploysService, "service", "", "Show deployments for a specific service")
	deploysCmd.Flags().IntVar(&deploysLimit, "limit", 10, "Maximum number of deployments to show")
	deploysCmd.Flags().StringVar(&deploysFormat, "format", "", "Output format (json)")
	rootCmd.AddCommand(deploysCmd)
}

type deployResult struct {
	Entry       config.ServiceEntry
	Deployments []platform.Deployment
	Err         error
}

func runDeploys(cmd *cobra.Command, args []string) error {
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

	// Filter to a specific service if requested
	entries := proj.Topology
	if deploysService != "" {
		var filtered []config.ServiceEntry
		for _, e := range entries {
			if e.Name == deploysService {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name)
			}
			return fmt.Errorf("service %q not found\nAvailable services: %s", deploysService, joinNames(names))
		}
		entries = filtered
	}

	// Fetch deployments concurrently
	results := make([]deployResult, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		results[i].Entry = entry
		wg.Add(1)
		go func(idx int, e config.ServiceEntry) {
			defer wg.Done()
			pc, ok := cfg.Platforms[e.Platform]
			if !ok {
				results[idx].Err = fmt.Errorf("platform %q not connected", e.Platform)
				return
			}
			token, err := config.Decrypt(key, pc.Token)
			if err != nil {
				results[idx].Err = fmt.Errorf("decrypt token: %w", err)
				return
			}
			p, err := platform.Get(e.Platform, token)
			if err != nil {
				results[idx].Err = err
				return
			}
			deploys, err := p.ListDeployments(e.ID, deploysLimit)
			results[idx].Deployments = deploys
			results[idx].Err = err
		}(i, entry)
	}
	wg.Wait()

	if deploysFormat == "json" {
		return renderDeploysJSON(projectName, results)
	}

	return renderDeploysTable(projectName, results)
}

func renderDeploysTable(projectName string, results []deployResult) error {
	for i, r := range results {
		if i > 0 {
			fmt.Println()
		}

		title := ui.ProjectTitleStyle.Render(fmt.Sprintf("%s / %s", projectName, r.Entry.Name))
		fmt.Println(title)

		if r.Err != nil {
			fmt.Printf("  %s %s\n", ui.ErrorStyle.Render(ui.IconError), ui.MutedStyle.Render(r.Err.Error()))
			continue
		}

		if len(r.Deployments) == 0 {
			fmt.Printf("  %s\n", ui.MutedStyle.Render("No deployments found."))
			continue
		}

		// Header
		fmt.Printf("  %-14s %-12s %-12s %-9s %s\n",
			ui.HeaderStyle.Render("Status"),
			ui.HeaderStyle.Render("Deployed"),
			ui.HeaderStyle.Render("Duration"),
			ui.HeaderStyle.Render("Commit"),
			ui.HeaderStyle.Render("Message"),
		)

		for _, d := range r.Deployments {
			status := ui.FormatStatus(d.Status)
			when := ui.TimeAgo(d.CreatedAt)
			dur := ui.Dash
			if d.Duration > 0 {
				dur = d.Duration.Truncate(1e9).String()
			}
			commit := ui.FormatCommit(d.Commit)
			msg := d.Message
			if len(msg) > 40 {
				msg = msg[:37] + "..."
			}
			if msg == "" {
				msg = ui.Dash
			}

			fmt.Printf("  %-14s %-12s %-12s %-9s %s\n",
				status, when, dur, commit, ui.MutedStyle.Render(msg))
		}
	}
	fmt.Println()
	return nil
}

type jsonDeployEntry struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Commit    string `json:"commit,omitempty"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	Duration  string `json:"duration,omitempty"`
	URL       string `json:"url,omitempty"`
}

type jsonDeployResult struct {
	Service     string            `json:"service"`
	Platform    string            `json:"platform"`
	Deployments []jsonDeployEntry `json:"deployments,omitempty"`
	Error       string            `json:"error,omitempty"`
}

func renderDeploysJSON(projectName string, results []deployResult) error {
	out := make([]jsonDeployResult, len(results))
	for i, r := range results {
		out[i] = jsonDeployResult{
			Service:  r.Entry.Name,
			Platform: r.Entry.Platform,
		}
		if r.Err != nil {
			out[i].Error = r.Err.Error()
			continue
		}
		for _, d := range r.Deployments {
			entry := jsonDeployEntry{
				ID:     d.ID,
				Status: d.Status,
				Commit: d.Commit,
				URL:    d.URL,
			}
			if d.Message != "" {
				entry.Message = d.Message
			}
			if !d.CreatedAt.IsZero() {
				entry.CreatedAt = d.CreatedAt.Format("2006-01-02T15:04:05Z")
			}
			if d.Duration > 0 {
				entry.Duration = d.Duration.Truncate(1e9).String()
			}
			out[i].Deployments = append(out[i].Deployments, entry)
		}
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
