package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const vercelBaseURL = "https://api.vercel.com"

func init() {
	Register("vercel", func(token string) Platform {
		return NewVercel(token)
	})
}

// Vercel implements the Platform interface using net/http.
type Vercel struct {
	token      string
	httpClient *http.Client
}

// NewVercel creates a new Vercel platform instance.
func NewVercel(token string) *Vercel {
	return &Vercel{
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (v *Vercel) Name() string {
	return "vercel"
}

func (v *Vercel) doRequest(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, vercelBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+v.token)
	req.Header.Set("Content-Type", "application/json")
	return v.httpClient.Do(req)
}

// Validate checks whether the token is valid by calling GET /v2/user.
func (v *Vercel) Validate(token string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", vercelBaseURL+"/v2/user", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("vercel API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("invalid token: unauthorized")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("vercel API returned status %d", resp.StatusCode)
	}
	return nil
}

func (v *Vercel) GetServiceStatus(serviceID string) (*ServiceStatus, error) {
	resp, err := v.doRequest("GET", fmt.Sprintf("/v6/deployments?projectId=%s&limit=1&state=READY", serviceID))
	if err != nil {
		return nil, fmt.Errorf("get deployments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vercel API returned status %d", resp.StatusCode)
	}

	var result struct {
		Deployments []struct {
			UID     string `json:"uid"`
			State   string `json:"state"`
			Created int64  `json:"created"`
			URL     string `json:"url"`
			Meta    struct {
				GitCommitSha     string `json:"githubCommitSha"`
				GitCommitMessage string `json:"githubCommitMessage"`
			} `json:"meta"`
		} `json:"deployments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	status := &ServiceStatus{
		Status: "healthy",
	}
	if len(result.Deployments) > 0 {
		d := result.Deployments[0]
		status.Status = mapVercelState(d.State)
		status.LastDeploy = &Deployment{
			ID:        d.UID,
			Status:    mapVercelState(d.State),
			Commit:    d.Meta.GitCommitSha,
			Message:   d.Meta.GitCommitMessage,
			CreatedAt: time.UnixMilli(d.Created),
			URL:       "https://" + d.URL,
		}
	}
	return status, nil
}

func mapVercelState(state string) string {
	switch state {
	case "READY":
		return "healthy"
	case "BUILDING":
		return "building"
	case "DEPLOYING", "INITIALIZING", "QUEUED":
		return "deploying"
	case "ERROR", "CANCELED":
		return "failed"
	default:
		return state
	}
}

func (v *Vercel) ListDeployments(serviceID string, limit int) ([]Deployment, error) {
	resp, err := v.doRequest("GET", fmt.Sprintf("/v6/deployments?projectId=%s&limit=%d", serviceID, limit))
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vercel API returned status %d", resp.StatusCode)
	}

	var result struct {
		Deployments []struct {
			UID     string `json:"uid"`
			State   string `json:"state"`
			Created int64  `json:"created"`
			URL     string `json:"url"`
			Meta    struct {
				GitCommitSha     string `json:"githubCommitSha"`
				GitCommitMessage string `json:"githubCommitMessage"`
			} `json:"meta"`
		} `json:"deployments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var deployments []Deployment
	for _, d := range result.Deployments {
		deployments = append(deployments, Deployment{
			ID:        d.UID,
			Status:    mapVercelState(d.State),
			Commit:    d.Meta.GitCommitSha,
			Message:   d.Meta.GitCommitMessage,
			CreatedAt: time.UnixMilli(d.Created),
			URL:       "https://" + d.URL,
		})
	}
	return deployments, nil
}

func (v *Vercel) GetDeployment(deployID string) (*Deployment, error) {
	resp, err := v.doRequest("GET", "/v6/deployments/"+deployID)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("deployment not found: %s", deployID)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vercel API returned status %d", resp.StatusCode)
	}

	var d struct {
		UID        string `json:"uid"`
		State      string `json:"state"`
		ReadyState string `json:"readyState"`
		Created    int64  `json:"created"`
		URL        string `json:"url"`
		Meta       struct {
			GitCommitSha     string `json:"githubCommitSha"`
			GitCommitMessage string `json:"githubCommitMessage"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// The individual deployment endpoint returns readyState instead of state
	state := d.State
	if state == "" {
		state = d.ReadyState
	}

	dep := &Deployment{
		ID:        d.UID,
		Status:    mapVercelState(state),
		Commit:    d.Meta.GitCommitSha,
		Message:   d.Meta.GitCommitMessage,
		CreatedAt: time.UnixMilli(d.Created),
	}
	if d.URL != "" {
		dep.URL = "https://" + d.URL
	}
	return dep, nil
}

func (v *Vercel) Redeploy(serviceID string) (*Deployment, error) {
	return nil, fmt.Errorf("not supported: push to git to trigger a new Vercel deployment")
}

func (v *Vercel) GetLogs(serviceID string, opts LogOptions) ([]LogEntry, error) {
	// Get the latest deployment for this project
	resp, err := v.doRequest("GET", fmt.Sprintf("/v6/deployments?projectId=%s&limit=1", serviceID))
	if err != nil {
		return nil, fmt.Errorf("get deployments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vercel API returned status %d", resp.StatusCode)
	}

	var deploys struct {
		Deployments []struct {
			UID string `json:"uid"`
		} `json:"deployments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&deploys); err != nil {
		return nil, fmt.Errorf("decode deployments: %w", err)
	}
	if len(deploys.Deployments) == 0 {
		return nil, nil
	}

	deployID := deploys.Deployments[0].UID

	// Fetch build events for this deployment
	eventsResp, err := v.doRequest("GET", fmt.Sprintf("/v2/deployments/%s/events", deployID))
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}
	defer eventsResp.Body.Close()

	if eventsResp.StatusCode != 200 {
		return nil, fmt.Errorf("vercel events API returned status %d", eventsResp.StatusCode)
	}

	var events []struct {
		Type    string `json:"type"`
		Created int64  `json:"created"`
		Text    string `json:"text"`
	}
	if err := json.NewDecoder(eventsResp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("decode events: %w", err)
	}

	var entries []LogEntry
	for _, e := range events {
		if e.Text == "" {
			continue
		}
		level := "info"
		if e.Type == "stderr" || e.Type == "error" {
			level = "error"
		}
		if opts.Level != "" && level != opts.Level {
			continue
		}
		entries = append(entries, LogEntry{
			Timestamp: time.UnixMilli(e.Created),
			Level:     level,
			Message:   e.Text,
			Source:    "build",
		})
	}

	if opts.Since > 0 {
		cutoff := time.Now().Add(-opts.Since)
		var filtered []LogEntry
		for _, e := range entries {
			if e.Timestamp.After(cutoff) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	if opts.Tail > 0 && len(entries) > opts.Tail {
		entries = entries[len(entries)-opts.Tail:]
	}

	return entries, nil
}

func (v *Vercel) Scale(serviceID string, opts ScaleOptions) error {
	return fmt.Errorf("not supported: Vercel uses automatic scaling that cannot be controlled via API")
}

func (v *Vercel) DiscoverServices() ([]DiscoveredService, error) {
	resp, err := v.doRequest("GET", "/v9/projects?limit=100")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vercel API returned status %d", resp.StatusCode)
	}

	var result struct {
		Projects []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var services []DiscoveredService
	for _, p := range result.Projects {
		services = append(services, DiscoveredService{
			ID:       p.ID,
			Name:     p.Name,
			Platform: "vercel",
		})
	}
	return services, nil
}

func (v *Vercel) WatchDeployment(serviceID string, currentDeployID string) (<-chan DeployEvent, error) {
	ch := make(chan DeployEvent)

	go func() {
		defer close(ch)

		const pollInterval = 3 * time.Second

		// Check if the latest deployment is already in-progress.
		// This handles the race where git push triggers a deployment before watch starts,
		// so currentDeployID already points to the new (building) deployment.
		deploys, err := v.ListDeployments(serviceID, 1)
		if err != nil {
			ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("poll deployments: %w", err)}
			return
		}
		if len(deploys) > 0 && isInProgress(deploys[0].Status) {
			d := deploys[0]
			ch <- DeployEvent{
				Phase:   "detected",
				Message: fmt.Sprintf("In-progress deployment found (%s)", d.ID),
				Deploy:  &d,
			}
			v.trackDeployment(ch, d.ID)
			return
		}

		// Phase 1: Detect a new deployment
		for {
			deploys, err := v.ListDeployments(serviceID, 1)
			if err != nil {
				ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("poll deployments: %w", err)}
				return
			}

			if len(deploys) > 0 {
				d := deploys[0]
				if d.ID != currentDeployID {
					ch <- DeployEvent{
						Phase:   "detected",
						Message: fmt.Sprintf("New deployment detected! (%s)", d.ID),
						Deploy:  &d,
					}
					v.trackDeployment(ch, d.ID)
					return
				}
			}

			ch <- DeployEvent{Phase: "waiting", Message: "Waiting for new deployment..."}
			time.Sleep(pollInterval)
		}
	}()

	return ch, nil
}

