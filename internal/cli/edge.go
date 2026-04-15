package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/liaisonio/cli/internal/client"
	"github.com/liaisonio/cli/internal/output"
	"github.com/spf13/cobra"
)

func newEdgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "edge",
		Aliases: []string{"edges", "connector", "connectors"},
		Short:   "Manage connectors (edges)",
		Long:    "Connectors are the edge agents that register with liaison-cloud and expose applications.",
	}
	cmd.AddCommand(
		newEdgeListCmd(),
		newEdgeGetCmd(),
		newEdgeCreateCmd(),
		newEdgeUpdateCmd(),
		newEdgeDeleteCmd(),
	)
	return cmd
}

// ─── list ────────────────────────────────────────────────────────────────────

func newEdgeListCmd() *cobra.Command {
	var (
		page     int
		pageSize int
		name     string
		online   int
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List connectors",
		Long: `List connectors belonging to the authenticated user.

Filter flags map directly to the underlying API query parameters.
  --online 1   only online (connected) connectors
  --online 2   only offline connectors
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			q := url.Values{}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			if pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pageSize))
			}
			if name != "" {
				q.Set("name", name)
			}
			if online > 0 {
				q.Set("online", strconv.Itoa(online))
			}
			data, err := r.client.Get("/api/v1/edges", q)
			if err != nil {
				return err
			}
			if r.fmt == output.FormatTable {
				var list client.EdgeList
				if err := json.Unmarshal(data, &list); err != nil {
					return err
				}
				rows := make([][]string, 0, len(list.Edges))
				for _, e := range list.Edges {
					rows = append(rows, []string{
						strconv.FormatUint(e.ID.Uint64(), 10),
						e.Name,
						edgeStatusLabel(e.Status),
						edgeOnlineLabel(e.Online),
						strconv.Itoa(e.ApplicationCount),
						e.CreatedAt,
					})
				}
				return output.PrintTable(cmd.OutOrStdout(),
					[]string{"ID", "NAME", "STATUS", "ONLINE", "APPS", "CREATED"},
					rows)
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "results per page")
	cmd.Flags().StringVar(&name, "name", "", "filter by name (substring match)")
	cmd.Flags().IntVar(&online, "online", 0, "filter by online status: 1=online, 2=offline")
	return cmd
}

// ─── get ─────────────────────────────────────────────────────────────────────

func newEdgeGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a connector by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id: %w", err)
			}
			data, err := r.client.Get(fmt.Sprintf("/api/v1/edges/%d", id), nil)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
}

// ─── create ──────────────────────────────────────────────────────────────────

func newEdgeCreateCmd() *cobra.Command {
	var (
		name        string
		description string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new connector and print its install command",
		Long: `Create a new connector. The response includes the generated AccessKey/SecretKey
pair and a one-line install command that the target machine can run to bootstrap
the edge agent.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			body := map[string]any{}
			if name != "" {
				body["name"] = name
			}
			if description != "" {
				body["description"] = description
			}
			data, err := r.client.Post("/api/v1/edges", body)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "connector name (auto-generated if empty)")
	cmd.Flags().StringVar(&description, "description", "", "connector description")
	return cmd
}

// ─── update ──────────────────────────────────────────────────────────────────

func newEdgeUpdateCmd() *cobra.Command {
	var (
		name        string
		description string
		status      string
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a connector",
		Long: `Update a connector's metadata or status. Only the flags you pass are sent
to the server — other fields are left untouched.

The --status flag accepts running|stopped (or 1|2). Setting status=stopped
kicks the connected edge and prevents it from reconnecting until re-enabled.

Examples:
  liaison edge update 100017 --name new-name
  liaison edge update 100017 --status stopped     # disable
  liaison edge update 100017 --status running     # re-enable
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id: %w", err)
			}
			body := map[string]any{}
			if name != "" {
				body["name"] = name
			}
			if description != "" {
				body["description"] = description
			}
			if status != "" {
				code, err := parseEdgeStatus(status)
				if err != nil {
					return err
				}
				body["status"] = code
			}
			if len(body) == 0 {
				return fmt.Errorf("nothing to update (pass at least one of --name, --description, --status)")
			}
			data, err := r.client.Put(fmt.Sprintf("/api/v1/edges/%d", id), body)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringVar(&status, "status", "", "running|stopped (or 1|2)")
	return cmd
}

// ─── delete ──────────────────────────────────────────────────────────────────

func newEdgeDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"rm"},
		Short:   "Delete a connector",
		Long: `Permanently delete a connector and its associated applications and entries.
Pass --yes to skip the confirmation prompt (required for non-interactive use).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			if !yes {
				return fmt.Errorf("refusing to delete without --yes (non-interactive safety)")
			}
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id: %w", err)
			}
			_, err = r.client.Delete(fmt.Sprintf("/api/v1/edges/%d", id))
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted edge %d\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion (required)")
	return cmd
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func parseEdgeStatus(s string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "running", "run", "enabled", "enable", "on":
		return 1, nil
	case "2", "stopped", "stop", "disabled", "disable", "off":
		return 2, nil
	}
	return 0, fmt.Errorf("invalid status %q (want running|stopped)", s)
}

func edgeStatusLabel(s int) string {
	switch s {
	case 1:
		return "running"
	case 2:
		return "stopped"
	}
	return strconv.Itoa(s)
}

func edgeOnlineLabel(o int) string {
	switch o {
	case 1:
		return "online"
	case 2:
		return "offline"
	}
	return strconv.Itoa(o)
}
