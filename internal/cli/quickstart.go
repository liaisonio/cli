package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/liaisonio/cli/internal/client"
	"github.com/liaisonio/cli/internal/output"
	"github.com/spf13/cobra"
)

// quickstartResult is what the command returns to stdout. Agent-friendly:
// always JSON, always contains every id the caller may need next.
type quickstartResult struct {
	Connector      quickstartConnector    `json:"connector"`
	InstallCommand string                 `json:"install_command"`
	Installed      bool                   `json:"installed"`
	OnlineWaited   bool                   `json:"online_waited"`
	OnlineAchieved bool                   `json:"online_achieved"`
	Application    *quickstartApplication `json:"application,omitempty"`
	Entry          *quickstartEntry       `json:"entry,omitempty"`
	NextSteps      []string               `json:"next_steps,omitempty"`
}

type quickstartConnector struct {
	ID          uint64 `json:"id"`
	Name        string `json:"name"`
	AccessKey   string `json:"access_key"`
	SecretKey   string `json:"secret_key"`
	Description string `json:"description,omitempty"`
}

type quickstartApplication struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
}

type quickstartEntry struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Protocol  string `json:"protocol,omitempty"`
	Port      int    `json:"port,omitempty"`
	Domain    string `json:"domain,omitempty"`
	AccessURL string `json:"access_url,omitempty"`
	ShareURL  string `json:"share_url,omitempty"`
}

