package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const flyBaseURL = "https://api.machines.dev"

func init() {
	Register("flyio", func(token string) Platform {
		return NewFlyio(token)
	})
}

// Flyio implements the Platform interface for Fly.io Machines API.
type Flyio struct {
	token      string
	orgSlug    string
	httpClient *http.Client
}

// NewFlyio creates a new Fly.io platform instance.
func NewFlyio(token string) *Flyio {
	return &Flyio{
		token:      token,
		orgSlug:    "personal",
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (f *Flyio) SetOrgSlug(slug string) {
	f.orgSlug = slug
}

func (f *Flyio) Name() string {
	return "flyio"
}

func (f *Flyio) doRequest(method, path string, body []byte) (*http.Response, error) {
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, flyBaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Content-Type", "application/json")
	return f.httpClient.Do(req)
}

func (f *Flyio) Validate(token string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", flyBaseURL+"/v1/apps?org_slug=personal", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fly.io API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("invalid token: unauthorized")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("fly.io API returned status %d", resp.StatusCode)
	}
	return nil
}

// flyMachine represents a Fly.io machine.
type flyMachine struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Region     string `json:"region"`
	InstanceID string `json:"instance_id"`
	PrivateIP  string `json:"private_ip"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
	Config     struct {
		Image string `json:"image"`
	} `json:"config"`
	Events []flyMachineEvent `json:"events"`
}

type flyMachineEvent struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Timestamp int64  `json:"timestamp"`
}

func mapFlyState(state string) string {
	switch state {
	case "started":
		return "healthy"
	case "stopped", "suspended":
		return "sleeping"
	case "created":
		return "deploying"
	case "failed", "destroyed":
		return "failed"
	case "replaced":
		return "deploying"
	default:
		return state
	}
}

func (f *Flyio) listMachines(appName string) ([]flyMachine, error) {
	resp, err := f.doRequest("GET", fmt.Sprintf("/v1/apps/%s/machines", appName), nil)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fly.io API returned status %d", resp.StatusCode)
	}

	var machines []flyMachine
	if err := json.NewDecoder(resp.Body).Decode(&machines); err != nil {
		return nil, fmt.Errorf("decode machines: %w", err)
	}
	return machines, nil
}

func (f *Flyio) GetServiceStatus(serviceID string) (*ServiceStatus, error) {
	machines, err := f.listMachines(serviceID)
	if err != nil {
		return nil, err
	}

	status := &ServiceStatus{
		Status:    "healthy",
		Instances: len(machines),
	}

	if len(machines) == 0 {
		status.Status = "sleeping"
		return status, nil
	}

	// Aggregate machine states: any failed → failed, any deploying → deploying, all stopped → sleeping
	hasStarted := false
	hasFailed := false
	hasDeploying := false
	for _, m := range machines {
		s := mapFlyState(m.State)
		switch s {
		case "healthy":
			hasStarted = true
		case "failed":
			hasFailed = true
		case "deploying":
			hasDeploying = true
		}
	}

	if hasFailed {
		status.Status = "failed"
	} else if hasDeploying {
		status.Status = "deploying"
	} else if !hasStarted {
		status.Status = "sleeping"
	}

	// Use the most recently updated machine for deploy info
	var latest *flyMachine
	for i := range machines {
		if latest == nil || machines[i].UpdatedAt > latest.UpdatedAt {
			latest = &machines[i]
		}
	}
	if latest != nil {
		updatedAt, _ := time.Parse(time.RFC3339, latest.UpdatedAt)
		image := latest.Config.Image
		// Extract tag from image (e.g. registry.fly.io/app:deployment-xxx)
		commit := ""
		if idx := strings.LastIndex(image, ":"); idx >= 0 {
			commit = image[idx+1:]
		}

		status.LastDeploy = &Deployment{
			ID:        latest.InstanceID,
			Status:    mapFlyState(latest.State),
			Commit:    commit,
			Message:   fmt.Sprintf("machine %s (%s)", latest.ID, latest.Region),
			CreatedAt: updatedAt,
		}
	}

	return status, nil
}

func (f *Flyio) ListDeployments(serviceID string, limit int) ([]Deployment, error) {
	machines, err := f.listMachines(serviceID)
	if err != nil {
		return nil, err
	}

	// Collect unique instance_ids as "deployments"
	seen := make(map[string]bool)
	var deployments []Deployment

	for _, m := range machines {
		if seen[m.InstanceID] {
			continue
		}
		seen[m.InstanceID] = true

		updatedAt, _ := time.Parse(time.RFC3339, m.UpdatedAt)
		image := m.Config.Image
		commit := ""
		if idx := strings.LastIndex(image, ":"); idx >= 0 {
			commit = image[idx+1:]
		}

		deployments = append(deployments, Deployment{
			ID:        m.InstanceID,
			Status:    mapFlyState(m.State),
			Commit:    commit,
			Message:   fmt.Sprintf("machine %s (%s)", m.ID, m.Region),
			CreatedAt: updatedAt,
		})

		if len(deployments) >= limit {
			break
		}
	}

	return deployments, nil
}

func (f *Flyio) GetDeployment(deployID string) (*Deployment, error) {
	// deployID format: "appName/machineID"
	parts := strings.SplitN(deployID, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("fly.io deploy ID must be appName/machineID, got: %s", deployID)
	}
	appName, machineID := parts[0], parts[1]

	resp, err := f.doRequest("GET", fmt.Sprintf("/v1/apps/%s/machines/%s", appName, machineID), nil)
	if err != nil {
		return nil, fmt.Errorf("get machine: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("machine not found: %s", deployID)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fly.io API returned status %d", resp.StatusCode)
	}

	var m flyMachine
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode machine: %w", err)
	}

	updatedAt, _ := time.Parse(time.RFC3339, m.UpdatedAt)
	commit := ""
	if idx := strings.LastIndex(m.Config.Image, ":"); idx >= 0 {
		commit = m.Config.Image[idx+1:]
	}

	return &Deployment{
		ID:        m.InstanceID,
		Status:    mapFlyState(m.State),
		Commit:    commit,
		Message:   fmt.Sprintf("machine %s (%s)", m.ID, m.Region),
		CreatedAt: updatedAt,
	}, nil
}

func (f *Flyio) Redeploy(serviceID string) (*Deployment, error) {
	machines, err := f.listMachines(serviceID)
	if err != nil {
		return nil, err
	}

	if len(machines) == 0 {
		return nil, fmt.Errorf("no machines found for app %s", serviceID)
	}

	// Restart each running machine: stop then start
	for _, m := range machines {
		if m.State != "started" {
			continue
		}
		// Stop
		resp, err := f.doRequest("POST", fmt.Sprintf("/v1/apps/%s/machines/%s/stop", serviceID, m.ID), nil)
		if err != nil {
			return nil, fmt.Errorf("stop machine %s: %w", m.ID, err)
		}
		resp.Body.Close()

		// Start
		resp, err = f.doRequest("POST", fmt.Sprintf("/v1/apps/%s/machines/%s/start", serviceID, m.ID), nil)
		if err != nil {
			return nil, fmt.Errorf("start machine %s: %w", m.ID, err)
		}
		resp.Body.Close()
	}

	// Return info about the first machine
	m := machines[0]
	updatedAt, _ := time.Parse(time.RFC3339, m.UpdatedAt)
	return &Deployment{
		ID:        m.InstanceID,
		Status:    "deploying",
		Message:   fmt.Sprintf("restarted %d machine(s)", len(machines)),
		CreatedAt: updatedAt,
	}, nil
}

func (f *Flyio) GetLogs(serviceID string, opts LogOptions) ([]LogEntry, error) {
	// Fly.io logs use a different path prefix: /api/v1/
	path := fmt.Sprintf("/api/v1/apps/%s/logs", serviceID)

	req, err := http.NewRequest("GET", flyBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+f.token)

	// Use a longer timeout for logs (NDJSON stream)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fly.io logs API returned status %d", resp.StatusCode)
	}

	// Response is NDJSON (newline-delimited JSON)
	var entries []LogEntry
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var logLine struct {
			Timestamp string `json:"timestamp"`
			Message   string `json:"message"`
			Level     string `json:"level"`
			Instance  string `json:"instance"`
			Region    string `json:"region"`
		}
		if err := decoder.Decode(&logLine); err != nil {
			break // End of stream or parse error
		}

		ts, _ := time.Parse(time.RFC3339Nano, logLine.Timestamp)

		level := strings.ToLower(logLine.Level)
		if level == "" {
			level = "info"
		}
		if opts.Level != "" && level != opts.Level {
			continue
		}

		if opts.Since > 0 {
			cutoff := time.Now().Add(-opts.Since)
			if ts.Before(cutoff) {
				continue
			}
		}

		entries = append(entries, LogEntry{
			Timestamp: ts,
			Level:     level,
			Message:   logLine.Message,
			Source:    logLine.Region,
		})
	}

	if opts.Tail > 0 && len(entries) > opts.Tail {
		entries = entries[len(entries)-opts.Tail:]
	}

	return entries, nil
}

func (f *Flyio) Scale(serviceID string, opts ScaleOptions) error {
	return fmt.Errorf("not supported: use 'fly scale' CLI or create/destroy machines via Fly.io dashboard")
}

func (f *Flyio) DiscoverServices() ([]DiscoveredService, error) {
	resp, err := f.doRequest("GET", fmt.Sprintf("/v1/apps?org_slug=%s", f.orgSlug), nil)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fly.io API returned status %d", resp.StatusCode)
	}

	var result struct {
		Apps []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"apps"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode apps: %w", err)
	}

	var services []DiscoveredService
	for _, app := range result.Apps {
		services = append(services, DiscoveredService{
			ID:       app.Name,
			Name:     app.Name,
			Platform: "flyio",
		})
	}
	return services, nil
}

func (f *Flyio) WatchDeployment(serviceID string, currentDeployID string) (<-chan DeployEvent, error) {
	ch := make(chan DeployEvent)

	go func() {
		defer close(ch)

		const pollInterval = 3 * time.Second

		// Get current machine states
		machines, err := f.listMachines(serviceID)
		if err != nil {
			ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("list machines: %w", err)}
			return
		}

		// Check if any machine is currently updating
		for _, m := range machines {
			if m.InstanceID != currentDeployID && isInProgress(mapFlyState(m.State)) {
				dep := machineToDeployment(m)
				ch <- DeployEvent{
					Phase:   "detected",
					Message: fmt.Sprintf("In-progress update found (machine %s)", m.ID),
					Deploy:  &dep,
				}
				f.trackMachines(ch, serviceID)
				return
			}
		}

		// Wait for instance_id change (new deployment)
		for {
			machines, err := f.listMachines(serviceID)
			if err != nil {
				ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("list machines: %w", err)}
				return
			}

			for _, m := range machines {
				if m.InstanceID != currentDeployID && m.InstanceID != "" {
					dep := machineToDeployment(m)
					ch <- DeployEvent{
						Phase:   "detected",
						Message: fmt.Sprintf("New deployment detected (machine %s)", m.ID),
						Deploy:  &dep,
					}
					f.trackMachines(ch, serviceID)
					return
				}
			}

			ch <- DeployEvent{Phase: "waiting", Message: "Waiting for new deployment..."}
			time.Sleep(pollInterval)
		}
	}()

	return ch, nil
}

func (f *Flyio) trackMachines(ch chan<- DeployEvent, appName string) {
	const pollInterval = 3 * time.Second
	lastPhase := ""

	for {
		machines, err := f.listMachines(appName)
		if err != nil {
			ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("list machines: %w", err)}
			return
		}

		// Aggregate: all started → done, any failed → failed, else deploying
		allStarted := true
		anyFailed := false
		for _, m := range machines {
			if m.State == "failed" || m.State == "destroyed" {
				anyFailed = true
			}
			if m.State != "started" {
				allStarted = false
			}
		}

		var phase string
		if anyFailed {
			phase = "failed"
		} else if allStarted && len(machines) > 0 {
			phase = "done"
		} else {
			phase = "deploying"
		}

		if phase != lastPhase {
			lastPhase = phase
			event := DeployEvent{Phase: phase}
			if len(machines) > 0 {
				dep := machineToDeployment(machines[0])
				event.Deploy = &dep
			}
			switch phase {
			case "deploying":
				event.Message = "Deploying..."
			case "done":
				event.Message = "All machines started!"
				ch <- event
				return
			case "failed":
				event.Message = "Machine failed!"
				event.Error = fmt.Errorf("one or more machines failed")
				ch <- event
				return
			}
			ch <- event
		}

		time.Sleep(pollInterval)
	}
}

func machineToDeployment(m flyMachine) Deployment {
	updatedAt, _ := time.Parse(time.RFC3339, m.UpdatedAt)
	commit := ""
	if idx := strings.LastIndex(m.Config.Image, ":"); idx >= 0 {
		commit = m.Config.Image[idx+1:]
	}
	return Deployment{
		ID:        m.InstanceID,
		Status:    mapFlyState(m.State),
		Commit:    commit,
		Message:   fmt.Sprintf("machine %s (%s)", m.ID, m.Region),
		CreatedAt: updatedAt,
	}
}
