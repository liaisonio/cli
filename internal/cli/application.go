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

func newApplicationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "application",
		Aliases: []string{"app", "applications", "apps"},
		Short:   "Manage backend applications",
		Long:    "Applications are the backend services (IP:port) behind a connector that entries can expose.",
	}
	cmd.AddCommand(
		newAppListCmd(),
		newAppGetCmd(),
		newAppCreateCmd(),
		newAppUpdateCmd(),
		newAppDeleteCmd(),
	)
	return cmd
}

func newAppListCmd() *cobra.Command {
	var (
		page     int
		pageSize int
		name     string
		edgeID   uint64
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List applications",
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
			if edgeID > 0 {
				q.Set("edge_id", strconv.FormatUint(edgeID, 10))
			}
			data, err := r.client.Get("/api/v1/applications", q)
			if err != nil {
				return err
			}
			if r.fmt == output.FormatTable {
				var list client.ApplicationList
				if err := json.Unmarshal(data, &list); err != nil {
					return err
				}
				rows := make([][]string, 0, len(list.Applications))
				for _, a := range list.Applications {
					rows = append(rows, []string{
						strconv.FormatUint(a.ID.Uint64(), 10),
						a.Name,
						a.ApplicationType,
						a.IP,
						strconv.Itoa(a.Port),
						strconv.FormatUint(a.EdgeID.Uint64(), 10),
					})
				}
				return output.PrintTable(cmd.OutOrStdout(),
					[]string{"ID", "NAME", "PROTOCOL", "IP", "PORT", "EDGE_ID"},
					rows)
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "results per page")
	cmd.Flags().StringVar(&name, "name", "", "filter by name (substring)")
	cmd.Flags().Uint64Var(&edgeID, "edge-id", 0, "filter by connector ID")
	return cmd
}

func newAppGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get an application by ID",
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
			data, err := r.client.Get(fmt.Sprintf("/api/v1/applications/%d", id), nil)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
}

func newAppCreateCmd() *cobra.Command {
	var (
		name        string
		description string
		protocol    string
		ip          string
		port        int
		edgeID      uint64
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new application (IP:port behind a connector)",
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}
			body := map[string]any{
				"name":             name,
				"description":     description,
				"application_type": protocol,
				"ip":              ip,
				"port":            port,
				"edge_id":         edgeID,
			}
			data, err := r.client.Post("/api/v1/applications", body)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "application name (required)")
	cmd.Flags().StringVar(&description, "description", "", "description")
	cmd.Flags().StringVar(&protocol, "protocol", "tcp", "tcp|http|ssh|rdp|mysql|postgresql|redis|mongodb")
	cmd.Flags().StringVar(&ip, "ip", "", "backend IP address (required)")
	cmd.Flags().IntVar(&port, "port", 0, "backend port (required)")
	cmd.Flags().Uint64Var(&edgeID, "edge-id", 0, "owning connector ID (required)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("ip")
	_ = cmd.MarkFlagRequired("port")
	_ = cmd.MarkFlagRequired("edge-id")
	return cmd
}

func newAppUpdateCmd() *cobra.Command {
	var (
		name        string
		description string
		ip          string
		port        int
	)
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update an application",
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
			body := map[string]any{}
			if name != "" {
				body["name"] = name
			}
			if description != "" {
				body["description"] = description
			}
			if ip != "" {
				body["ip"] = ip
			}
			if port > 0 {
				body["port"] = port
			}
			if len(body) == 0 {
				return fmt.Errorf("nothing to update")
			}
			data, err := r.client.Put(fmt.Sprintf("/api/v1/applications/%d", id), body)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().StringVar(&ip, "ip", "", "new backend IP")
	cmd.Flags().IntVar(&port, "port", 0, "new backend port")
	return cmd
}

func newAppDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"rm"},
		Short:   "Delete an application",
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
			if _, err := r.client.Delete(fmt.Sprintf("/api/v1/applications/%d", id)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted application %d\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion (required)")
	return cmd
}
