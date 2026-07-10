package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/scaffold"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage environments",
}

var envAddFlags struct {
	path string
}

var envAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an environment: cluster entry point, overlays, tenants, and apps",
	Long: `Scaffolds clusters/<env> and infra/overlays/<env> for a new environment
and re-registers every recorded tenant and app in it, so the new cluster
converges to the same shape as the existing ones.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		env := args[0]
		if !nameRe.MatchString(env) {
			return fmt.Errorf("invalid environment name %q (lowercase alphanumerics and dashes)", env)
		}
		cfg, err := config.Load(envAddFlags.path)
		if err != nil {
			return err
		}
		flux := detectFlux()
		res := &scaffold.Result{}
		if err := scaffold.AddEnv(res, cfg, envAddFlags.path, env, flux); err != nil {
			return err
		}
		cfg.Environments = append(cfg.Environments, env)
		if err := cfg.Save(envAddFlags.path); err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Environment %q added (%d files touched", env, len(res.Written))
		if len(cfg.Tenants) > 0 || len(cfg.Apps) > 0 {
			fmt.Fprintf(out, "; re-registered %d tenants, %d apps", len(cfg.Tenants), len(cfg.Apps))
		}
		fmt.Fprintln(out, ")")
		fmt.Fprintln(out, "\nNext steps:")
		fmt.Fprintln(out, "  1. commit and push")
		fmt.Fprintf(out, "  2. %s\n", scaffold.BootstrapCommand(cfg.GitURL, env))
		return nil
	},
}

func init() {
	envAddCmd.Flags().StringVar(&envAddFlags.path, "path", ".", "repo root (containing fluxboost.yaml)")
	envCmd.AddCommand(envAddCmd)
	rootCmd.AddCommand(envCmd)
}
