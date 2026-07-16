package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/scaffold"
)

var serviceCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"services"},
	Short:   "Manage platform services",
}

var serviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List platform services, their providers, and profile membership",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Mark enabled services when run inside a scaffolded repo.
		enabled := map[string]bool{}
		if cfg, err := config.Load("."); err == nil {
			for _, s := range cfg.Services {
				enabled[s] = true
			}
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tENABLED\tPROVIDERS\tPROFILES\tDESCRIPTION")
		for _, s := range scaffold.Registry {
			providers := "all"
			if len(s.Providers) > 0 {
				providers = strings.Join(s.Providers, ",")
			}
			var profiles []string
			for _, p := range scaffold.ProfileNames() {
				for _, n := range scaffold.Profiles[p] {
					if n == s.Name {
						profiles = append(profiles, p)
						break
					}
				}
			}
			mark := ""
			if enabled[s.Name] {
				mark = "yes"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Name, mark, providers, strings.Join(profiles, ","), s.Desc)
		}
		return w.Flush()
	},
}

var serviceOpFlags struct {
	path string
}

var serviceAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Enable a platform service (and its requirements) on this repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(serviceOpFlags.path)
		if err != nil {
			return err
		}
		flux := detectFlux()
		res := &scaffold.Result{}
		added, err := scaffold.AddService(res, cfg, serviceOpFlags.path, args[0], flux)
		if err != nil {
			return err
		}
		if err := cfg.Save(serviceOpFlags.path); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Enabled: %s (%d files touched)\n", strings.Join(added, ", "), len(res.Written))
		fmt.Fprintln(cmd.OutOrStdout(), "Review the new manifests under infra/services/, then commit and push.")
		return nil
	},
}

var serviceRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Disable a platform service and delete its manifests",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(serviceOpFlags.path)
		if err != nil {
			return err
		}
		flux := detectFlux()
		res := &scaffold.Result{}
		if err := scaffold.RemoveService(res, cfg, serviceOpFlags.path, args[0], flux); err != nil {
			return err
		}
		if err := cfg.Save(serviceOpFlags.path); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Disabled %q (removed %s)\n", args[0], strings.Join(res.Removed, ", "))
		fmt.Fprintln(cmd.OutOrStdout(), "Commit and push; Flux prunes the service from the cluster.")
		return nil
	},
}

func init() {
	serviceAddCmd.Flags().StringVar(&serviceOpFlags.path, "path", ".", "repo root (containing fluxboost.yaml)")
	serviceRemoveCmd.Flags().StringVar(&serviceOpFlags.path, "path", ".", "repo root (containing fluxboost.yaml)")
	serviceCmd.AddCommand(serviceListCmd, serviceAddCmd, serviceRemoveCmd)
	rootCmd.AddCommand(serviceCmd)
}
