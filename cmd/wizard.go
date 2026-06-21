package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/everythingisacomputer/fluxboost/internal/scaffold"
)

// errAborted signals the user backed out of the wizard; nothing was written.
var errAborted = errors.New("aborted")

// wizardAnswers is everything init needs to scaffold, however it was collected.
type wizardAnswers struct {
	Domain      string
	Email       string
	Provider    string
	Profile     string
	GitURL      string
	Envs        []string
	Vars        map[string]string
	Tenants     []string
	SopsEnabled bool
}

func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func validateDomain(s string) error {
	if s == "" || !strings.Contains(s, ".") || strings.ContainsAny(s, " /") {
		return fmt.Errorf("enter a DNS domain like example.com")
	}
	return nil
}

func validateEmail(s string) error {
	if !strings.Contains(s, "@") {
		return fmt.Errorf("enter an email address")
	}
	return nil
}

func validateRFC1123(kind string) func(string) error {
	return func(s string) error {
		if !nameRe.MatchString(s) {
			return fmt.Errorf("invalid %s name %q (lowercase alphanumerics and dashes)", kind, s)
		}
		return nil
	}
}

func parseEnvs(raw string) ([]string, error) {
	var envs []string
	for _, e := range strings.Split(raw, ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !nameRe.MatchString(e) {
			return nil, fmt.Errorf("invalid environment name %q (lowercase alphanumerics and dashes)", e)
		}
		envs = append(envs, e)
	}
	if len(envs) == 0 {
		return nil, fmt.Errorf("at least one environment is required")
	}
	return envs, nil
}

// newForm builds a huh form, pinning a fixed width when the terminal cannot
// report its size (zero-size PTYs crash bubbletea's renderer otherwise).
func newForm(groups ...*huh.Group) *huh.Form {
	f := huh.NewForm(groups...)
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err != nil || w <= 0 {
		f = f.WithWidth(80)
	}
	return f
}

// runPrettyWizard collects answers with huh forms (bubbletea under the hood).
func runPrettyWizard(a *wizardAnswers) error {
	if err := newForm(huh.NewGroup(
		huh.NewInput().
			Title("Base domain").
			Description("Apex DNS zone the platform will serve, e.g. example.com").
			Placeholder("example.com").
			Value(&a.Domain).
			Validate(validateDomain),
		huh.NewInput().
			Title("Domain manager email").
			Description("Used for Let's Encrypt registration").
			Value(&a.Email).
			Validate(validateEmail),
		huh.NewSelect[string]().
			Title("Cloud platform").
			Options(
				huh.NewOption("GCP — GKE, Cloud DNS, Workload Identity", "gcp"),
				huh.NewOption("AWS — EKS, Route53, IRSA", "aws"),
				huh.NewOption("Bare metal", "baremetal"),
			).
			Value(&a.Provider),
	)).Run(); err != nil {
		return err
	}

	switch a.Provider {
	case "gcp":
		var project string
		if err := newForm(huh.NewGroup(
			huh.NewInput().
				Title("GCP project id").
				Description("Project owning the DNS zone; used for Workload Identity annotations").
				Value(&project).
				Validate(huh.ValidateNotEmpty()),
		)).Run(); err != nil {
			return err
		}
		a.Vars["gcloudProjectId"] = project
	case "aws":
		region, account := "us-east-1", ""
		if err := newForm(huh.NewGroup(
			huh.NewInput().
				Title("AWS region").
				Value(&region).
				Validate(huh.ValidateNotEmpty()),
			huh.NewInput().
				Title("AWS account id").
				Description("Used to render IRSA role ARNs for cert-manager and external-dns").
				Value(&account).
				Validate(huh.ValidateNotEmpty()),
		)).Run(); err != nil {
			return err
		}
		a.Vars["awsRegion"] = region
		a.Vars["awsAccountId"] = account
	}

	envsRaw := "dev"
	a.Profile = wizardProfiles[0]
	profileOpts := make([]huh.Option[string], len(wizardProfiles))
	for i, p := range wizardProfiles {
		profileOpts[i] = huh.NewOption(p, p)
	}
	if err := newForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Profile").
			Description("Bundle of platform services to install").
			Options(profileOpts...).
			Value(&a.Profile),
		huh.NewInput().
			Title("Environments").
			Description("Comma-separated cluster environments").
			Value(&envsRaw).
			Validate(func(s string) error { _, err := parseEnvs(s); return err }),
		huh.NewInput().
			Title("Git remote URL").
			Description("Optional; used for flux bootstrap hints").
			Value(&a.GitURL),
	)).Run(); err != nil {
		return err
	}
	a.Envs, _ = parseEnvs(envsRaw)

	a.SopsEnabled = true
	var wantTenants bool
	if err := newForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Enable SOPS secrets encryption?").
			Description("Generates an age key (kept out of git) and wires Flux decryption").
			Value(&a.SopsEnabled),
		huh.NewConfirm().
			Title("Set up tenants now?").
			Description("Each tenant gets a namespace and a reconciler scoped to it").
			Value(&wantTenants),
	)).Run(); err != nil {
		return err
	}
	for wantTenants {
		var name string
		if err := newForm(huh.NewGroup(
			huh.NewInput().
				Title("Tenant name").
				Description("Leave empty to finish").
				Value(&name).
				Validate(func(s string) error {
					if s == "" {
						return nil
					}
					for _, t := range a.Tenants {
						if t == s {
							return fmt.Errorf("%q already added", s)
						}
					}
					return validateRFC1123("tenant")(s)
				}),
		)).Run(); err != nil {
			return err
		}
		if name == "" {
			break
		}
		a.Tenants = append(a.Tenants, name)
	}

	// Recap before touching the filesystem.
	confirmed := true
	if err := newForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Scaffold repository?").
			Description(a.summary()).
			Affirmative("Scaffold").
			Negative("Abort").
			Value(&confirmed),
	)).Run(); err != nil {
		return err
	}
	if !confirmed {
		return errAborted
	}
	return nil
}

