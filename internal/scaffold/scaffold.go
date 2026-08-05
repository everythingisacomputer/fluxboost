// Package scaffold renders the GitOps repository layout: cluster entry
// points under clusters/<env>, platform services under infra/, and the
// tenant/app trees managed by later commands.
package scaffold

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/fluxver"
)

//go:embed templates
var templatesFS embed.FS

// Var is a postBuild substitution variable. Cluster-level Kustomizations
// set concrete values; service-level ones pass ${name} through.
type Var struct {
	Name  string
	Value string
}

// Ctx carries everything templates need.
type Ctx struct {
	Env        string
	Provider   string
	BaseDomain string
	Email      string
	GitURL     string
	Sops       bool
	PrimaryEnv bool // true when Env is the first configured environment
	Vars       []Var
	APIs       fluxver.APIVersions
	Selected   map[string]bool
}

// SyncEntry is one Flux Kustomization block in an overlay sync.yaml.
type SyncEntry struct {
	Name            string
	Path            string
	TargetNamespace string
	DependsOn       []string
}

// Options for Init.
type Options struct {
	RepoPath   string
	Provider   string
	Envs       []string
	Services   []string // resolved, registry order
	BaseDomain string
	Email      string
	GitURL     string
	Vars       map[string]string // provider-specific substitution vars
	Sops       *config.Sops
	Flux       fluxver.Version
	Force      bool
}

// OptionsFromConfig rebuilds Options from a saved fluxboost.yaml, for
// commands that extend an existing repo.
func OptionsFromConfig(c *config.Config, repoPath string, flux fluxver.Version) Options {
	return Options{
		RepoPath:   repoPath,
		Provider:   c.Provider,
		Envs:       c.Environments,
		Services:   c.Services,
		BaseDomain: c.BaseDomain,
		Email:      c.DomainManagerEmail,
		GitURL:     c.GitURL,
		Vars:       c.Vars,
		Sops:       c.Sops,
		Flux:       flux,
	}
}

// Result reports what a scaffolding operation did.
type Result struct {
	Written []string
	Removed []string
}

func render(tmplPath string, ctx *Ctx, extra map[string]any) ([]byte, error) {
	b, err := templatesFS.ReadFile("templates/" + tmplPath)
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", tmplPath, err)
	}
	funcs := template.FuncMap{
		"has": func(name string) bool { return ctx.Selected[name] },
		"ref": func(name string) string { return "${" + name + "}" },
	}
	t, err := template.New(filepath.Base(tmplPath)).Funcs(funcs).Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", tmplPath, err)
	}
	data := map[string]any{"C": ctx}
	for k, v := range extra {
		data[k] = v
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering template %s: %w", tmplPath, err)
	}
	return buf.Bytes(), nil
}

func writeFile(res *Result, path string, content []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return err
	}
	res.Written = append(res.Written, path)
	return nil
}

func writeRendered(res *Result, path, tmplPath string, ctx *Ctx, extra map[string]any, force bool) error {
	b, err := render(tmplPath, ctx, extra)
	if err != nil {
		return err
	}
	return writeFile(res, path, b, force)
}

// writeRenderedIfMissing skips silently when the file already exists — used
// for files shared across environments (e.g. a tenant's _base) so
// registering the same thing for a second env is idempotent.
func writeRenderedIfMissing(res *Result, path, tmplPath string, ctx *Ctx, extra map[string]any) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return writeRendered(res, path, tmplPath, ctx, extra, false)
}

// buildVars returns the substitution variable list: common vars first, then
// provider-specific ones in sorted order.
func buildVars(env, baseDomain, email string, extra map[string]string) []Var {
	vars := []Var{
		{Name: "env", Value: env},
		{Name: "baseDomain", Value: baseDomain},
		{Name: "domainManagerEmail", Value: email},
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vars = append(vars, Var{Name: k, Value: extra[k]})
	}
	return vars
}

func ctxFor(env string, o Options) *Ctx {
	selected := map[string]bool{}
	for _, s := range o.Services {
		selected[s] = true
	}
	return &Ctx{
		Env:        env,
		Provider:   o.Provider,
		BaseDomain: o.BaseDomain,
		Email:      o.Email,
		GitURL:     o.GitURL,
		Sops:       o.Sops != nil,
		PrimaryEnv: len(o.Envs) > 0 && env == o.Envs[0],
		Vars:       buildVars(env, o.BaseDomain, o.Email, o.Vars),
		APIs:       o.Flux.APIs(),
		Selected:   selected,
	}
}

// CtxFromConfig rebuilds a render context from a saved fluxboost.yaml, for
// commands that run after init.
func CtxFromConfig(c *config.Config, env string, flux fluxver.Version) *Ctx {
	return ctxFor(env, OptionsFromConfig(c, "", flux))
}

