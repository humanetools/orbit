package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	koyeb "github.com/koyeb/koyeb-api-client-go/api/v1/koyeb"
)

const koyebBaseURL = "https://app.koyeb.com"

func init() {
	Register("koyeb", func(token string) Platform {
		return NewKoyeb(token)
	})
}

// Koyeb implements the Platform interface using the official SDK.
type Koyeb struct {
	token  string
	client *koyeb.APIClient
	ctx    context.Context
}

// NewKoyeb creates a new Koyeb platform instance.
func NewKoyeb(token string) *Koyeb {
	cfg := koyeb.NewConfiguration()
	cfg.AddDefaultHeader("Authorization", "Bearer "+token)

	return &Koyeb{
		token:  token,
		client: koyeb.NewAPIClient(cfg),
		ctx:    context.Background(),
	}
}

func (k *Koyeb) Name() string {
	return "koyeb"
}

// Validate checks whether the token is valid by listing services.
func (k *Koyeb) Validate(token string) error {
	cfg := koyeb.NewConfiguration()
	cfg.AddDefaultHeader("Authorization", "Bearer "+token)
	client := koyeb.NewAPIClient(cfg)

	_, resp, err := client.ServicesApi.ListServices(k.ctx).Limit("1").Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == 401 {
			return fmt.Errorf("invalid token: unauthorized")
		}
		return fmt.Errorf("koyeb API error: %w", err)
	}
	return nil
}

// mapKoyebStatus converts a Koyeb service status to an Orbit status string.
func mapKoyebStatus(status string) string {
	switch status {
	case "HEALTHY":
		return "healthy"
	case "DEGRADED":
		return "degraded"
	case "UNHEALTHY", "ERROR":
		return "unhealthy"
	case "SLEEPING", "PAUSED", "PAUSING":
		return "sleeping"
	case "STARTING", "PROVISIONING":
		return "building"
	default:
		return status
	}
}

// mapKoyebDeployStatus converts a Koyeb deployment status to an Orbit status.
func mapKoyebDeployStatus(status string) string {
	switch status {
	case "PENDING", "QUEUED":
		return "pending"
	case "PROVISIONING", "SCHEDULED":
		return "building"
	case "DEPLOYING", "STARTING":
		return "deploying"
	case "HEALTHY":
		return "healthy"
	case "DEGRADED":
		return "degraded"
	case "UNHEALTHY", "ERROR", "ERRORING":
		return "failed"
	case "STOPPED", "SLEEPING":
		return "sleeping"
	default:
		return status
	}
}

func (k *Koyeb) GetServiceStatus(serviceID string) (*ServiceStatus, error) {
	svc, resp, err := k.client.ServicesApi.GetService(k.ctx, serviceID).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, fmt.Errorf("service not found: %s", serviceID)
		}
		return nil, fmt.Errorf("get service: %w", err)
	}

	service := svc.GetService()
	status := &ServiceStatus{
		Status: mapKoyebStatus(string(service.GetStatus())),
	}

	// Get latest deployment for additional context
	deploys, _, err := k.client.DeploymentsApi.ListDeployments(k.ctx).
		ServiceId(serviceID).Limit("1").Execute()
	if err == nil {
		deployList := deploys.GetDeployments()
		if len(deployList) > 0 {
			d := deployList[0]
			status.LastDeploy = &Deployment{
				ID:        d.GetId(),
				Status:    mapKoyebDeployStatus(string(d.GetStatus())),
				CreatedAt: d.GetCreatedAt(),
			}
			def := d.GetDefinition()
			if def.HasGit() {
				git := def.GetGit()
				status.LastDeploy.Commit = git.GetSha()
			}
		}
	}

	return status, nil
}

func (k *Koyeb) ListDeployments(serviceID string, limit int) ([]Deployment, error) {
	reply, _, err := k.client.DeploymentsApi.ListDeployments(k.ctx).
		ServiceId(serviceID).Limit(strconv.Itoa(limit)).Execute()
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	var deployments []Deployment
	for _, d := range reply.GetDeployments() {
		dep := Deployment{
			ID:        d.GetId(),
			Status:    mapKoyebDeployStatus(string(d.GetStatus())),
			CreatedAt: d.GetCreatedAt(),
		}
		def := d.GetDefinition()
		if def.HasGit() {
			git := def.GetGit()
			dep.Commit = git.GetSha()
			dep.Message = git.GetRepository()
		}
		deployments = append(deployments, dep)
	}
	return deployments, nil
}