func (a *wizardAnswers) summary() string {
	sops := "disabled"
	if a.SopsEnabled {
		sops = "enabled (age)"
	}
	lines := []string{
		"domain: " + a.Domain,
		"platform: " + a.Provider,
		"profile: " + a.Profile,
		"environments: " + strings.Join(a.Envs, ", "),
		"secrets: " + sops,
	}
	for k, v := range a.Vars {
		lines = append(lines, k+": "+v)
	}
	if len(a.Tenants) > 0 {
		lines = append(lines, "tenants: "+strings.Join(a.Tenants, ", "))
	}
	return strings.Join(lines, "\n")
}

// runPlainWizard is the non-TTY fallback: line-based prompts that behave the
// same when answers are piped in (scripting, tests).
func runPlainWizard(cmd *cobra.Command, a *wizardAnswers) error {
	p := newPrompter(cmd.InOrStdin(), cmd.OutOrStdout())

	var err error
	if a.Domain, err = p.ask("Base domain (e.g. example.com)", "", true); err != nil {
		return err
	}
	if a.Email, err = p.ask("Domain manager email (Let's Encrypt registration)", "", true); err != nil {
		return err
	}
	if a.Provider, err = p.askChoice("Cloud platform", scaffold.Providers, ""); err != nil {
		return err
	}
	switch a.Provider {
	case "gcp":
		project, err := p.ask("GCP project id (DNS + Workload Identity)", "", true)
		if err != nil {
			return err
		}
		a.Vars["gcloudProjectId"] = project
	case "aws":
		region, err := p.ask("AWS region", "us-east-1", true)
		if err != nil {
			return err
		}
		account, err := p.ask("AWS account id (for IRSA role ARNs)", "", true)
		if err != nil {
			return err
		}
		a.Vars["awsRegion"] = region
		a.Vars["awsAccountId"] = account
	}
	if a.Profile, err = p.askChoice("Profile", wizardProfiles, "standard"); err != nil {
		return err
	}
	envsRaw, err := p.ask("Environments (comma-separated)", "dev", true)
	if err != nil {
		return err
	}
	if a.Envs, err = parseEnvs(envsRaw); err != nil {
		return err
	}
	if a.GitURL, err = p.ask("Git remote URL (optional, used for flux bootstrap hints)", "", false); err != nil {
		return err
	}
	if a.SopsEnabled, err = p.askYesNo("Enable SOPS secrets encryption (age)?", true); err != nil {
		return err
	}
	wantTenants, err := p.askYesNo("Set up tenants now?", false)
	if err != nil {
		return err
	}
	if wantTenants {
		if a.Tenants, err = p.askList("  Tenant name", validateRFC1123("tenant")); err != nil {
			return err
		}
	}
	return nil
}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "28", Dark: "42"})
)
