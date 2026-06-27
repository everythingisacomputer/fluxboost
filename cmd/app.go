package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/scaffold"
)

var appCmd = &cobra.Command{
	Use:   "app",
	Short: "Manage platform applications",
}

var appAddFlags struct {
	path            string
	envs            []string
	appType         string
	image           string
	port            int
	host            string
	schedule        string
	imageAutomation bool
}

var appAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an application and register it with Flux",
	Long: `Scaffolds apps/services/<name> and registers it in each environment.

Types:
  deployment  Deployment + Service + ServiceAccount (+ VirtualService with istio)
  cronjob     CronJob + ServiceAccount

With --image-automation, Flux image automation objects (ImageRepository,
ImagePolicy, ImageUpdateAutomation) are registered in the primary environment
and the workload image carries the update marker, so new semver tags are
committed back to this repo automatically.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if !nameRe.MatchString(name) {
			return fmt.Errorf("invalid app name %q (lowercase alphanumerics and dashes)", name)
		}
		if appAddFlags.appType != "deployment" && appAddFlags.appType != "cronjob" {
			return fmt.Errorf("--type must be deployment or cronjob")
		}
		cfg, err := config.Load(appAddFlags.path)
		if err != nil {
			return err
		}
		if _, exists := cfg.App(name); exists {
			return fmt.Errorf("app %q is already registered in fluxboost.yaml", name)
		}
		app := config.App{
			Name:            name,
			Type:            appAddFlags.appType,
			Image:           appAddFlags.image,
			Port:            appAddFlags.port,
			Host:            appAddFlags.host,
			Schedule:        appAddFlags.schedule,
			ImageAutomation: appAddFlags.imageAutomation,
		}

		envs := appAddFlags.envs
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
			if err := scaffold.AddApp(res, appAddFlags.path, app, ctx); err != nil {
				return err
			}
		}
		cfg.Apps = append(cfg.Apps, app)
		if err := cfg.Save(appAddFlags.path); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "App %q (%s) added for %v (%d files touched)\n", name, app.Type, envs, len(res.Written))
		if app.ImageAutomation {
			fmt.Fprintf(cmd.OutOrStdout(), "Image automation registered in %q; it commits tag updates to branch main.\n", cfg.Environments[0])
		}
		return nil
	},
}

func init() {
	fl := appAddCmd.Flags()
	fl.StringVar(&appAddFlags.path, "path", ".", "repo root (containing fluxboost.yaml)")
	fl.StringArrayVar(&appAddFlags.envs, "env", nil, "environment to register the app in (repeatable; default: all)")
	fl.StringVar(&appAddFlags.appType, "type", "deployment", "workload type: deployment | cronjob")
	fl.StringVar(&appAddFlags.image, "image", "nginx:1.27", "container image")
	fl.IntVar(&appAddFlags.port, "port", 80, "container port (deployment)")
	fl.StringVar(&appAddFlags.host, "host", "", "VirtualService host (deployment; default <name>.<env>.<baseDomain>)")
	fl.StringVar(&appAddFlags.schedule, "schedule", "0 * * * *", "cron schedule (cronjob)")
	fl.BoolVar(&appAddFlags.imageAutomation, "image-automation", false, "enable Flux image automation for this app")
	appCmd.AddCommand(appAddCmd)
	rootCmd.AddCommand(appCmd)
}