func newQuickstartCmd() *cobra.Command {
	var (
		// connector
		name        string
		description string
		install     bool
		waitOnline  time.Duration

		// application
		appName     string
		appIP       string
		appPort     int
		appProtocol string

		// entry
		expose      bool
		entryName   string
		entryPort   int
		entryDomain string
	)

	cmd := &cobra.Command{
		Use:   "quickstart",
		Short: "Create a connector, optionally register a local application, and expose it — all in one call",
		Long: `One-shot bootstrap of a fresh connector and its first application+entry.

Equivalent to running ` + "`edge create`" + `, ` + "`application create`" + ` and ` + "`proxy create`" + ` in
sequence, wrapped so an agent only makes a single tool call and the caller
gets every id/key they'll need next in one structured JSON response.

Examples:

  # Just create the connector and print its install command.
  liaison quickstart --name mybox

  # Create connector + register an application behind it.
  liaison quickstart --name mybox \
    --app-name ssh --app-ip 127.0.0.1 --app-port 22 --app-protocol ssh

  # Create connector + app + expose it publicly, waiting for the
  # connector to come online before creating the entry.
  liaison quickstart --name mybox \
    --app-name web --app-ip 127.0.0.1 --app-port 8080 --app-protocol http \
    --expose --wait-online 2m

  # Run the install script locally (requires sudo). Only safe when the
  # CLI is running on the same machine that will host the connector.
  liaison quickstart --name mybox --install --wait-online 2m

Output (JSON):
  {
    "connector":        { "id": 100042, "name": "mybox", "access_key": "...", "secret_key": "..." },
    "install_command":  "curl ... | bash -s -- --access-key=... ...",
    "installed":        false,
    "online_waited":    true,
    "online_achieved":  true,
    "application":      { "id": 1, "name": "web", "protocol": "http", "ip": "127.0.0.1", "port": 8080 },
    "entry":            { "id": 10, "name": "web", "protocol": "http" },
    "next_steps":       [ "Run install_command on the target host if not done yet." ]
  }`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := current()
			if err != nil {
				return err
			}

			// ─── 1. Create the connector ─────────────────────────────────────
			createBody := map[string]any{}
			if name != "" {
				createBody["name"] = name
			}
			if description != "" {
				createBody["description"] = description
			}
			data, err := r.client.Post("/api/v1/edges", createBody)
			if err != nil {
				return fmt.Errorf("create connector: %w", err)
			}
			var edgeCreate client.EdgeCreateResult
			if err := json.Unmarshal(data, &edgeCreate); err != nil {
				return fmt.Errorf("decode edge create response: %w", err)
			}

			// The create endpoint historically returned only ak/sk/command
			// (no id, because the upstream caller was an installer that
			// didn't need one). Recover the id by listing the newest edge —
			// single-user flows reliably put the just-created one at the top.
			edgeID, edgeName, err := resolveNewestEdge(r.client)
			if err != nil {
				return fmt.Errorf("resolve connector id after create: %w", err)
			}

			result := quickstartResult{
				Connector: quickstartConnector{
					ID:          edgeID,
					Name:        edgeName,
					AccessKey:   edgeCreate.AccessKey,
					SecretKey:   edgeCreate.SecretKey,
					Description: description,
				},
				InstallCommand: edgeCreate.Command,
			}

			// ─── 2. Optionally run the install script locally ────────────────
			if install {
				if err := runInstallScript(edgeCreate.Command, cmd.ErrOrStderr()); err != nil {
					result.NextSteps = append(result.NextSteps,
						fmt.Sprintf("Install attempt failed: %v. Run install_command manually on the target host.", err))
				} else {
					result.Installed = true
				}
			} else {
				result.NextSteps = append(result.NextSteps,
					"Run install_command on the target host (needs curl + bash + sudo).")
			}

			// ─── 3. Optionally wait for the connector to come online ────────
			if waitOnline > 0 {
				result.OnlineWaited = true
				if pollEdgeOnline(r.client, edgeID, waitOnline) {
					result.OnlineAchieved = true
				} else {
					result.NextSteps = append(result.NextSteps,
						fmt.Sprintf("Connector did not come online within %s — check the install succeeded on the host.", waitOnline))
				}
			}

			// ─── 4. Register the application ─────────────────────────────────
			needsApp := appName != "" && appIP != "" && appPort > 0
			if needsApp {
				if appProtocol == "" {
					appProtocol = "tcp"
				}
				appBody := map[string]any{
					"name":             appName,
					"application_type": appProtocol,
					"ip":               appIP,
					"port":             appPort,
					"edge_id":          edgeID,
				}
				appData, err := r.client.Post("/api/v1/applications", appBody)
				if err != nil {
					return fmt.Errorf("create application: %w", err)
				}
				var app client.Application
				if err := json.Unmarshal(appData, &app); err != nil {
					return fmt.Errorf("decode application response: %w", err)
				}
				result.Application = &quickstartApplication{
					ID:       app.ID.Uint64(),
					Name:     app.Name,
					Protocol: app.ApplicationType,
					IP:       app.IP,
					Port:     app.Port,
				}
			}

			// ─── 5. Optionally expose via entry ──────────────────────────────
			if expose {
				if result.Application == nil {
					return fmt.Errorf("--expose requires --app-name, --app-ip and --app-port to create an application first")
				}
				if entryName == "" {
					entryName = appName
				}
				entryBody := map[string]any{
					"name":           entryName,
					"protocol":       appProtocol,
					"application_id": result.Application.ID,
				}
				if entryPort > 0 {
					entryBody["port"] = entryPort
				}
				if entryDomain != "" {
					entryBody["domain"] = entryDomain
				}
				// HTTP entries always require a domain_label; derive from
				// --entry-name. Any custom --entry-domain is layered on top.
				if appProtocol == "http" {
					entryBody["domain_label"] = entryName
				}
				entryData, err := r.client.Post("/api/v1/proxies", entryBody)
				if err != nil {
					return fmt.Errorf("create entry: %w", err)
				}
				var entry client.Proxy
				if err := json.Unmarshal(entryData, &entry); err != nil {
					return fmt.Errorf("decode entry response: %w", err)
				}
				qe := &quickstartEntry{
					ID:       entry.ID.Uint64(),
					Name:     entry.Name,
					Protocol: entry.Protocol,
					Port:     entry.Port,
					Domain:   entry.Domain,
				}

				// Fetch a temporary share link for the newly created entry
				sharePath := fmt.Sprintf("/api/v1/proxies/%d/share_session", entry.ID.Uint64())
				shareData, err := r.client.Post(sharePath, nil)
				if err == nil {
					var shareResp struct {
						ShareURL  string `json:"share_url"`
						AccessURL string `json:"access_url"`
					}
					if json.Unmarshal(shareData, &shareResp) == nil {
						if shareResp.ShareURL != "" {
							qe.ShareURL = shareResp.ShareURL
						}
						if shareResp.AccessURL != "" {
							qe.AccessURL = shareResp.AccessURL
						}
					}
				}
				// Fallback: build access_url from domain if share API didn't return it
				if qe.AccessURL == "" && qe.Domain != "" {
					qe.AccessURL = "https://" + qe.Domain
				}

				result.Entry = qe
			}

			// If user explicitly requested a format (-o json/yaml), output full
			// structured data for agents. Otherwise show a human-friendly summary.
			if cmd.Flags().Changed("output") {
				return output.Print(cmd.OutOrStdout(), r.fmt, result)
			}

			// Human-friendly default output
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "✓ Connector %q created\n", result.Connector.Name)
			if result.Installed {
				fmt.Fprintln(w, "✓ Connector agent installed locally")
			}
			if result.OnlineAchieved {
				fmt.Fprintln(w, "✓ Connector is online")
			}
			if result.Application != nil {
				fmt.Fprintf(w, "✓ Application %q registered (%s %s:%d)\n",
					result.Application.Name, result.Application.Protocol,
					result.Application.IP, result.Application.Port)
			}
			if result.Entry != nil {
				fmt.Fprintln(w)
				if result.Entry.AccessURL != "" {
					fmt.Fprintf(w, "  Access URL:  %s\n", result.Entry.AccessURL)
				} else if result.Entry.Port > 0 && result.Entry.Port != 443 {
					fmt.Fprintf(w, "  Access:      liaison.cloud:%d\n", result.Entry.Port)
				}
				if result.Entry.ShareURL != "" {
					fmt.Fprintf(w, "  Share URL:   %s\n", result.Entry.ShareURL)
				}
				fmt.Fprintln(w)
			}
			if !result.Installed && result.InstallCommand != "" {
				fmt.Fprintln(w, "Next: run this on the target host to bring the connector online:")
				fmt.Fprintf(w, "  %s\n", result.InstallCommand)
			}
			return nil
		},
	}

	// connector flags
	cmd.Flags().StringVar(&name, "name", "", "connector name (auto-generated if empty)")
	cmd.Flags().StringVar(&description, "description", "", "connector description")
	cmd.Flags().BoolVar(&install, "install", false, "run the install script locally (needs sudo + bash + curl)")
	cmd.Flags().DurationVar(&waitOnline, "wait-online", 0, "poll until the connector reports online, e.g. 2m (0 = don't wait)")

	// application flags
	cmd.Flags().StringVar(&appName, "app-name", "", "application name to register behind the connector")
	cmd.Flags().StringVar(&appIP, "app-ip", "", "application backend IP (e.g. 127.0.0.1)")
	cmd.Flags().IntVar(&appPort, "app-port", 0, "application backend port")
	cmd.Flags().StringVar(&appProtocol, "app-protocol", "", "application protocol: tcp|http|ssh|rdp|mysql|postgresql|redis|mongodb (default tcp)")

	// entry flags
	cmd.Flags().BoolVar(&expose, "expose", false, "also create a public entry for the application")
	cmd.Flags().StringVar(&entryName, "entry-name", "", "entry name (defaults to --app-name)")
	cmd.Flags().IntVar(&entryPort, "entry-port", 0, "public port for tcp-like entries (0 = auto-allocate)")
	cmd.Flags().StringVar(&entryDomain, "entry-domain", "", "public domain for http entries")

	return cmd
}

