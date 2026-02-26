package cmd

import (
	"fmt"
	"sort"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var connectionsCmd = &cobra.Command{
	Use:   "connections",
	Short: "List all connected platforms and their status",
	RunE:  runConnections,
}

func init() {
	rootCmd.AddCommand(connectionsCmd)
}

func runConnections(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(cfg.Platforms) == 0 {
		fmt.Println("No platforms connected.")
		fmt.Println("Use `orbit connect <platform>` to connect one.")
		return nil
	}

	key, err := config.LoadOrCreateKey()
	if err != nil {
		return fmt.Errorf("load encryption key: %w", err)
	}

	// Sort platform names for consistent output
	names := make([]string, 0, len(cfg.Platforms))
	for name := range cfg.Platforms {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Println(ui.HeaderStyle.Render("Platform") +
		ui.HeaderStyle.Render("Status") +
		ui.HeaderStyle.Render("Info"))
	fmt.Println("─────────────────────────────────────────────")

	for _, name := range names {
		pc := cfg.Platforms[name]
		token, err := config.Decrypt(key, pc.Token)
		if err != nil {
			fmt.Printf("%-12s %s  %s\n",
				ui.CellStyle.Render(name),
				ui.ErrorStyle.Render(ui.IconError+" error"),
				ui.MutedStyle.Render("decrypt failed"),
			)
			continue
		}

		p, err := platform.Get(name, token)
		if err != nil {
			fmt.Printf("%-12s %s  %s\n",
				ui.CellStyle.Render(name),
				ui.ErrorStyle.Render(ui.IconError+" error"),
				ui.MutedStyle.Render("unknown platform"),
			)
			continue
		}

		if err := p.Validate(token); err != nil {
			fmt.Printf("%-12s %s  %s\n",
				ui.CellStyle.Render(name),
				ui.ErrorStyle.Render(ui.IconError+" invalid"),
				ui.MutedStyle.Render(err.Error()),
			)
		} else {
			fmt.Printf("%-12s %s\n",
				ui.CellStyle.Render(name),
				ui.HealthyStyle.Render(ui.IconHealthy+" connected"),
			)
		}
	}

	return nil
}
