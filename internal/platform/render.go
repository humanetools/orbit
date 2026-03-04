package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const renderBaseURL = "https://api.render.com/v1"

func init() {
	Register("render", func(token string) Platform {
		return NewRender(token)
	})
}

// Render implements the Platform interface using net/http.
type Render struct {
	token      string
	httpClient *http.Client
}

// NewRender creates a new Render platform instance.
func NewRender(token string) *Render {
	return &Render{
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *Render) Name() string {
	return "render"
}

func (r *Render) doRequest(method, path string, body []byte) (*http.Response, error) {
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, renderBaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return r.httpClient.Do(req)
}

// Validate checks whether the token is valid by calling GET /owners.
func (r *Render) Validate(token string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", renderBaseURL+"/owners", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("render API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("invalid token: unauthorized")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("render API returned status %d", resp.StatusCode)
	}
	return nil
}

func mapRenderStatus(status string) string {
	switch status {
	case "live":
		return "healthy"
	case "created", "build_in_progress", "update_in_progress":
		return "building"
	case "pre_deploy_in_progress":
		return "deploying"
	case "deactivated":
		return "sleeping"
	case "build_failed", "update_failed", "pre_deploy_failed", "canceled":
		return "failed"
	default:
		return status
	}
}

// renderDeploy is the JSON shape for a Render deploy object.
type renderDeploy struct {
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Trigger    string    `json:"trigger"`
	Commit     struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"commit"`
}

func (d *renderDeploy) toDeployment() Deployment {
	dep := Deployment{
		ID:        d.ID,
		Status:    mapRenderStatus(d.Status),
		CreatedAt: d.CreatedAt,
	}
	if !d.FinishedAt.IsZero() && d.FinishedAt.After(d.CreatedAt) {
		dep.Duration = d.FinishedAt.Sub(d.CreatedAt)
	}
	dep.Commit = d.Commit.ID
	dep.Message = d.Commit.Message
	return dep
}

func (r *Render) GetServiceStatus(serviceID string) (*ServiceStatus, error) {
	// Get service info
	resp, err := r.doRequest("GET", "/services/"+serviceID, nil)
	if err != nil {
		return nil, fmt.Errorf("get service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("render API returned status %d", resp.StatusCode)
	}

	var svc struct {
		Suspended string `json:"suspended"` // not_suspended / suspended
	}
	if err := json.NewDecoder(resp.Body).Decode(&svc); err != nil {
		return nil, fmt.Errorf("decode service: %w", err)
	}

	status := &ServiceStatus{
		Status: "healthy",
	}

	if svc.Suspended == "suspended" {
		status.Status = "sleeping"
	}

	// Get latest deploy
	deploys, err := r.ListDeployments(serviceID, 1)
	if err == nil && len(deploys) > 0 {
		d := deploys[0]
		status.LastDeploy = &d
		if svc.Suspended != "suspended" {
			status.Status = d.Status
		}
	}

	return status, nil
}

func (r *Render) ListDeployments(serviceID string, limit int) ([]Deployment, error) {
	resp, err := r.doRequest("GET", fmt.Sprintf("/services/%s/deploys?limit=%d", serviceID, limit), nil)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("render API returned status %d", resp.StatusCode)
	}

	// Render wraps each deploy in a cursor object: [{"deploy": {...}}, ...]
	var items []struct {
		Deploy renderDeploy `json:"deploy"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var deployments []Deployment
	for _, item := range items {
		deployments = append(deployments, item.Deploy.toDeployment())
	}
	return deployments, nil
}

// GetDeployment retrieves a single deployment.
// deployID should be "serviceID/deployID" since Render requires both.
func (r *Render) GetDeployment(deployID string) (*Deployment, error) {
	parts := strings.SplitN(deployID, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("render deploy ID must be serviceID/deployID, got: %s", deployID)
	}
	svcID, dID := parts[0], parts[1]

	resp, err := r.doRequest("GET", fmt.Sprintf("/services/%s/deploys/%s", svcID, dID), nil)
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("deployment not found: %s", deployID)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("render API returned status %d", resp.StatusCode)
	}

	var d renderDeploy
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	dep := d.toDeployment()
	return &dep, nil
}

