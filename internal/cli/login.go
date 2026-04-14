package cli

import (
	"encoding/json"
	"fmt"

	"github.com/liaison-cloud/cli/internal/client"
	"github.com/liaison-cloud/cli/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var (
		token  string
		server string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Persist an auth token to the config file",
		Long: `Persist an auth token to ~/.liaison/config.yaml so subsequent commands can
use it without needing to pass --token or set LIAISON_TOKEN.

The token is a JWT issued by liaison.cloud. For now the CLI does not support
the interactive slider-captcha login flow; obtain the token from:

  1. The web UI → browser dev tools → Application → Local Storage → authorization
  2. Or an administrator-issued personal access token (future)

Examples:
  liaison login --token eyJhbGciOi...
  liaison login --token eyJhbGciOi... --server https://liaison.cloud
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if token == "" {
				return fmt.Errorf("--token is required")
			}

			persisted, err := config.Load(flags.configPath)
			if err != nil {
				return err
			}
			if server != "" {
				persisted.Server = server
			} else if persisted.Server == "" {
				persisted.Server = config.DefaultServer
			}
			persisted.Token = token

			// Verify the token before persisting — fail fast if it's wrong.
			c := client.New(persisted.Server, persisted.Token, flags.insecure, flags.verbose)
			data, err := c.Get("/api/v1/iam/profile_json", nil)
			if err != nil {
				return fmt.Errorf("token verification failed: %w", err)
			}
			var user client.CurrentUser
			if err := json.Unmarshal(data, &user); err != nil {
				return fmt.Errorf("decode user: %w", err)
			}

			if err := config.Save(flags.configPath, persisted); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			path, _ := config.DefaultPath()
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s (id=%d) — credentials saved to %s\n", user.Username, user.ID, path)
			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "JWT bearer token (required)")
	cmd.Flags().StringVar(&server, "server", "", "override server URL before saving")
	return cmd
}
