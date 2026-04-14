package cli

import (
	"encoding/json"
	"fmt"

	"github.com/liaisonio/cli/internal/client"
	"github.com/liaisonio/cli/internal/output"
	"github.com/spf13/cobra"
)

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the currently authenticated user",
		Long:  "Call /api/v1/iam/profile_json with the configured token and print the result.",
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			data, err := r.client.Get("/api/v1/iam/profile_json", nil)
			if err != nil {
				return err
			}
			if r.fmt == output.FormatTable {
				var u client.CurrentUser
				if err := json.Unmarshal(data, &u); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "ID:\t%d\nUsername:\t%s\nEmail:\t%s\nPhone:\t%s\n", u.ID, u.Username, u.Email, u.Phone)
				return nil
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
}
