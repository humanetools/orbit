package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	statusService string
	statusFormat  string
)

var statusCmd = &cobra.Command{
	Use:   "status [project]",
	Short: "Show service status across platforms",
	Long: `Show service status for your projects.

  orbit status                         Overview of all projects (L0)
  orbit status <project>               Detailed metrics for a project (L1)
  orbit status <project> --service X   Single service detail card (L2)

Flags:
  --format json    Output as JSON
  --service NAME   Show detail for a specific service`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusService, "service", "", "Show detail for a specific service")
	statusCmd.Flags().StringVar(&statusFormat, "format", "", "Output format (json)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	key, err := config.LoadOrCreateKey()
	if err != nil {
		return fmt.Errorf("load encryption key: %w", err)
	}

	switch {
	case len(args) == 0:
		return runStatusAllProjects(cfg, key)
	case statusService != "":
		return runStatusService(cfg, key, args[0], statusService)
	default:
		return runStatusProject(cfg, key, args[0])
	}
}

// --- L0: All Projects Overview ---

func runStatusAllProjects(cfg *config.Config, key []byte) error {
	if len(cfg.Projects) == 0 {
		fmt.Println("No projects configured.")
		fmt.Println("Add projects to ~/.orbit/config.yaml to get started.")
		return nil
	}

	// Sort project names for consistent output
	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	if statusFormat == "json" {
		return renderAllProjectsJSON(cfg, key, names)
	}

	for i, name := range names {
		proj := cfg.Projects[name]
		results := fetchStatuses(proj.Topology, cfg, key)
		fmt.Print(ui.RenderOverviewTable(name, results))
		if i < len(names)-1 {
			fmt.Println()
		}
	}
	fmt.Println()

	return nil
}

// --- L1: Single Project Detail ---

func runStatusProject(cfg *config.Config, key []byte, name string) error {
	proj, ok := cfg.Projects[name]
	if !ok {
		return fmt.Errorf("project %q not found\nAvailable projects: %s", name, projectNames(cfg))
	}

	results := fetchStatuses(proj.Topology, cfg, key)

	if statusFormat == "json" {
		return renderProjectJSON(name, results)
	}

	output, violations := ui.RenderDetailTable(name, results, cfg.Thresholds)
	fmt.Println(output)
	if warn := ui.RenderViolations(violations); warn != "" {
		fmt.Println(warn)
	}
	return nil
}

// --- L2: Single Service Detail ---

func runStatusService(cfg *config.Config, key []byte, projectName, serviceName string) error {
	proj, ok := cfg.Projects[projectName]
	if !ok {
		return fmt.Errorf("project %q not found\nAvailable projects: %s", projectName, projectNames(cfg))
	}

	// Find the service entry
	var entry *config.ServiceEntry
	for i := range proj.Topology {
		if proj.Topology[i].Name == serviceName {
			entry = &proj.Topology[i]
			break
		}
	}
	if entry == nil {
		var svcNames []string
		for _, e := range proj.Topology {
			svcNames = append(svcNames, e.Name)
		}
		return fmt.Errorf("service %q not found in project %q\nAvailable services: %s",
			serviceName, projectName, joinNames(svcNames))
	}

	status, err := fetchSingleStatus(*entry, cfg, key)
	if err != nil {
		return fmt.Errorf("fetch status for %s: %w", serviceName, err)
	}

	if statusFormat == "json" {
		return renderServiceJSON(*entry, status)
	}

	output, violations := ui.RenderServiceDetail(projectName, *entry, status, cfg.Thresholds)
	fmt.Println(output)
	if warn := ui.RenderViolations(violations); warn != "" {
		fmt.Println(warn)
	}
	return nil
}

// --- Parallel Fetch ---