func syncEntries(ctx *Ctx, services []string) []SyncEntry {
	var entries []SyncEntry
	for _, name := range services {
		s, _ := Lookup(name)
		var deps []string
		for _, d := range s.DependsOn {
			if ctx.Selected[d] {
				deps = append(deps, d)
			}
		}
		entries = append(entries, SyncEntry{
			Name:            s.Name,
			Path:            fmt.Sprintf("./infra/services/%s/%s/", s.Dir, ctx.Env),
			TargetNamespace: s.TargetNamespace,
			DependsOn:       deps,
		})
	}
	return entries
}

// namespaceEntry is a namespace to declare in infra/overlays/<env>/namespaces.yaml.
type namespaceEntry struct {
	Name   string
	Labels map[string]string
}

func namespaceEntries(ctx *Ctx, services []string) []namespaceEntry {
	seen := map[string]bool{}
	var out []namespaceEntry
	for _, name := range services {
		s, _ := Lookup(name)
		if s.CreateNamespace == "" || seen[s.CreateNamespace] {
			continue
		}
		seen[s.CreateNamespace] = true
		out = append(out, namespaceEntry{Name: s.CreateNamespace, Labels: s.NamespaceLabels})
	}
	// The environment namespace, where tenant-less apps land.
	envLabels := map[string]string{}
	if ctx.Selected["istiod"] {
		envLabels["istio-injection"] = "enabled"
	}
	out = append(out, namespaceEntry{Name: "${env}", Labels: envLabels})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// RegenerateInfraOverlay force-writes the fluxboost-owned files of
// infra/overlays/<env> from the current service selection.
func RegenerateInfraOverlay(res *Result, repoPath string, ctx *Ctx, services []string) error {
	overlay := filepath.Join(repoPath, "infra", "overlays", ctx.Env)
	if err := writeRendered(res, filepath.Join(overlay, "kustomization.yaml"), "overlays/infra-kustomization.yaml.tmpl", ctx, nil, true); err != nil {
		return err
	}
	if err := writeRendered(res, filepath.Join(overlay, "namespaces.yaml"), "overlays/namespaces.yaml.tmpl", ctx,
		map[string]any{"Namespaces": namespaceEntries(ctx, services)}, true); err != nil {
		return err
	}
	return writeRendered(res, filepath.Join(overlay, "sync.yaml"), "overlays/infra-sync.yaml.tmpl", ctx,
		map[string]any{"Entries": syncEntries(ctx, services)}, true)
}

// WriteServiceBase renders a service's shared _base directory. Existing
// files are left untouched (users own their values tweaks).
func WriteServiceBase(res *Result, repoPath, name string, ctx *Ctx) error {
	s, ok := Lookup(name)
	if !ok {
		return fmt.Errorf("unknown service %q", name)
	}
	svcBase := filepath.Join(repoPath, "infra", "services", filepath.FromSlash(s.Dir))
	var baseFiles []string
	for _, f := range s.Files {
		if f.When != nil && !f.When(ctx) {
			continue
		}
		if err := writeRenderedIfMissing(res, filepath.Join(svcBase, "_base", f.Name), f.Tmpl, ctx, nil); err != nil {
			return err
		}
		baseFiles = append(baseFiles, f.Name)
	}
	return writeRenderedIfMissing(res, filepath.Join(svcBase, "_base", "kustomization.yaml"), "kustomization-resources.yaml.tmpl", ctx,
		map[string]any{"Resources": baseFiles})
}

// writeServiceEnvOverlay renders infra/services/<svc>/<env>/kustomization.yaml.
func writeServiceEnvOverlay(res *Result, repoPath, name string, ctx *Ctx) error {
	s, ok := Lookup(name)
	if !ok {
		return fmt.Errorf("unknown service %q", name)
	}
	svcBase := filepath.Join(repoPath, "infra", "services", filepath.FromSlash(s.Dir))
	return writeRenderedIfMissing(res, filepath.Join(svcBase, ctx.Env, "kustomization.yaml"), "kustomization-base.yaml.tmpl", ctx, nil)
}

// renderEnv scaffolds everything environment-specific for one env: the
// cluster entry point, the infra overlay, per-service env overlays, and the
// (initially empty) apps overlay.
func renderEnv(res *Result, o Options, env string) error {
	ctx := ctxFor(env, o)

	base := filepath.Join(o.RepoPath, "clusters", env, "infra")
	if err := writeRendered(res, filepath.Join(base, "kustomization.yaml"), "kustomization-sync.yaml.tmpl", ctx, nil, o.Force); err != nil {
		return err
	}
	if err := writeRendered(res, filepath.Join(base, "sync.yaml"), "cluster/infra-sync.yaml.tmpl", ctx, nil, o.Force); err != nil {
		return err
	}

	if err := RegenerateInfraOverlay(res, o.RepoPath, ctx, o.Services); err != nil {
		return err
	}

	for _, name := range o.Services {
		if err := writeServiceEnvOverlay(res, o.RepoPath, name, ctx); err != nil {
			return err
		}
	}

	// apps/overlays/<env> — empty until `fluxboost app add` registers the
	// first app (which also wires clusters/<env>/apps).
	appsOverlay := filepath.Join(o.RepoPath, "apps", "overlays", env)
	return writeRenderedIfMissing(res, filepath.Join(appsOverlay, "kustomization.yaml"), "kustomization-empty.yaml.tmpl", ctx,
		map[string]any{"Hint": "fluxboost app add"})
}

// Init renders the full repository scaffold and writes fluxboost.yaml.
func Init(o Options) (*Result, error) {
	res := &Result{}

	for _, env := range o.Envs {
		if err := renderEnv(res, o, env); err != nil {
			return nil, err
		}
	}

	// infra/services/<svc>/_base — shared across environments; templates only
	// reference runtime ${env} vars, never a concrete environment.
	ctx := ctxFor(o.Envs[0], o)
	for _, name := range o.Services {
		if err := WriteServiceBase(res, o.RepoPath, name, ctx); err != nil {
			return nil, err
		}
	}

	if o.Sops != nil {
		if err := writeRendered(res, filepath.Join(o.RepoPath, ".sops.yaml"), "sops/sops-yaml.tmpl", ctx,
			map[string]any{"Recipient": o.Sops.AgeRecipient}, o.Force); err != nil {
			return nil, err
		}
		if err := writeRendered(res, filepath.Join(o.RepoPath, ".gitignore"), "sops/gitignore.tmpl", ctx, nil, o.Force); err != nil {
			return nil, err
		}
	}

	// Repo README.
	type bootstrap struct{ Env, Cmd string }
	var boots []bootstrap
	for _, env := range o.Envs {
		boots = append(boots, bootstrap{Env: env, Cmd: BootstrapCommand(o.GitURL, env)})
	}
	if err := writeRendered(res, filepath.Join(o.RepoPath, "README.md"), "repo-readme.md.tmpl", ctx,
		map[string]any{"Envs": o.Envs, "Services": o.Services, "FluxVersion": o.Flux.String(), "Bootstrap": boots}, o.Force); err != nil {
		return nil, err
	}

	cfg := &config.Config{
		Version:            1,
		Provider:           o.Provider,
		BaseDomain:         o.BaseDomain,
		DomainManagerEmail: o.Email,
		GitURL:             o.GitURL,
		Environments:       o.Envs,
		Services:           o.Services,
		Vars:               o.Vars,
		Sops:               o.Sops,
	}
	if err := cfg.Save(o.RepoPath); err != nil {
		return nil, err
	}
	res.Written = append(res.Written, filepath.Join(o.RepoPath, config.FileName))
	return res, nil
}

// AddEnv scaffolds one new environment on an existing repo and re-registers
// every recorded tenant and app in it. The caller appends the env to the
// config and saves it.
func AddEnv(res *Result, cfg *config.Config, repoPath, env string, flux fluxver.Version) error {
	if cfg.HasEnv(env) {
		return fmt.Errorf("environment %q already exists", env)
	}
	o := OptionsFromConfig(cfg, repoPath, flux)
	if err := renderEnv(res, o, env); err != nil {
		return err
	}
	ctx := ctxFor(env, o)
	for _, t := range cfg.Tenants {
		if err := AddTenant(res, repoPath, t, ctx); err != nil {
			return err
		}
	}
	for _, a := range cfg.Apps {
		if err := AddApp(res, repoPath, a, ctx); err != nil {
			return err
		}
	}
	return nil
}

// BootstrapCommand returns the flux bootstrap invocation for an env.
func BootstrapCommand(gitURL, env string) string {
	path := fmt.Sprintf("clusters/%s", env)
	if gitURL == "" {
		return fmt.Sprintf("flux bootstrap git --url=<your-git-url> --branch=main --path=%s", path)
	}
	if strings.Contains(gitURL, "github.com") {
		trimmed := strings.TrimSuffix(gitURL, ".git")
		trimmed = strings.TrimPrefix(trimmed, "ssh://")
		if i := strings.Index(trimmed, "github.com"); i >= 0 {
			parts := strings.Split(strings.Trim(trimmed[i+len("github.com"):], ":/"), "/")
			if len(parts) == 2 {
				return fmt.Sprintf("flux bootstrap github --owner=%s --repository=%s --branch=main --path=%s --personal", parts[0], parts[1], path)
			}
		}
	}
	return fmt.Sprintf("flux bootstrap git --url=%s --branch=main --path=%s", gitURL, path)
}
