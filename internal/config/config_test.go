package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Check defaults
	if cfg.Thresholds.ResponseTimeMs != 500 {
		t.Errorf("ResponseTimeMs: got %d, want 500", cfg.Thresholds.ResponseTimeMs)
	}
	if cfg.Thresholds.CPUPercent != 80 {
		t.Errorf("CPUPercent: got %d, want 80", cfg.Thresholds.CPUPercent)
	}
	if cfg.Thresholds.MemoryPercent != 85 {
		t.Errorf("MemoryPercent: got %d, want 85", cfg.Thresholds.MemoryPercent)
	}

	// Maps should be initialized
	if cfg.Platforms == nil {
		t.Error("Platforms map should not be nil")
	}
	if cfg.Projects == nil {
		t.Error("Projects map should not be nil")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	original := &Config{
		DefaultProject: "myshop",
		Platforms: map[string]PlatformConfig{
			"vercel": {Token: "ENC:abc123"},
			"koyeb":  {Token: "ENC:def456"},
		},
		Projects: map[string]ProjectConfig{
			"myshop": {
				Topology: []ServiceEntry{
					{Name: "frontend", Platform: "vercel", ID: "prj_xxxx"},
					{Name: "api", Platform: "koyeb", ID: "svc_xxxx"},
				},
			},
		},
		Thresholds: ThresholdConfig{
			ResponseTimeMs: 300,
			CPUPercent:     70,
			MemoryPercent:  75,
		},
	}

	if err := Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpHome, ".orbit", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.DefaultProject != original.DefaultProject {
		t.Errorf("DefaultProject: got %q, want %q", loaded.DefaultProject, original.DefaultProject)
	}

	if len(loaded.Platforms) != 2 {
		t.Errorf("Platforms count: got %d, want 2", len(loaded.Platforms))
	}

	if loaded.Platforms["vercel"].Token != "ENC:abc123" {
		t.Errorf("Vercel token: got %q, want %q", loaded.Platforms["vercel"].Token, "ENC:abc123")
	}

	proj, ok := loaded.Projects["myshop"]
	if !ok {
		t.Fatal("myshop project not found")
	}
	if len(proj.Topology) != 2 {
		t.Errorf("Topology count: got %d, want 2", len(proj.Topology))
	}

	if loaded.Thresholds.ResponseTimeMs != 300 {
		t.Errorf("ResponseTimeMs: got %d, want 300", loaded.Thresholds.ResponseTimeMs)
	}
}

func TestEnsureDir(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dir, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("dir permissions: got %o, want 0700", perm)
	}
}
