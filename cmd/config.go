package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or modify Orbit configuration",
	Long: `View or modify Orbit configuration.

  orbit config                                    Show current config
  orbit config set default-project myshop          Set default project
  orbit config set threshold.response-time 500     Set response time threshold (ms)
  orbit config set threshold.cpu 80                Set CPU threshold (%)
  orbit config set threshold.memory 85             Set memory threshold (%)`,
	RunE: runConfigShow,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dir, err := config.Dir()
	if err != nil {
		return fmt.Errorf("config dir: %w", err)
	}

	fmt.Printf("\n  %s\n\n", ui.ProjectTitleStyle.Render("Orbit Configuration"))

	fmt.Printf("  Config:          %s/config.yaml\n", dir)

	if cfg.DefaultProject != "" {
		fmt.Printf("  Default project: %s\n", ui.HealthyStyle.Render(cfg.DefaultProject))
	} else {
		fmt.Printf("  Default project: %s\n", ui.MutedStyle.Render("(not set)"))
	}

	fmt.Printf("  Platforms:       %s\n", ui.MutedStyle.Render(fmt.Sprintf("%d connected", len(cfg.Platforms))))
	fmt.Printf("  Projects:        %s\n", ui.MutedStyle.Render(fmt.Sprintf("%d configured", len(cfg.Projects))))

	fmt.Printf("\n  %s\n", ui.ProjectTitleStyle.Render("Thresholds"))
	fmt.Printf("  Response time:   %dms\n", cfg.Thresholds.ResponseTimeMs)
	fmt.Printf("  CPU:             %d%%\n", cfg.Thresholds.CPUPercent)
	fmt.Printf("  Memory:          %d%%\n", cfg.Thresholds.MemoryPercent)

	fmt.Println()
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := strings.ToLower(args[0])
	value := args[1]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	switch key {
	case "default-project", "default_project":
		if value != "" {
			if _, ok := cfg.Projects[value]; !ok {
				return fmt.Errorf("project %q not found\nAvailable projects: %s", value, projectNames(cfg))
			}
		}
		cfg.DefaultProject = value

	case "threshold.response-time", "threshold.response_time", "threshold.response_time_ms":
		v, err := strconv.Atoi(strings.TrimSuffix(value, "ms"))
		if err != nil {
			return fmt.Errorf("invalid value %q: expected integer (ms)", value)
		}
		cfg.Thresholds.ResponseTimeMs = v

	case "threshold.cpu", "threshold.cpu_percent":
		v, err := strconv.Atoi(strings.TrimSuffix(value, "%"))
		if err != nil {
			return fmt.Errorf("invalid value %q: expected integer (%%)", value)
		}
		cfg.Thresholds.CPUPercent = v

	case "threshold.memory", "threshold.memory_percent":
		v, err := strconv.Atoi(strings.TrimSuffix(value, "%"))
		if err != nil {
			return fmt.Errorf("invalid value %q: expected integer (%%)", value)
		}
		cfg.Thresholds.MemoryPercent = v

	default:
		return fmt.Errorf("unknown config key: %s\nValid keys: default-project, threshold.response-time, threshold.cpu, threshold.memory", key)
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("  %s %s = %s\n", ui.IconSuccess, key, value)
	return nil
}
