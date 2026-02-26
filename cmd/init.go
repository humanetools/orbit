package cmd

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup wizard for Orbit",
	Long: `Launch an interactive wizard that walks you through:
  1. Selecting cloud platforms to connect
  2. Entering and validating API tokens
  3. Naming your project
  4. Selecting discovered services to monitor`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	p := tea.NewProgram(ui.NewWizardModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("wizard error: %w", err)
	}
	return nil
}
