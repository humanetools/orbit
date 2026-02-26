package cmd

import (
	"fmt"
	"sort"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
)

// resolvedService holds everything needed to interact with a specific service.
type resolvedService struct {
	Entry    config.ServiceEntry
	Platform platform.Platform
	Token    string
}

// resolveProject validates that a project exists and returns its config.
func resolveProject(cfg *config.Config, name string) (*config.ProjectConfig, error) {
	if name == "" {
		name = cfg.DefaultProject
	}
	if name == "" {
		return nil, fmt.Errorf("no project specified and no default project set\nUse: orbit <command> <project>")
	}
	proj, ok := cfg.Projects[name]
	if !ok {
		names := make([]string, 0, len(cfg.Projects))
		for n := range cfg.Projects {
			names = append(names, n)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("project %q not found\nAvailable projects: %s", name, joinNames(names))
	}
	return &proj, nil
}

// resolveService finds a service within a project and returns a ready-to-use platform client.
func resolveService(cfg *config.Config, key []byte, projectName, serviceName string) (*resolvedService, error) {
	proj, err := resolveProject(cfg, projectName)
	if err != nil {
		return nil, err
	}

	var entry *config.ServiceEntry
	var svcNames []string
	for i := range proj.Topology {
		svcNames = append(svcNames, proj.Topology[i].Name)
		if proj.Topology[i].Name == serviceName {
			entry = &proj.Topology[i]
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("service %q not found in project %q\nAvailable services: %s",
			serviceName, projectName, joinNames(svcNames))
	}

	pc, ok := cfg.Platforms[entry.Platform]
	if !ok {
		return nil, fmt.Errorf("platform %q not connected\nRun: orbit connect %s", entry.Platform, entry.Platform)
	}

	token, err := config.Decrypt(key, pc.Token)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}

	p, err := platform.Get(entry.Platform, token)
	if err != nil {
		return nil, err
	}

	return &resolvedService{
		Entry:    *entry,
		Platform: p,
		Token:    token,
	}, nil
}