func (r *Render) Redeploy(serviceID string) (*Deployment, error) {
	resp, err := r.doRequest("POST", fmt.Sprintf("/services/%s/deploys", serviceID), []byte("{}"))
	if err != nil {
		return nil, fmt.Errorf("trigger deploy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		return nil, fmt.Errorf("render API returned status %d", resp.StatusCode)
	}

	var d renderDeploy
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	dep := d.toDeployment()
	return &dep, nil
}

func (r *Render) GetLogs(serviceID string, opts LogOptions) ([]LogEntry, error) {
	path := fmt.Sprintf("/services/%s/logs", serviceID)

	resp, err := r.doRequest("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("get logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("render API returned status %d", resp.StatusCode)
	}

	var rawLogs []struct {
		ID        string    `json:"id"`
		Timestamp time.Time `json:"timestamp"`
		Level     string    `json:"level"`
		Message   string    `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rawLogs); err != nil {
		return nil, fmt.Errorf("decode logs: %w", err)
	}

	var entries []LogEntry
	for _, l := range rawLogs {
		level := strings.ToLower(l.Level)
		if level == "" {
			level = "info"
		}
		if opts.Level != "" && level != opts.Level {
			continue
		}
		entries = append(entries, LogEntry{
			Timestamp: l.Timestamp,
			Level:     level,
			Message:   l.Message,
			Source:    "runtime",
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

func (r *Render) Scale(serviceID string, opts ScaleOptions) error {
	body, err := json.Marshal(map[string]int{"numInstances": opts.MinInstances})
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	resp, err := r.doRequest("POST", fmt.Sprintf("/services/%s/scale", serviceID), body)
	if err != nil {
		return fmt.Errorf("scale service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 202 {
		return fmt.Errorf("render API returned status %d", resp.StatusCode)
	}
	return nil
}

func (r *Render) DiscoverServices() ([]DiscoveredService, error) {
	resp, err := r.doRequest("GET", "/services?limit=100", nil)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("render API returned status %d", resp.StatusCode)
	}

	// Render wraps each service: [{"service": {...}}, ...]
	var items []struct {
		Service struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"service"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var services []DiscoveredService
	for _, item := range items {
		services = append(services, DiscoveredService{
			ID:       item.Service.ID,
			Name:     item.Service.Name,
			Platform: "render",
		})
	}
	return services, nil
}

func (r *Render) WatchDeployment(serviceID string, currentDeployID string) (<-chan DeployEvent, error) {
	ch := make(chan DeployEvent)

	go func() {
		defer close(ch)

		const pollInterval = 3 * time.Second

		// Check if the latest deployment is already in-progress.
		deploys, err := r.ListDeployments(serviceID, 1)
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
			r.trackDeployment(ch, serviceID, d.ID)
			return
		}

		// Phase 1: Detect a new deployment
		for {
			deploys, err := r.ListDeployments(serviceID, 1)
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
					r.trackDeployment(ch, serviceID, d.ID)
					return
				}
			}

			ch <- DeployEvent{Phase: "waiting", Message: "Waiting for new deployment..."}
			time.Sleep(pollInterval)
		}
	}()

	return ch, nil
}

func (r *Render) trackDeployment(ch chan<- DeployEvent, serviceID, deployID string) {
	const pollInterval = 3 * time.Second
	lastPhase := ""
	compositeID := serviceID + "/" + deployID

	for {
		deploy, err := r.GetDeployment(compositeID)
		if err != nil {
			ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("get deployment: %w", err)}
			return
		}

		phase := mapRenderToWatchPhase(deploy.Status)
		if phase != lastPhase {
			lastPhase = phase

			event := DeployEvent{Phase: phase, Deploy: deploy}
			switch phase {
			case "building":
				event.Message = "Building..."
			case "deploying":
				event.Message = "Deploying..."
			case "done":
				event.Message = "Deploy successful!"
				ch <- event
				return
			case "failed":
				event.Message = "Deployment failed!"
				event.Error = fmt.Errorf("deployment %s failed", deployID)
				ch <- event
				return
			}
			ch <- event
		}

		time.Sleep(pollInterval)
	}
}

func mapRenderToWatchPhase(status string) string {
	switch status {
	case "building":
		return "building"
	case "deploying":
		return "deploying"
	case "healthy":
		return "done"
	case "failed", "sleeping":
		return "failed"
	default:
		return "building"
	}
}
