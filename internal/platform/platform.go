package platform

import (
	"fmt"
	"time"
)

// ServiceStatus represents the normalized status of a service.
type ServiceStatus struct {
	Status       string        // healthy, degraded, unhealthy, sleeping
	ResponseMs   int           // average response time in ms
	CPU          float64       // CPU usage percentage
	Memory       float64       // Memory usage percentage
	Instances    int           // current running instances
	MaxInstances int           // maximum configured instances
	LastDeploy   *Deployment   // most recent deployment
}

// Deployment represents a single deployment event.
type Deployment struct {
	ID        string
	Status    string // pending, building, deploying, healthy, failed, sleeping
	Commit    string
	Message   string
	CreatedAt time.Time
	Duration  time.Duration
	URL       string
}

// DeployEvent represents a real-time deployment state change.
type DeployEvent struct {
	Phase   string // waiting, detected, building, deploying, healthcheck, done, failed
	Message string
	Deploy  *Deployment
	Error   error
	Logs    []string // error logs when failed
}

// LogEntry represents a single log line.
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Source    string
}

// LogOptions controls log retrieval.
type LogOptions struct {
	Follow bool
	Level  string
	Tail   int
	Since  time.Duration
}

// ScaleOptions controls scaling parameters.
type ScaleOptions struct {
	MinInstances int
	MaxInstances int
	InstanceType string
}

// ScaleInfoProvider is implemented by platforms that can report current scaling config.
type ScaleInfoProvider interface {
	GetCurrentScale(serviceID string) (min, max int, instanceType string, err error)
}

// Platform defines the interface all cloud platform adapters must implement.
type Platform interface {
	Name() string
	Validate(token string) error
	GetServiceStatus(serviceID string) (*ServiceStatus, error)
	ListDeployments(serviceID string, limit int) ([]Deployment, error)
	GetDeployment(deployID string) (*Deployment, error)
	Redeploy(serviceID string) (*Deployment, error)
	GetLogs(serviceID string, opts LogOptions) ([]LogEntry, error)
	Scale(serviceID string, opts ScaleOptions) error
	WatchDeployment(serviceID string, currentDeployID string) (<-chan DeployEvent, error)
}

// Constructor creates a new Platform instance with the given API token.
type Constructor func(token string) Platform

// registry maps platform names to their constructors.
var registry = map[string]Constructor{}

// Register adds a platform constructor to the registry.
func Register(name string, ctor Constructor) {
	registry[name] = ctor
}

// Get returns a Platform instance for the given name and token.
func Get(name, token string) (Platform, error) {
	ctor, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown platform: %s", name)
	}
	return ctor(token), nil
}

// Names returns all registered platform names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// IsSupported checks if a platform name is registered.
func IsSupported(name string) bool {
	_, ok := registry[name]
	return ok
}

// isInProgress returns true if the deployment status indicates a non-terminal state.
// Used by WatchDeployment to detect in-progress deployments that started before watch began.
func isInProgress(status string) bool {
	switch status {
	case "building", "deploying", "pending":
		return true
	default:
		return false
	}
}

// TokenURL returns the URL where users can obtain an API token for a platform.
func TokenURL(name string) string {
	switch name {
	case "vercel":
		return "https://vercel.com/account/tokens"
	case "koyeb":
		return "https://app.koyeb.com/account/api"
	case "supabase":
		return "https://supabase.com/dashboard/account/tokens"
	default:
		return ""
	}
}