func (v *Vercel) trackDeployment(ch chan<- DeployEvent, deployID string) {
	const pollInterval = 3 * time.Second
	lastPhase := ""

	for {
		deploy, err := v.GetDeployment(deployID)
		if err != nil {
			ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("get deployment: %w", err)}
			return
		}

		phase := mapVercelToWatchPhase(deploy.Status)
		if phase != lastPhase {
			lastPhase = phase

			event := DeployEvent{Phase: phase, Deploy: deploy}
			switch phase {
			case "building":
				event.Message = "Building..."
			case "deploying":
				event.Message = "Deploying..."
			case "healthcheck":
				event.Message = "Health check..."
			case "done":
				event.Message = "Deploy successful!"
				ch <- event
				return
			case "failed":
				event.Message = "Deployment failed!"
				event.Error = fmt.Errorf("deployment %s failed", deployID)
				// Try to get error logs
				if logs, err := v.getDeploymentErrors(deployID); err == nil {
					event.Logs = logs
				}
				ch <- event
				return
			}
			ch <- event
		}

		time.Sleep(pollInterval)
	}
}

func mapVercelToWatchPhase(status string) string {
	switch status {
	case "building":
		return "building"
	case "deploying":
		return "deploying"
	case "healthy":
		return "done"
	case "failed":
		return "failed"
	default:
		return "building"
	}
}

func (v *Vercel) getDeploymentErrors(deployID string) ([]string, error) {
	resp, err := v.doRequest("GET", fmt.Sprintf("/v2/deployments/%s/events", deployID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var events []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}

	var errLogs []string
	for _, e := range events {
		if (e.Type == "stderr" || e.Type == "error") && e.Text != "" {
			errLogs = append(errLogs, e.Text)
		}
	}
	return errLogs, nil
}
