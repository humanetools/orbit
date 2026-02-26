package cmd

import (
	"fmt"
	"strings"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var disconnectCmd = &cobra.Command{
	Use:   "disconnect <platform>",
	Short: "Disconnect a cloud platform and remove its token",
	Args:  cobra.ExactArgs(1),
	RunE:  runDisconnect,
}

func init() {
	rootCmd.AddCommand(disconnectCmd)
}

func runDisconnect(cmd *cobra.Command, args []string) error {
	name := strings.ToLower(args[0])

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if _, exists := cfg.Platforms[name]; !exists {
		return fmt.Errorf("platform %q is not connected", name)
	}

	delete(cfg.Platforms, name)

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("%s %s disconnected.\n", ui.IconSuccess, strings.Title(name))
	return nil
}
