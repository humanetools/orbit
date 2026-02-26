package platform

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const supabaseBaseURL = "https://api.supabase.com"

func init() {
	Register("supabase", func(token string) Platform {
		return NewSupabase(token)
	})
}

// Supabase implements the Platform interface using net/http (Management API).
type Supabase struct {
	token      string
	httpClient *http.Client
}

// NewSupabase creates a new Supabase platform instance.
func NewSupabase(token string) *Supabase {
	return &Supabase{
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Supabase) Name() string {
	return "supabase"
}

func (s *Supabase) doRequest(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, supabaseBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	return s.httpClient.Do(req)
}

// Validate checks whether the token is valid by calling GET /v1/projects.
func (s *Supabase) Validate(token string) error {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", supabaseBaseURL+"/v1/projects", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("supabase API error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("invalid token: unauthorized")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("supabase API returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Supabase) GetServiceStatus(serviceID string) (*ServiceStatus, error) {
	resp, err := s.doRequest("GET", fmt.Sprintf("/v1/projects/%s/health?services=auth&services=db&services=realtime&services=rest&services=storage", serviceID))
	if err != nil {
		return nil, fmt.Errorf("get health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("project not found: %s", serviceID)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("supabase API returned status %d", resp.StatusCode)
	}

	var health []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	status := &ServiceStatus{
		Status: "healthy",
	}
	for _, h := range health {
		if h.Status == "UNHEALTHY" || h.Status == "ERROR" {
			status.Status = "unhealthy"
			break
		}
		if h.Status == "COMING_UP" || h.Status == "INACTIVE" {
			status.Status = "sleeping"
		}
	}
	return status, nil
}

func (s *Supabase) ListDeployments(serviceID string, limit int) ([]Deployment, error) {
	// Supabase doesn't have a traditional deployment concept
	return nil, fmt.Errorf("not supported: supabase does not track deployments")
}

func (s *Supabase) GetDeployment(deployID string) (*Deployment, error) {
	return nil, fmt.Errorf("not supported: supabase does not track deployments")
}

func (s *Supabase) Redeploy(serviceID string) (*Deployment, error) {
	return nil, fmt.Errorf("not supported: use supabase dashboard to manage projects")
}

func (s *Supabase) GetLogs(serviceID string, opts LogOptions) ([]LogEntry, error) {
	return nil, fmt.Errorf("not supported: supabase logs are only available via the Supabase dashboard")
}

func (s *Supabase) Scale(serviceID string, opts ScaleOptions) error {
	return fmt.Errorf("not supported: use the Supabase dashboard to change project plans")
}

func (s *Supabase) DiscoverServices() ([]DiscoveredService, error) {
	resp, err := s.doRequest("GET", "/v1/projects")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("supabase API returned status %d", resp.StatusCode)
	}

	var projects []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var services []DiscoveredService
	for _, p := range projects {
		services = append(services, DiscoveredService{
			ID:       p.ID,
			Name:     p.Name,
			Platform: "supabase",
		})
	}
	return services, nil
}

func (s *Supabase) WatchDeployment(serviceID string, currentDeployID string) (<-chan DeployEvent, error) {
	return nil, fmt.Errorf("not supported: supabase does not support deployment watching")
}
