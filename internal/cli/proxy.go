package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/liaisonio/cli/internal/client"
	"github.com/liaisonio/cli/internal/output"
	"github.com/spf13/cobra"
)

func newProxyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "proxy",
		Aliases: []string{"proxies", "entry", "entries"},
		Short:   "Manage entries (public proxies)",
		Long:    "Entries (aka proxies) expose an application from behind a connector to the public internet.",
	}
	cmd.AddCommand(
		newProxyListCmd(),
		newProxyGetCmd(),
		newProxyCreateCmd(),
		newProxyUpdateCmd(),
		newProxyDeleteCmd(),
	)
	return cmd
}

func newProxyListCmd() *cobra.Command {
	var (
		page     int
		pageSize int
		name     string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entries",
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
			data, err := r.client.Get("/api/v1/proxies", q)
			if err != nil {
				return err
			}
			if r.fmt == output.FormatTable {
				var list client.ProxyList
				if err := json.Unmarshal(data, &list); err != nil {
					return err
				}
				rows := make([][]string, 0, len(list.Proxies))
				for _, p := range list.Proxies {
					rows = append(rows, []string{
						strconv.FormatUint(p.ID.Uint64(), 10),
						p.Name,
						p.Protocol,
						strconv.Itoa(p.Port),
						p.Domain,
						p.Status,
					})
				}
				return output.PrintTable(cmd.OutOrStdout(),
					[]string{"ID", "NAME", "PROTOCOL", "PORT", "DOMAIN", "STATUS"},
					rows)
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "results per page")
	cmd.Flags().StringVar(&name, "name", "", "filter by name (substring)")
	return cmd
}

func newProxyGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get an entry by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			id, err := strconv.ParseUint(args[0], 10, 64)
			if err != nil {
				return err
			}
			data, err := r.client.Get(fmt.Sprintf("/api/v1/proxies/%d", id), nil)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
}

func newProxyCreateCmd() *cobra.Command {
	var (
		name          string
		description   string
		protocol      string
		port          int
		domain        string
		domainLabel   string
		applicationID uint64
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new entry",
		Long: `Create a new entry that exposes an application via a public port or domain.

For TCP-like protocols (tcp|ssh|rdp|mysql|postgresql|redis|mongodb), leave
--port at 0 to have the server auto-allocate one.

For HTTP entries, the server requires a "domain label" — the subdomain under
liaison.cloud. Pass --domain-label explicitly, or leave it empty and the CLI
will derive one from --name. --domain is only needed when binding a custom
BYO domain on top of the label.

Examples:
  # HTTP — auto-derive label from --name (result: <name>-<user>.liaison.cloud)
  liaison proxy create --name ongrid-web --protocol http --application-id 100077

  # HTTP — pick an explicit label
  liaison proxy create --name web --protocol http --domain-label myapp --application-id 100077

  # SSH — server allocates a public port
  liaison proxy create --name my-ssh --protocol ssh --application-id 100033`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			if applicationID == 0 {
				return fmt.Errorf("--application-id is required")
			}
			body := map[string]any{
				"name":           name,
				"description":    description,
				"protocol":       protocol,
				"port":           port,
				"application_id": applicationID,
			}
			if domain != "" {
				body["domain"] = domain
			}
			if protocol == "http" {
				label := domainLabel
				if label == "" {
					label = name
				}
				body["domain_label"] = label
			} else if domainLabel != "" {
				body["domain_label"] = domainLabel
			}
			data, err := r.client.Post("/api/v1/proxies", body)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "entry name (required)")
	cmd.Flags().StringVar(&description, "description", "", "description")
	cmd.Flags().StringVar(&protocol, "protocol", "tcp", "tcp|http|ssh|rdp|mysql|postgresql|redis|mongodb")
	cmd.Flags().IntVar(&port, "port", 0, "public port (0 = auto-allocate; ignored for http)")
	cmd.Flags().StringVar(&domainLabel, "domain-label", "", "subdomain label for http entries (defaults to --name)")
	cmd.Flags().StringVar(&domain, "domain", "", "full custom domain for http entries (optional; requires DNS pointing at liaison.cloud)")
	cmd.Flags().Uint64Var(&applicationID, "application-id", 0, "backend application ID (required)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newProxyUpdateCmd() *cobra.Command {
	var (
		name        string
		description string
		port        int
		status      string
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an entry",
		Long: `Update an entry's metadata, port, or status.

The --status flag accepts running|stopped. Setting status=stopped takes the
entry offline without deleting it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			id, err := strconv.ParseUint(args[0], 10, 64)
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
			if port > 0 {
				body["port"] = port
			}
			if status != "" {
				body["status"] = status // server takes string "running"|"stopped"
			}
			if len(body) == 0 {
				return fmt.Errorf("nothing to update (pass at least one of --name, --description, --port, --status)")
			}
			data, err := r.client.Put(fmt.Sprintf("/api/v1/proxies/%d", id), body)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().IntVar(&port, "port", 0, "new port")
	cmd.Flags().StringVar(&status, "status", "", "running|stopped")
	return cmd
}

func newProxyDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"rm"},
		Short:   "Delete an entry",
		Args:    cobra.ExactArgs(1),
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
				return err
			}
			if _, err := r.client.Delete(fmt.Sprintf("/api/v1/proxies/%d", id)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted proxy %d\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion (required)")
	return cmd
}
