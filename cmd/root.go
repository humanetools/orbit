package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/humanetools/orbit/internal/version"
	"github.com/spf13/cobra"
)

// ExitCodeError allows commands to specify a custom process exit code.
type ExitCodeError struct {
	Code int
	Msg  string
}

func (e *ExitCodeError) Error() string { return e.Msg }

var showVersion bool

var rootCmd = &cobra.Command{
	Use:   "orbit",
	Short: "Monitor services deployed across multiple cloud platforms",
	Long: `Orbit is a unified CLI tool for monitoring services
deployed across multiple cloud platforms such as Vercel, Koyeb, and Supabase.

Get a single-pane view of deployments, logs, health status, and more.`,
	Run: func(cmd *cobra.Command, args []string) {
		if showVersion {
			fmt.Println(version.Full())
			return
		}
		cmd.Help()
	},
}

func init() {
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Print version information")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(1)
	}
}
