package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/humanetools/orbit/internal/config"
	"github.com/humanetools/orbit/internal/platform"
	"github.com/humanetools/orbit/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var connectToken string

var connectCmd = &cobra.Command{
	Use:   "connect <platform>",
	Short: "Connect a cloud platform with an API token",
	Long: `Connect a cloud platform by providing an API token.
Supported platforms: vercel, koyeb, supabase.

The token is validated against the platform API, then encrypted and stored locally.`,
	Args: cobra.ExactArgs(1),
	RunE: runConnect,
}

func init() {
	connectCmd.Flags().StringVar(&connectToken, "token", "", "API token (non-interactive mode)")
	rootCmd.AddCommand(connectCmd)
}

func runConnect(cmd *cobra.Command, args []string) error {
	name := strings.ToLower(args[0])

	if !platform.IsSupported(name) {
		return fmt.Errorf("unsupported platform: %s\nSupported: vercel, koyeb, supabase", name)
	}

	token := connectToken

	// Interactive mode: prompt for token
	if token == "" {
		tokenURL := platform.TokenURL(name)
		if tokenURL != "" {
			fmt.Printf("  Get your token at: %s\n", ui.MutedStyle.Render(tokenURL))
		}
		fmt.Printf("%s API Token: ", strings.Title(name))

		// Read token with echo disabled
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		fmt.Println() // newline after hidden input
		token = strings.TrimSpace(string(raw))
	}

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	// Validate token against the platform API
	p, err := platform.Get(name, token)
	if err != nil {
		return err
	}

	fmt.Printf("  Validating token... ")
	if err := p.Validate(token); err != nil {
		fmt.Println(ui.ErrorStyle.Render("failed"))
		return fmt.Errorf("token validation failed: %w", err)
	}
	fmt.Println(ui.HealthyStyle.Render("valid"))

	// Encrypt and save
	key, err := config.LoadOrCreateKey()
	if err != nil {
		return fmt.Errorf("load encryption key: %w", err)
	}

	encrypted, err := config.Encrypt(key, token)
	if err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.Platforms[name] = config.PlatformConfig{Token: encrypted}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("\n%s %s connected successfully!\n", ui.IconSuccess, strings.Title(name))
	return nil
}
