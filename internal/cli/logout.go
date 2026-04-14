package cli

import (
	"fmt"

	"github.com/liaisonio/cli/internal/config"
	"github.com/spf13/cobra"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear the saved auth token",
		Long: `Clear the token from ~/.liaison/config.yaml. The server URL is preserved
so subsequent logins can reuse it without re-specifying.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			persisted, err := config.Load(flags.configPath)
			if err != nil {
				return err
			}
			if persisted.Token == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Already logged out")
				return nil
			}
			persisted.Token = ""
			if err := config.Save(flags.configPath, persisted); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out")
			return nil
		},
	}
}
