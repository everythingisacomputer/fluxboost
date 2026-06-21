package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"filippo.io/age"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/scaffold"
)

var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// The wizard offers a single profile for now; the registry knows more, but
// they stay off the menu until they are ready to be supported.
var wizardProfiles = []string{"standard"}

// checkEmptyDir refuses to scaffold into a directory that already has
// content. A lone .git directory is allowed so a freshly cloned or freshly
// `git init`ed repo still counts as empty.
func checkEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		return fmt.Errorf("directory %s is not empty (found %q) — fluxboost init only runs in an empty directory", dir, e.Name())
	}
	return nil
}

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Interactively scaffold a Flux GitOps repository",
	Long: `Runs an interactive wizard in the given directory (default: the current
one), creating it first if needed. The directory must be empty (a lone .git
is fine). The wizard asks for your domain, cloud platform and its
credentials, a service profile, and optionally a list of tenants, then
scaffolds the full repository.

In a terminal the wizard is a styled TUI; when input is piped it falls back
to plain line prompts so the same questions can be answered from a script.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		if len(args) == 1 {
			if dir, err = filepath.Abs(args[0]); err != nil {
				return err
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		if err := checkEmptyDir(dir); err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		a := &wizardAnswers{Vars: map[string]string{}}
		if isInteractive() {
			fmt.Fprintln(out, titleStyle.Render("fluxboost"), "— scaffolding a Flux GitOps repository in", dir)
			err = runPrettyWizard(a)
		} else {
			fmt.Fprintln(out, "fluxboost — scaffolding a Flux GitOps repository in", dir)
			fmt.Fprintln(out)
			err = runPlainWizard(cmd, a)
		}
		if errors.Is(err, errAborted) || errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(out, "Aborted — nothing written.")
			return nil
		}
		if err != nil {
			return err
		}

		services, warnings, err := scaffold.ResolveServices(a.Profile, a.Provider, nil, nil)
		if err != nil {
			return err
		}
		for _, w := range warnings {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
		}

		var sopsCfg *config.Sops
		var ageKey string
		if a.SopsEnabled {
			id, err := age.GenerateX25519Identity()
			if err != nil {
				return fmt.Errorf("generating age key: %w", err)
			}
			sopsCfg = &config.Sops{AgeRecipient: id.Recipient().String()}
			ageKey = fmt.Sprintf("# public key: %s\n%s\n", id.Recipient(), id)
		}

		flux := detectFlux()
		res, err := scaffold.Init(scaffold.Options{
			RepoPath:   dir,
			Provider:   a.Provider,
			Envs:       a.Envs,
			Services:   services,
			BaseDomain: a.Domain,
			Email:      a.Email,
			GitURL:     a.GitURL,
			Vars:       a.Vars,
			Sops:       sopsCfg,
			Flux:       flux,
		})
		if err != nil {
			return err
		}
		if a.SopsEnabled {
			keyPath := filepath.Join(dir, "age.agekey")
			if err := os.WriteFile(keyPath, []byte(ageKey), 0o600); err != nil {
				return err
			}
			res.Written = append(res.Written, keyPath)
		}

		if len(a.Tenants) > 0 {
			cfg, err := config.Load(dir)
			if err != nil {
				return err
			}
			for _, env := range a.Envs {
				ctx := scaffold.CtxFromConfig(cfg, env, flux)
				for _, tenant := range a.Tenants {
					if err := scaffold.AddTenant(res, dir, config.Tenant{Name: tenant}, ctx); err != nil {
						return err
					}
				}
			}
			for _, tenant := range a.Tenants {
				cfg.Tenants = append(cfg.Tenants, config.Tenant{Name: tenant})
			}
			if err := cfg.Save(dir); err != nil {
				return err
			}
		}

		fmt.Fprintln(out)
		fmt.Fprintln(out, successStyle.Render(fmt.Sprintf("Scaffolded %d files", len(res.Written))),
			fmt.Sprintf("(provider: %s, profile: %s, flux: %s)", a.Provider, a.Profile, flux))
		fmt.Fprintf(out, "Services: %s\n", strings.Join(services, ", "))
		if len(a.Tenants) > 0 {
			fmt.Fprintf(out, "Tenants: %s\n", strings.Join(a.Tenants, ", "))
		}
		fmt.Fprintln(out, "\nNext steps:")
		if cwd, err := os.Getwd(); err == nil && cwd != dir {
			fmt.Fprintf(out, "  0. cd %s\n", dir)
		}
		fmt.Fprintln(out, "  1. git init && git add -A && git commit, then push to your git remote")
		for _, env := range a.Envs {
			fmt.Fprintf(out, "  2. %s\n", scaffold.BootstrapCommand(a.GitURL, env))
		}
		if a.SopsEnabled {
			fmt.Fprintln(out, "  3. back up age.agekey somewhere safe (it is gitignored), then per cluster:")
			fmt.Fprintln(out, "     kubectl -n flux-system create secret generic sops-age --from-file=age.agekey=age.agekey")
		}
		fmt.Fprintln(out, "  4. fluxboost tenant add <team>   # add more tenants later")
		fmt.Fprintln(out, "  5. fluxboost app add <name> --image <image>   # register workloads")
		fmt.Fprintln(out, "  6. fluxboost check   # validate the repo any time")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