func (k *Koyeb) GetDeployment(deployID string) (*Deployment, error) {
	reply, _, err := k.client.DeploymentsApi.GetDeployment(k.ctx, deployID).Execute()
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}

	d := reply.GetDeployment()
	dep := &Deployment{
		ID:        d.GetId(),
		Status:    mapKoyebDeployStatus(string(d.GetStatus())),
		CreatedAt: d.GetCreatedAt(),
	}
	return dep, nil
}

func (k *Koyeb) Redeploy(serviceID string) (*Deployment, error) {
	reply, _, err := k.client.ServicesApi.ReDeploy(k.ctx, serviceID).
		Info(*koyeb.NewRedeployRequestInfo()).Execute()
	if err != nil {
		return nil, fmt.Errorf("redeploy: %w", err)
	}

	d := reply.GetDeployment()
	return &Deployment{
		ID:        d.GetId(),
		Status:    mapKoyebDeployStatus(string(d.GetStatus())),
		CreatedAt: d.GetCreatedAt(),
	}, nil
}

func (k *Koyeb) GetLogs(serviceID string, opts LogOptions) ([]LogEntry, error) {
	limit := 100
	if opts.Tail > 0 {
		limit = opts.Tail
	}

	url := fmt.Sprintf("%s/v1/streams/logs/query?type=runtime&service_id=%s&limit=%d&order=asc", koyebBaseURL, serviceID, limit)
	if opts.Since > 0 {
		start := time.Now().UTC().Add(-opts.Since).Format(time.RFC3339)
		url += "&start=" + start
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+k.token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("koyeb logs API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("invalid token: unauthorized")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("koyeb logs API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Msg       string `json:"msg"`
			CreatedAt string `json:"created_at"`
			Labels    struct {
				Stream string `json:"stream"`
			} `json:"labels"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode logs response: %w", err)
	}

	var entries []LogEntry
	for _, item := range result.Data {
		if item.Msg == "" {
			continue
		}

		level := "info"
		if item.Labels.Stream == "stderr" {
			level = "error"
		}
		if opts.Level != "" && level != opts.Level {
			continue
		}

		ts, _ := time.Parse(time.RFC3339Nano, item.CreatedAt)
		entries = append(entries, LogEntry{
			Timestamp: ts,
			Level:     level,
			Message:   item.Msg,
			Source:    "runtime",
		})
	}

	return entries, nil
}

func (k *Koyeb) Scale(serviceID string, opts ScaleOptions) error {
	// Get current service definition to preserve existing settings
	svc, _, err := k.client.ServicesApi.GetService(k.ctx, serviceID).Execute()
	if err != nil {
		return fmt.Errorf("get service: %w", err)
	}

	service := svc.GetService()
	latestDeployID := service.GetLatestDeploymentId()

	// Get the current deployment definition
	deployReply, _, err := k.client.DeploymentsApi.GetDeployment(k.ctx, latestDeployID).Execute()
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}
	deploy := deployReply.GetDeployment()
	currentDef := deploy.GetDefinition()

	// Build updated definition
	def := koyeb.NewDeploymentDefinition()

	// Update scaling (min/max)
	if opts.MinInstances > 0 || opts.MaxInstances > 0 {
		scaling := koyeb.NewDeploymentScaling()
		// Preserve existing values, override with provided ones
		existingScalings := currentDef.GetScalings()
		if len(existingScalings) > 0 {
			existing := existingScalings[0]
			scaling.SetMin(existing.GetMin())
			scaling.SetMax(existing.GetMax())
			scaling.SetScopes(existing.GetScopes())
		}
		if opts.MinInstances > 0 {
			scaling.SetMin(int64(opts.MinInstances))
		}
		if opts.MaxInstances > 0 {
			scaling.SetMax(int64(opts.MaxInstances))
		}
		def.SetScalings([]koyeb.DeploymentScaling{*scaling})
	} else {
		def.SetScalings(currentDef.GetScalings())
	}

	// Update instance type
	if opts.InstanceType != "" {
		it := koyeb.NewDeploymentInstanceType()
		it.SetType(opts.InstanceType)
		existingTypes := currentDef.GetInstanceTypes()
		if len(existingTypes) > 0 {
			it.SetScopes(existingTypes[0].GetScopes())
		}
		def.SetInstanceTypes([]koyeb.DeploymentInstanceType{*it})
	} else {
		def.SetInstanceTypes(currentDef.GetInstanceTypes())
	}

	// Preserve other definition fields
	if currentDef.HasGit() {
		def.SetGit(currentDef.GetGit())
	}
	if currentDef.HasDocker() {
		def.SetDocker(currentDef.GetDocker())
	}
	def.SetEnv(currentDef.GetEnv())
	def.SetPorts(currentDef.GetPorts())
	def.SetRoutes(currentDef.GetRoutes())
	def.SetRegions(currentDef.GetRegions())

	updateReq := koyeb.NewUpdateService()
	updateReq.SetDefinition(*def)

	_, _, err = k.client.ServicesApi.UpdateService(k.ctx, serviceID).Service(*updateReq).Execute()
	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}

	return nil
}

// GetCurrentScale retrieves the current scaling configuration for a Koyeb service.
func (k *Koyeb) GetCurrentScale(serviceID string) (min, max int, instanceType string, err error) {
	svc, _, err := k.client.ServicesApi.GetService(k.ctx, serviceID).Execute()
	if err != nil {
		return 0, 0, "", fmt.Errorf("get service: %w", err)
	}

	service := svc.GetService()
	latestDeployID := service.GetLatestDeploymentId()

	deployReply, _, err := k.client.DeploymentsApi.GetDeployment(k.ctx, latestDeployID).Execute()
	if err != nil {
		return 0, 0, "", fmt.Errorf("get deployment: %w", err)
	}
	deploy := deployReply.GetDeployment()
	def := deploy.GetDefinition()

	scalings := def.GetScalings()
	if len(scalings) > 0 {
		min = int(scalings[0].GetMin())
		max = int(scalings[0].GetMax())
	}

	instanceTypes := def.GetInstanceTypes()
	if len(instanceTypes) > 0 {
		instanceType = instanceTypes[0].GetType()
	}

	return min, max, instanceType, nil
}

func (k *Koyeb) DiscoverServices() ([]DiscoveredService, error) {
	reply, _, err := k.client.ServicesApi.ListServices(k.ctx).Limit("100").Execute()
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}

	var services []DiscoveredService
	for _, s := range reply.GetServices() {
		services = append(services, DiscoveredService{
			ID:       s.GetId(),
			Name:     s.GetName(),
			Platform: "koyeb",
		})
	}
	return services, nil
}

func (k *Koyeb) WatchDeployment(serviceID string, currentDeployID string) (<-chan DeployEvent, error) {
	ch := make(chan DeployEvent)

	go func() {
		defer close(ch)

		const pollInterval = 3 * time.Second

		// Phase 1: Detect a new deployment
		for {
			deploys, err := k.ListDeployments(serviceID, 1)
			if err != nil {
				ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("poll deployments: %w", err)}
				return
			}

			if len(deploys) > 0 && deploys[0].ID != currentDeployID {
				d := deploys[0]
				ch <- DeployEvent{
					Phase:   "detected",
					Message: fmt.Sprintf("New deployment detected! (%s)", d.ID),
					Deploy:  &d,
				}

				// Phase 2: Track deployment progress
				k.trackDeployment(ch, d.ID)
				return
			}

			ch <- DeployEvent{Phase: "waiting", Message: "Waiting for new deployment..."}
			time.Sleep(pollInterval)
		}
	}()

	return ch, nil
}

func (k *Koyeb) trackDeployment(ch chan<- DeployEvent, deployID string) {
	const pollInterval = 3 * time.Second
	lastPhase := ""

	for {
		deploy, err := k.GetDeployment(deployID)
		if err != nil {
			ch <- DeployEvent{Phase: "failed", Error: fmt.Errorf("get deployment: %w", err)}
			return
		}

		phase := mapKoyebToWatchPhase(deploy.Status)
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
				// Try to get error logs from runtime
				if logs, err := k.getDeploymentErrors(deployID); err == nil {
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

func mapKoyebToWatchPhase(status string) string {
	switch status {
	case "pending":
		return "building"
	case "building":
		return "building"
	case "deploying":
		return "deploying"
	case "healthy":
		return "done"
	case "degraded":
		return "done"
	case "failed":
		return "failed"
	case "sleeping":
		return "done"
	default:
		return "building"
	}
}

func (k *Koyeb) getDeploymentErrors(deployID string) ([]string, error) {
	url := fmt.Sprintf("%s/v1/streams/logs/query?type=runtime&deployment_id=%s&limit=20&order=desc", koyebBaseURL, deployID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+k.token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Msg    string `json:"msg"`
			Labels struct {
				Stream string `json:"stream"`
			} `json:"labels"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var errLogs []string
	for _, item := range result.Data {
		if item.Labels.Stream == "stderr" && item.Msg != "" {
			errLogs = append(errLogs, item.Msg)
		}
	}
	return errLogs, nil
}

