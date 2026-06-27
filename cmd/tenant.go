package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/scaffold"
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Manage Flux tenants",
}

var tenantAddFlags struct {
	path     string
	envs     []string
	repo     string
	branch   string
	repoPath string
}

var tenantAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a tenant: namespace, scoped reconciler, and app tree",
	Long: `Scaffolds tenants/<name> with a namespace, a ServiceAccount that is
admin only within it, and a tenant-owned Kustomization, then registers the
tenant for each environment.

By default the tenant reconciles tenants/<name>/apps/<env> from the platform
repo. With --repo, the tenant instead reconciles from its own git repository:
a GitRepository source is created in the tenant namespace and the tenant
Kustomization points at it.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if !nameRe.MatchString(name) {
			return fmt.Errorf("invalid tenant name %q (lowercase alphanumerics and dashes)", name)
		}
		cfg, err := config.Load(tenantAddFlags.path)
		if err != nil {
			return err
		}
		if _, exists := cfg.Tenant(name); exists {
			return fmt.Errorf("tenant %q is already registered in fluxboost.yaml", name)
		}
		if tenantAddFlags.repo == "" && (tenantAddFlags.branch != "" || tenantAddFlags.repoPath != "") {
			return fmt.Errorf("--branch and --repo-path require --repo")
		}
		tenant := config.Tenant{
			Name:   name,
			Repo:   tenantAddFlags.repo,
			Branch: tenantAddFlags.branch,
			Path:   tenantAddFlags.repoPath,
		}

		envs := tenantAddFlags.envs
		if len(envs) == 0 {
			envs = cfg.Environments
		}
		flux := detectFlux()
		res := &scaffold.Result{}
		for _, env := range envs {
			if !cfg.HasEnv(env) {
				return fmt.Errorf("environment %q is not in fluxboost.yaml (have: %v)", env, cfg.Environments)
			}
			ctx := scaffold.CtxFromConfig(cfg, env, flux)
			if err := scaffold.AddTenant(res, tenantAddFlags.path, tenant, ctx); err != nil {
				return err
			}
		}
		cfg.Tenants = append(cfg.Tenants, tenant)
		if err := cfg.Save(tenantAddFlags.path); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Tenant %q added for %v (%d files touched)\n", name, envs, len(res.Written))
		if tenant.Repo != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "The tenant reconciles from %s (branch %s). For a private repo, create a\ngit credentials Secret in the %s namespace and reference it from the GitRepository.\n",
				tenant.Repo, orDefault(tenant.Branch, "main"), name)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Commit and push; Flux will create the namespace, RBAC, and tenant reconciler.")
		return nil
	},
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func init() {
	fl := tenantAddCmd.Flags()
	fl.StringVar(&tenantAddFlags.path, "path", ".", "repo root (containing fluxboost.yaml)")
	fl.StringArrayVar(&tenantAddFlags.envs, "env", nil, "environment to register the tenant in (repeatable; default: all)")
	fl.StringVar(&tenantAddFlags.repo, "repo", "", "tenant-owned git repository URL (tenant reconciles from it instead of the platform repo)")
	fl.StringVar(&tenantAddFlags.branch, "branch", "", "branch of the tenant repo (default main)")
	fl.StringVar(&tenantAddFlags.repoPath, "repo-path", "", "path within the tenant repo to reconcile (default ./)")
	tenantCmd.AddCommand(tenantAddCmd)
	rootCmd.AddCommand(tenantCmd)
}