// resolveNewestEdge fetches the top of the edge list (ordered by id desc on
// the server) and returns its id + name. In single-user quickstart flows this
// is the edge we just created.
func resolveNewestEdge(c *client.Client) (uint64, string, error) {
	q := url.Values{}
	q.Set("page", "1")
	q.Set("page_size", "1")
	data, err := c.Get("/api/v1/edges", q)
	if err != nil {
		return 0, "", err
	}
	var list client.EdgeList
	if err := json.Unmarshal(data, &list); err != nil {
		return 0, "", err
	}
	if len(list.Edges) == 0 {
		return 0, "", fmt.Errorf("empty edge list after create")
	}
	return list.Edges[0].ID.Uint64(), list.Edges[0].Name, nil
}

// pollEdgeOnline polls GET /edges/{id} every 3s until online == 1 or the
// deadline passes. Returns true on success, false on timeout.
func pollEdgeOnline(c *client.Client, edgeID uint64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	path := fmt.Sprintf("/api/v1/edges/%d", edgeID)
	for time.Now().Before(deadline) {
		data, err := c.Get(path, nil)
		if err == nil {
			var edge client.Edge
			if err := json.Unmarshal(data, &edge); err == nil && edge.Online == 1 {
				return true
			}
		}
		time.Sleep(3 * time.Second)
	}
	return false
}

// runInstallScript shells out to `bash -c <command>` so the curl|bash
// pipeline returned by the server works as-is. Output is captured silently;
// only shown to the user if the install fails.
func runInstallScript(installCommand string, stderr io.Writer) error {
	cmdline := strings.TrimSpace(installCommand)
	if cmdline == "" {
		return fmt.Errorf("empty install command from server")
	}
	fmt.Fprintln(stderr, "Installing connector agent...")
	c := exec.Command("bash", "-c", cmdline)
	var buf strings.Builder
	c.Stdout = &buf
	c.Stderr = &buf
	if err := c.Run(); err != nil {
		// Install failed — dump the captured output so the user can diagnose
		fmt.Fprintln(stderr, buf.String())
		return err
	}
	return nil
}