func fetchStatuses(entries []config.ServiceEntry, cfg *config.Config, key []byte) []ui.ServiceResult {
	results := make([]ui.ServiceResult, len(entries))
	var wg sync.WaitGroup

	for i, entry := range entries {
		results[i].Entry = entry
		wg.Add(1)
		go func(idx int, e config.ServiceEntry) {
			defer wg.Done()
			status, err := fetchSingleStatus(e, cfg, key)
			results[idx].Status = status
			results[idx].Err = err
		}(i, entry)
	}

	wg.Wait()
	return results
}

func fetchSingleStatus(entry config.ServiceEntry, cfg *config.Config, key []byte) (*platform.ServiceStatus, error) {
	pc, ok := cfg.Platforms[entry.Platform]
	if !ok {
		return nil, fmt.Errorf("platform %q not connected", entry.Platform)
	}

	token, err := config.Decrypt(key, pc.Token)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}

	p, err := platform.Get(entry.Platform, token)
	if err != nil {
		return nil, err
	}

	return p.GetServiceStatus(entry.ID)
}

// --- JSON Output ---

type jsonServiceStatus struct {
	Name     string  `json:"name"`
	Platform string  `json:"platform"`
	ID       string  `json:"id"`
	Status   string  `json:"status,omitempty"`
	Response int     `json:"response_ms,omitempty"`
	CPU      float64 `json:"cpu,omitempty"`
	Memory   float64 `json:"memory,omitempty"`
	Instance int     `json:"instances,omitempty"`
	MaxInst  int     `json:"max_instances,omitempty"`
	Deploy   *jsonDeploy `json:"last_deploy,omitempty"`
	Error    string  `json:"error,omitempty"`
}

type jsonDeploy struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Commit  string `json:"commit,omitempty"`
	Message string `json:"message,omitempty"`
	Created string `json:"created_at,omitempty"`
	URL     string `json:"url,omitempty"`
}

func toJSONService(r ui.ServiceResult) jsonServiceStatus {
	js := jsonServiceStatus{
		Name:     r.Entry.Name,
		Platform: r.Entry.Platform,
		ID:       r.Entry.ID,
	}
	if r.Err != nil {
		js.Error = r.Err.Error()
		return js
	}
	js.Status = r.Status.Status
	js.Response = r.Status.ResponseMs
	js.CPU = r.Status.CPU
	js.Memory = r.Status.Memory
	js.Instance = r.Status.Instances
	js.MaxInst = r.Status.MaxInstances
	if r.Status.LastDeploy != nil {
		d := r.Status.LastDeploy
		js.Deploy = &jsonDeploy{
			ID:      d.ID,
			Status:  d.Status,
			Commit:  d.Commit,
			Message: d.Message,
			URL:     d.URL,
		}
		if !d.CreatedAt.IsZero() {
			js.Deploy.Created = d.CreatedAt.Format("2006-01-02T15:04:05Z")
		}
	}
	return js
}

func renderAllProjectsJSON(cfg *config.Config, key []byte, names []string) error {
	out := make(map[string][]jsonServiceStatus)
	for _, name := range names {
		proj := cfg.Projects[name]
		results := fetchStatuses(proj.Topology, cfg, key)
		services := make([]jsonServiceStatus, len(results))
		for i, r := range results {
			services[i] = toJSONService(r)
		}
		out[name] = services
	}
	return printJSON(out)
}

func renderProjectJSON(name string, results []ui.ServiceResult) error {
	services := make([]jsonServiceStatus, len(results))
	for i, r := range results {
		services[i] = toJSONService(r)
	}
	out := map[string][]jsonServiceStatus{name: services}
	return printJSON(out)
}

func renderServiceJSON(entry config.ServiceEntry, status *platform.ServiceStatus) error {
	r := ui.ServiceResult{Entry: entry, Status: status}
	return printJSON(toJSONService(r))
}

func printJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// --- Helpers ---

func projectNames(cfg *config.Config) string {
	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)
	return joinNames(names)
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	result := names[0]
	for _, n := range names[1:] {
		result += ", " + n
	}
	return result
}
