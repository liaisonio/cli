package cli

import (
	"encoding/json"
	"fmt"

	"github.com/liaisonio/cli/internal/client"
	"github.com/liaisonio/cli/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var (
		token     string
		server    string
		name      string
		noBrowser bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with liaison.cloud and persist a token locally",
		Long: `Authenticate with liaison.cloud and store the resulting Personal Access Token
in ~/.liaison/config.yaml so subsequent commands can use it without flags.

Three modes:

  liaison login                       # default: opens a browser, you approve
                                      # in the web UI, the CLI receives the
                                      # PAT via a localhost callback

  liaison login --no-browser          # same flow, but prints the URL instead
                                      # of auto-opening (useful over SSH)

  liaison login --token liaison_pat_x # skip the browser flow and save an
                                      # already-obtained PAT directly

Examples:
  liaison login
  liaison login --name agent-prod
  liaison login --no-browser --server https://staging.liaison.cloud
  liaison login --token liaison_pat_a1b2c3d4e5f6...
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			persisted, err := config.Load(flags.configPath)
			if err != nil {
				return err
			}
			// Resolve server: explicit flag > existing config > built-in default.
			if server != "" {
				persisted.Server = server
			} else if persisted.Server == "" {
				persisted.Server = config.DefaultServer
			}

			// Mode A: explicit --token bypass.
			if token != "" {
				return saveAndVerifyToken(cmd, persisted, token)
			}

			// Mode B: browser flow (default).
			if name == "" {
				name = defaultTokenName()
			}
			obtained, err := browserLogin(persisted.Server, name, noBrowser, cmd.OutOrStderr())
			if err != nil {
				return err
			}
			return saveAndVerifyToken(cmd, persisted, obtained)
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "skip the browser flow and save this token directly")
	cmd.Flags().StringVar(&server, "server", "", "override server URL")
	cmd.Flags().StringVar(&name, "name", "", "name for the new token (default: cli-<host>-<yyyymmdd>)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print URL instead of auto-opening (for SSH/headless)")
	return cmd
}

// saveAndVerifyToken verifies the token works against the server, then writes
// it to the config file. Verification first means we never persist a broken
// token to disk.
func saveAndVerifyToken(cmd *cobra.Command, cfg *config.Config, tok string) error {
	cfg.Token = tok
	c := client.New(cfg.Server, cfg.Token, flags.insecure, flags.verbose)
	data, err := c.Get("/api/v1/iam/profile_json", nil)
	if err != nil {
		return fmt.Errorf("token verification failed: %w", err)
	}
	var user client.CurrentUser
	if err := json.Unmarshal(data, &user); err != nil {
		return fmt.Errorf("decode user: %w", err)
	}
	if err := config.Save(flags.configPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	path, _ := config.DefaultPath()
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Logged in as %s\n  Credentials saved to %s\n", user.Username, path)
	return nil
}
