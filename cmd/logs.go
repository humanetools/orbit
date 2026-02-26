package cmd

import (
	"fmt"
	"time"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
)

var (
	logsService string
	logsFollow  bool
	logsLevel   string
	logsTail    int
	logsSince   string
)

var logsCmd = &cobra.Command{
	Use:   "logs <project>",
	Short: "View service logs",
	Long: `View logs for a service in a project.

  orbit logs myshop --service api
  orbit logs myshop --service api --follow
  orbit logs myshop --service api --level error
  orbit logs myshop --service api --tail 50
  orbit logs myshop --service api --since 2h`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().StringVar(&logsService, "service", "", "Service name (required)")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Stream logs in real time")
	logsCmd.Flags().StringVar(&logsLevel, "level", "", "Filter by log level (info, error)")
	logsCmd.Flags().IntVar(&logsTail, "tail", 0, "Show last N log entries")
	logsCmd.Flags().StringVar(&logsSince, "since", "", "Show logs since duration (e.g. 1h, 30m, 2h30m)")
	logsCmd.MarkFlagRequired("service")
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	key, err := config.LoadOrCreateKey()
	if err != nil {
		return fmt.Errorf("load encryption key: %w", err)
	}

	projectName := ""
	if len(args) > 0 {
		projectName = args[0]
	} else {
		projectName = cfg.DefaultProject
	}

	resolved, err := resolveService(cfg, key, projectName, logsService)
	if err != nil {
		return err
	}

	opts := platform.LogOptions{
		Follow: logsFollow,
		Level:  logsLevel,
		Tail:   logsTail,
	}

	if logsSince != "" {
		d, err := time.ParseDuration(logsSince)
		if err != nil {
			return fmt.Errorf("invalid --since value %q: %w", logsSince, err)
		}
		opts.Since = d
	}

	if logsFollow {
		return runLogsFollow(resolved, opts)
	}

	entries, err := resolved.Platform.GetLogs(resolved.Entry.ID, opts)
	if err != nil {
		return fmt.Errorf("get logs: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println(ui.MutedStyle.Render("No log entries found."))
		return nil
	}

	for _, e := range entries {
		printLogEntry(e)
	}
	return nil
}

func runLogsFollow(resolved *resolvedService, opts platform.LogOptions) error {
	fmt.Printf("%s Streaming logs for %s/%s (%s)... press Ctrl+C to stop\n\n",
		ui.IconWatch,
		resolved.Entry.Platform,
		resolved.Entry.Name,
		resolved.Entry.ID,
	)

	// Track the latest timestamp to avoid duplicates
	var lastTimestamp time.Time

	for {
		// Adjust since to only get new entries
		if !lastTimestamp.IsZero() {
			opts.Since = time.Since(lastTimestamp)
		}
		opts.Tail = 0 // Don't limit in follow mode after initial fetch

		entries, err := resolved.Platform.GetLogs(resolved.Entry.ID, opts)
		if err != nil {
			fmt.Printf("%s %s\n", ui.IconWarning, ui.ErrorStyle.Render("error fetching logs: "+err.Error()))
		}

		for _, e := range entries {
			if !e.Timestamp.After(lastTimestamp) {
				continue
			}
			printLogEntry(e)
			lastTimestamp = e.Timestamp
		}

		time.Sleep(3 * time.Second)
	}
}

func printLogEntry(e platform.LogEntry) {
	ts := e.Timestamp.Format("15:04:05")

	levelStr := ui.MutedStyle.Render(e.Level)
	switch e.Level {
	case "error":
		levelStr = ui.ErrorStyle.Render("ERR")
	case "warn", "warning":
		levelStr = ui.WarningStyle.Render("WRN")
	case "info":
		levelStr = ui.HealthyStyle.Render("INF")
	}

	fmt.Printf("%s %s %s\n",
		ui.MutedStyle.Render(ts),
		levelStr,
		e.Message,
	)
}
