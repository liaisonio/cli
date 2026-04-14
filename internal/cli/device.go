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

func newDeviceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "device",
		Aliases: []string{"devices"},
		Short:   "Manage devices (hosts that run a connector)",
	}
	cmd.AddCommand(
		newDeviceListCmd(),
		newDeviceGetCmd(),
		newDeviceDeleteCmd(),
	)
	return cmd
}

func newDeviceListCmd() *cobra.Command {
	var (
		page     int
		pageSize int
		name     string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List devices",
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
			data, err := r.client.Get("/api/v1/devices", q)
			if err != nil {
				return err
			}
			if r.fmt == output.FormatTable {
				var list client.DeviceList
				if err := json.Unmarshal(data, &list); err != nil {
					return err
				}
				rows := make([][]string, 0, len(list.Devices))
				for _, d := range list.Devices {
					rows = append(rows, []string{
						strconv.FormatUint(d.ID, 10),
						d.Name,
						d.OS,
						d.Arch,
						strconv.Itoa(d.Online),
					})
				}
				return output.PrintTable(cmd.OutOrStdout(),
					[]string{"ID", "NAME", "OS", "ARCH", "ONLINE"},
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

func newDeviceGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a device by ID",
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
			data, err := r.client.Get(fmt.Sprintf("/api/v1/devices/%d", id), nil)
			if err != nil {
				return err
			}
			return output.Print(cmd.OutOrStdout(), r.fmt, data)
		},
	}
}

func newDeviceDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"rm"},
		Short:   "Delete a device",
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
			if _, err := r.client.Delete(fmt.Sprintf("/api/v1/devices/%d", id)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted device %d\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion (required)")
	return cmd
}
