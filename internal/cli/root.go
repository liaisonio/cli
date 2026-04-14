package cli

import (
	"fmt"
	"os"

	"github.com/liaisonio/cli/internal/client"
	"github.com/liaisonio/cli/internal/config"
	"github.com/liaisonio/cli/internal/output"
	"github.com/spf13/cobra"
)

// Global state shared by all commands. Populated in PersistentPreRunE.
type globalFlags struct {
	configPath string
	server     string
	token      string
	output     string
	insecure   bool
	verbose    bool
}

var flags globalFlags

// resolved is populated after flags parse and env vars are merged.
type resolved struct {
	cfg    *config.Config
	client *client.Client
	fmt    output.Format
}

func current() (*resolved, error) {
	persisted, err := config.Load(flags.configPath)
	if err != nil {
		return nil, err
	}
	cfg := config.Resolve(persisted, flags.server, flags.token)
	f, err := output.Parse(flags.output)
	if err != nil {
		return nil, err
	}
	return &resolved{
		cfg:    cfg,
		client: client.New(cfg.Server, cfg.Token, flags.insecure, flags.verbose),
		fmt:    f,
	}, nil
}

// NewRootCmd builds the top-level `liaison` command with all subcommands wired in.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "liaison",
		Short: "Liaison Cloud CLI — manage connectors, entries, and applications from the command line",
		Long: `Liaison Cloud CLI is the official command-line interface for liaison.cloud.
It's designed to be scripted and agent-friendly: every command returns JSON by
default, supports --output table for humans, and reads credentials from either
a config file, environment variables, or explicit flags.

Authentication:
  Set the LIAISON_TOKEN environment variable, or run ` + "`liaison login --token <jwt>`" + ` once to
  persist it to ~/.liaison/config.yaml. The token is a JWT issued by liaison.cloud;
  you can obtain one from the web UI (browser dev tools → cookies/localStorage) or
  via the interactive login flow when supported.

Examples:
  liaison edge list
  liaison edge get 100017
  liaison edge create --name my-connector --description "lab server"
  liaison edge update 100017 --status stopped       # disable & kick
  liaison proxy list --output table
  LIAISON_TOKEN=xxx liaison application list        # one-shot override
`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&flags.configPath, "config", "", "config file path (default ~/.liaison/config.yaml)")
	root.PersistentFlags().StringVar(&flags.server, "server", "", "liaison server base URL (env: LIAISON_SERVER, default: "+config.DefaultServer+")")
	root.PersistentFlags().StringVar(&flags.token, "token", "", "bearer token (env: LIAISON_TOKEN)")
	root.PersistentFlags().StringVarP(&flags.output, "output", "o", "json", "output format: json|yaml|table")
	root.PersistentFlags().BoolVar(&flags.insecure, "insecure", false, "skip TLS certificate verification")
	root.PersistentFlags().BoolVarP(&flags.verbose, "verbose", "v", false, "print request details to stderr")

	root.AddCommand(
		newLoginCmd(),
		newLogoutCmd(),
		newWhoamiCmd(),
		newEdgeCmd(),
		newProxyCmd(),
		newApplicationCmd(),
		newDeviceCmd(),
		newVersionCmd(),
	)
	return root
}

// exitWithErr is a helper used by commands when printing structured output.
func exitWithErr(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
