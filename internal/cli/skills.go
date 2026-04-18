package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	liaisoncli "github.com/liaisonio/cli"
	"github.com/spf13/cobra"
)

// agentTarget mirrors the hard-coded agent map used by the `skills` npm
// package (node_modules/skills/dist/cli.mjs). Keeping the paths in sync
// lets `liaison skills install` and `npx skills add liaisonio/cli -a '*'`
// land in the same places, so either command is a drop-in for the other.
type agentTarget struct {
	id        string   // stable id used by --agent (e.g. "claude", "codex")
	label     string   // human name for output
	globalDir string   // absolute global skills dir
	markers   []string // if any of these dirs exist, the agent is considered installed
}

func knownAgents(home string) []agentTarget {
	return []agentTarget{
		{
			id:        "claude",
			label:     "Claude Code",
			globalDir: filepath.Join(home, ".claude", "skills"),
			markers:   []string{filepath.Join(home, ".claude")},
		},
		{
			id:        "codex",
			label:     "Codex",
			globalDir: filepath.Join(home, ".codex", "skills"),
			markers:   []string{filepath.Join(home, ".codex")},
		},
		{
			id:        "cursor",
			label:     "Cursor",
			globalDir: filepath.Join(home, ".cursor", "skills"),
			markers:   []string{filepath.Join(home, ".cursor")},
		},
		{
			id:        "pi",
			label:     "Pi",
			globalDir: filepath.Join(home, ".pi", "skills"),
			markers:   []string{filepath.Join(home, ".pi")},
		},
		{
			id:        "trae",
			label:     "Trae",
			globalDir: filepath.Join(home, ".trae", "skills"),
			markers:   []string{filepath.Join(home, ".trae")},
		},
		{
			id:        "trae-cn",
			label:     "Trae (CN)",
			globalDir: filepath.Join(home, ".trae-cn", "skills"),
			markers:   []string{filepath.Join(home, ".trae-cn")},
		},
		{
			id:        "openclaw",
			label:     "OpenClaw",
			globalDir: filepath.Join(home, ".openclaw", "skills"),
			markers: []string{
				filepath.Join(home, ".openclaw"),
				filepath.Join(home, ".clawdbot"),
				filepath.Join(home, ".moltbot"),
			},
		},
	}
}

func (a agentTarget) installed() bool {
	for _, m := range a.markers {
		if st, err := os.Stat(m); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Install the agent skill files bundled with this CLI",
		Long: `Offline fallback for installing the Liaison agent skill files (SKILL.md)
that teach AI agents how to drive this CLI.

The recommended path is ` + "`npx skills add liaisonio/cli -y -g`" + `, which AI agents
already recognise. This command is the network-free alternative: the skill
files are embedded in the CLI binary, so installation always matches the CLI
version and works even when GitHub is unreachable.`,
	}
	cmd.AddCommand(
		newSkillsInstallCmd(),
		newSkillsUninstallCmd(),
		newSkillsListCmd(),
		newSkillsAgentsCmd(),
	)
	return cmd
}

func newSkillsUninstallCmd() *cobra.Command {
	var (
		project bool
		dir     string
		agents  string
	)
	cmd := &cobra.Command{
		Use:     "uninstall",
		Aliases: []string{"remove", "rm"},
		Short:   "Remove Liaison agent skills from every agent this CLI installed them into",
		Long: `Delete the liaison-* skill directories from each target.

By default removes from every detected agent — the same set that
` + "`liaison skills install`" + ` writes to. Useful when ` + "`npx skills remove`" + ` doesn't
cover an agent (e.g. Pi) but ` + "`liaison skills install`" + ` did.

Target selection mirrors install: --project / --dir / --agent.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			targets, err := resolveSkillsTargets(project, dir, agents)
			if err != nil {
				return err
			}
			return removeFromTargets(targets, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&project, "project", "p", false, "remove from ./.claude/skills instead")
	cmd.Flags().StringVar(&dir, "dir", "", "custom target directory (overrides --project)")
	cmd.Flags().StringVar(&agents, "agent", "", "comma-separated agent ids or '*' (default: all detected)")
	return cmd
}

func removeFromTargets(targets []installTarget, out io.Writer) error {
	if len(targets) == 0 {
		return fmt.Errorf("no uninstall targets resolved")
	}
	skillNames, err := embeddedSkillNames()
	if err != nil {
		return err
	}
	for i, t := range targets {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "→ %s  (%s)\n", t.label, t.path)
		removed, missing := 0, 0
		for _, name := range skillNames {
			dst := filepath.Join(t.path, name)
			switch _, err := os.Lstat(dst); {
			case err == nil:
				if err := os.RemoveAll(dst); err != nil {
					return fmt.Errorf("remove %s: %w", dst, err)
				}
				fmt.Fprintf(out, "  removed: %s\n", name)
				removed++
			case os.IsNotExist(err):
				fmt.Fprintf(out, "  skip (missing): %s\n", name)
				missing++
			default:
				return fmt.Errorf("stat %s: %w", dst, err)
			}
		}
		fmt.Fprintf(out, "  ✓ %d removed, %d missing\n", removed, missing)
	}
	return nil
}

func embeddedSkillNames() ([]string, error) {
	entries, err := fs.ReadDir(liaisoncli.SkillsFS, "skills")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func newSkillsInstallCmd() *cobra.Command {
	var (
		global  bool
		project bool
		dir     string
		agents  string
		force   bool
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Liaison agent skills offline (multi-agent)",
		Long: `Copy the bundled Liaison skill files into every AI agent's skills
directory, mirroring the coverage of ` + "`npx skills add liaisonio/cli -a '*'`" + `.

Target selection:
  --global / -g          install globally for every detected agent (default)
  --project / -p         install into ./.claude/skills  (per-repo, Claude Code only)
  --dir <path>           install into an arbitrary directory (overrides -g/-p)
  --agent <list>         restrict the global install to specific agents;
                         comma-separated ids or '*' (default: all detected).
                         Known ids: claude, codex, cursor, pi, trae, trae-cn, openclaw.

Re-running is safe: existing files are skipped unless you pass --force.

Examples:
  liaison skills install                         # every detected agent
  liaison skills install -g --agent claude       # Claude Code only
  liaison skills install -g --agent claude,codex # pick two
  liaison skills install -g --agent '*'          # all known agents, even undetected
  liaison skills install -p                      # ./.claude/skills (repo-local)
  liaison skills install --dir /opt/skills       # raw directory
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if global && project {
				return fmt.Errorf("--global and --project are mutually exclusive")
			}
			targets, err := resolveSkillsTargets(project, dir, agents)
			if err != nil {
				return err
			}
			return installToTargets(targets, force, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "install globally for every detected agent (default)")
	cmd.Flags().BoolVarP(&project, "project", "p", false, "install into ./.claude/skills instead")
	cmd.Flags().StringVar(&dir, "dir", "", "custom target directory (overrides -g/-p)")
	cmd.Flags().StringVar(&agents, "agent", "", "comma-separated agent ids or '*' (default: all detected)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing files in the target")
	return cmd
}

func newSkillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the skill names bundled in this CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := fs.ReadDir(liaisoncli.SkillsFS, "skills")
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.IsDir() {
					fmt.Fprintln(cmd.OutOrStdout(), e.Name())
				}
			}
			return nil
		},
	}
}

func newSkillsAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "List known AI agents and whether they're detected on this machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-10s %-14s %-10s %s\n", "ID", "NAME", "DETECTED", "SKILLS DIR")
			for _, a := range knownAgents(home) {
				flag := "no"
				if a.installed() {
					flag = "yes"
				}
				fmt.Fprintf(w, "%-10s %-14s %-10s %s\n", a.id, a.label, flag, a.globalDir)
			}
			return nil
		},
	}
}

// installTarget is one resolved destination — usually a specific agent, but
// may be a bare directory if --dir/--project was used.
type installTarget struct {
	label string // shown in output; agent label for known agents, path for raw dirs
	path  string // absolute directory to write SKILL.md files into
}

func resolveSkillsTargets(project bool, dir, agentList string) ([]installTarget, error) {
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		return []installTarget{{label: abs, path: abs}}, nil
	}
	if project {
		abs, err := filepath.Abs(filepath.Join(".claude", "skills"))
		if err != nil {
			return nil, err
		}
		return []installTarget{{label: "Claude Code (project)", path: abs}}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve home directory: %w", err)
	}
	all := knownAgents(home)

	selector := strings.TrimSpace(agentList)
	var chosen []agentTarget

	switch {
	case selector == "" || selector == "auto":
		// default: every detected agent, with a safety-net fallback to Claude
		// Code so a brand-new machine still gets something.
		for _, a := range all {
			if a.installed() {
				chosen = append(chosen, a)
			}
		}
		if len(chosen) == 0 {
			chosen = append(chosen, all[0]) // claude
		}
	case selector == "*":
		chosen = all
	default:
		requested := map[string]bool{}
		for _, id := range strings.Split(selector, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				requested[id] = true
			}
		}
		known := map[string]agentTarget{}
		for _, a := range all {
			known[a.id] = a
		}
		var unknown []string
		for id := range requested {
			a, ok := known[id]
			if !ok {
				unknown = append(unknown, id)
				continue
			}
			chosen = append(chosen, a)
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			validIDs := make([]string, 0, len(all))
			for _, a := range all {
				validIDs = append(validIDs, a.id)
			}
			return nil, fmt.Errorf("unknown agent(s): %s (valid: %s)",
				strings.Join(unknown, ", "), strings.Join(validIDs, ", "))
		}
	}

	targets := make([]installTarget, 0, len(chosen))
	for _, a := range chosen {
		targets = append(targets, installTarget{label: a.label, path: a.globalDir})
	}
	return targets, nil
}

func installToTargets(targets []installTarget, force bool, out io.Writer) error {
	if len(targets) == 0 {
		return fmt.Errorf("no install targets resolved")
	}
	for i, t := range targets {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "→ %s  (%s)\n", t.label, t.path)
		if err := copyEmbeddedSkills(t.path, force, out); err != nil {
			return fmt.Errorf("install to %s: %w", t.path, err)
		}
	}
	return nil
}

func copyEmbeddedSkills(target string, force bool, out io.Writer) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("create target %s: %w", target, err)
	}
	installed, skipped := 0, 0
	err := fs.WalkDir(liaisoncli.SkillsFS, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "skills" {
			return nil
		}
		rel := strings.TrimPrefix(p, "skills/")
		dst := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if !force {
			if _, err := os.Stat(dst); err == nil {
				fmt.Fprintf(out, "  skip (exists): %s\n", rel)
				skipped++
				return nil
			}
		}
		data, err := fs.ReadFile(liaisoncli.SkillsFS, p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "  installed: %s\n", rel)
		installed++
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  ✓ %d new, %d skipped", installed, skipped)
	if skipped > 0 && !force {
		fmt.Fprint(out, " (re-run with --force to overwrite)")
	}
	fmt.Fprintln(out)
	return nil
}
