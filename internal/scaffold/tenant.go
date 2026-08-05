package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/everythingisacomputer/fluxboost/internal/config"
)

// validSegment guards path segments (tenant/app names) that callers already
// validate at the CLI layer; scaffold enforces it again so no caller can
// smuggle path separators into generated paths.
var validSegment = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func checkSegment(kind, name string) error {
	if !validSegment.MatchString(name) {
		return fmt.Errorf("invalid %s name %q", kind, name)
	}
	return nil
}

// ensureResource makes sure a kustomization.yaml lists the given resource,
// creating the file if needed.
func ensureResource(res *Result, path, resource string) error {
	b, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path is inside the repo fluxboost was invoked on
	if os.IsNotExist(err) {
		content := fmt.Sprintf("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - %s\n", resource)
		return writeFile(res, path, []byte(content), false)
	}
	if err != nil {
		return err
	}
	var doc struct {
		APIVersion string   `yaml:"apiVersion"`
		Kind       string   `yaml:"kind"`
		Resources  []string `yaml:"resources"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	for _, r := range doc.Resources {
		if r == resource {
			return nil
		}
	}
	doc.Resources = append(doc.Resources, resource)
	out, err := yaml.Marshal(map[string]any{
		"apiVersion": "kustomize.config.k8s.io/v1beta1",
		"kind":       "Kustomization",
		"resources":  doc.Resources,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return err
	}
	res.Written = append(res.Written, path)
	return nil
}

// appendBlock appends a rendered YAML block to a sync.yaml, creating it with
// a header when missing. Refuses to append twice for the same marker.
func appendBlock(res *Result, path, header, marker string, block []byte) error {
	existing, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- path is inside the repo fluxboost was invoked on
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if strings.Contains(string(existing), marker) {
		return fmt.Errorf("%s already contains %q", path, marker)
	}
	var out []byte
	if len(existing) == 0 {
		out = []byte(header)
	} else {
		out = existing
		if !strings.HasSuffix(string(out), "\n") {
			out = append(out, '\n')
		}
	}
	out = append(out, block...)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// #nosec G703 -- path segments are validated by checkSegment before use
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return err
	}
	res.Written = append(res.Written, path)
	return nil
}

// ensureClusterSync wires clusters/<env>/<kind> (kind: tenants or apps) — the
// cluster-level Flux Kustomization that points at <kind>/overlays/<env>. It
// is created lazily by the first tenant/app add so Flux never reconciles an
// empty overlay.
func ensureClusterSync(res *Result, repoPath, kind string, ctx *Ctx) error {
	dir := filepath.Join(repoPath, "clusters", ctx.Env, kind)
	syncPath := filepath.Join(dir, "sync.yaml")
	if _, err := os.Stat(syncPath); err == nil {
		return nil
	}
	if err := writeRendered(res, syncPath, "cluster/group-sync.yaml.tmpl", ctx,
		map[string]any{"Kind": kind}, false); err != nil {
		return err
	}
	return writeRendered(res, filepath.Join(dir, "kustomization.yaml"), "kustomization-sync.yaml.tmpl", ctx, nil, false)
}

// AddTenant scaffolds tenants/<name> and registers it for one environment.
// A tenant with Repo set reconciles its apps from its own git repository
// instead of the platform repo.
func AddTenant(res *Result, repoPath string, t config.Tenant, ctx *Ctx) error {
	if err := checkSegment("tenant", t.Name); err != nil {
		return err
	}
	if err := ensureClusterSync(res, repoPath, "tenants", ctx); err != nil {
		return err
	}
	overlay := filepath.Join(repoPath, "tenants", "overlays", ctx.Env)
	if err := ensureResource(res, filepath.Join(overlay, "kustomization.yaml"), "sync.yaml"); err != nil {
		return err
	}

	branch := t.Branch
	if branch == "" {
		branch = "main"
	}
	path := t.Path
	if path == "" {
		path = "./"
	}
	extra := map[string]any{"Name": t.Name, "Repo": t.Repo, "Branch": branch, "Path": path}

	tenantDir := filepath.Join(repoPath, "tenants", t.Name)
	baseResources := []string{"rbac.yaml", "sync.yaml"}
	if err := writeRenderedIfMissing(res, filepath.Join(tenantDir, "_base", "rbac.yaml"), "tenant/rbac.yaml.tmpl", ctx, extra); err != nil {
		return err
	}
	if err := writeRenderedIfMissing(res, filepath.Join(tenantDir, "_base", "sync.yaml"), "tenant/sync.yaml.tmpl", ctx, extra); err != nil {
		return err
	}
	if t.Repo != "" {
		if err := writeRenderedIfMissing(res, filepath.Join(tenantDir, "_base", "gitrepository.yaml"), "tenant/gitrepository.yaml.tmpl", ctx, extra); err != nil {
			return err
		}
		baseResources = append(baseResources, "gitrepository.yaml")
	}
	if err := writeRenderedIfMissing(res, filepath.Join(tenantDir, "_base", "kustomization.yaml"), "kustomization-resources.yaml.tmpl", ctx,
		map[string]any{"Resources": baseResources}); err != nil {
		return err
	}
	if err := writeRendered(res, filepath.Join(tenantDir, ctx.Env, "kustomization.yaml"), "kustomization-base.yaml.tmpl", ctx, nil, false); err != nil {
		return err
	}
	if t.Repo == "" {
		// Seed the in-repo tenant's app tree with a real resource so the
		// tenant-owned Kustomization never reconciles an empty build.
		if err := writeRendered(res, filepath.Join(tenantDir, "apps", ctx.Env, "kustomization.yaml"), "kustomization-resources.yaml.tmpl", ctx,
			map[string]any{"Resources": []string{"tenant-info.yaml"}}, false); err != nil {
			return err
		}
		if err := writeRendered(res, filepath.Join(tenantDir, "apps", ctx.Env, "tenant-info.yaml"), "tenant/tenant-info.yaml.tmpl", ctx, extra, false); err != nil {
			return err
		}
	}

	block, err := render("tenant/register.yaml.tmpl", ctx, extra)
	if err != nil {
		return err
	}
	return appendBlock(res, filepath.Join(overlay, "sync.yaml"),
		"# Tenants registered for this environment. Managed by `fluxboost tenant add`.\n",
		"name: tenant-"+t.Name, block)
}

// AddApp scaffolds apps/services/<name> and registers it for one environment.
func AddApp(res *Result, repoPath string, app config.App, ctx *Ctx) error {
	if err := checkSegment("app", app.Name); err != nil {
		return err
	}
	if app.Type == "" {
		app.Type = "deployment"
	}
	if err := ensureClusterSync(res, repoPath, "apps", ctx); err != nil {
		return err
	}
	overlay := filepath.Join(repoPath, "apps", "overlays", ctx.Env)
	if err := ensureResource(res, filepath.Join(overlay, "kustomization.yaml"), "sync.yaml"); err != nil {
		return err
	}

	host := app.Host
	if host == "" {
		host = app.Name + ".${env}.${baseDomain}"
	}
	schedule := app.Schedule
	if schedule == "" {
		schedule = "0 * * * *"
	}
	// The registry-watching ImageRepository wants the image without its tag.
	imageRepo := app.Image
	if i := strings.LastIndex(imageRepo, ":"); i > strings.LastIndex(imageRepo, "/") {
		imageRepo = imageRepo[:i]
	}
	extra := map[string]any{
		"Name": app.Name, "Image": app.Image, "Port": app.Port, "Host": host,
		"Schedule": schedule, "ImageAutomation": app.ImageAutomation, "ImageRepo": imageRepo,
	}

	appDir := filepath.Join(repoPath, "apps", "services", app.Name)
	var files []struct{ name, tmpl string }
	switch app.Type {
	case "deployment":
		files = []struct{ name, tmpl string }{
			{"deployment.yaml", "app/deployment.yaml.tmpl"},
			{"svc.yaml", "app/svc.yaml.tmpl"},
			{"sa.yaml", "app/sa.yaml.tmpl"},
		}
		if ctx.Selected["gateway"] {
			files = append(files, struct{ name, tmpl string }{"virtualservice.yaml", "app/virtualservice.yaml.tmpl"})
		}
	case "cronjob":
		files = []struct{ name, tmpl string }{
			{"cronjob.yaml", "app/cronjob.yaml.tmpl"},
			{"sa.yaml", "app/sa.yaml.tmpl"},
		}
	default:
		return fmt.Errorf("unknown app type %q (deployment | cronjob)", app.Type)
	}
	var names []string
	for _, f := range files {
		if err := writeRenderedIfMissing(res, filepath.Join(appDir, "_base", f.name), f.tmpl, ctx, extra); err != nil {
			return err
		}
		names = append(names, f.name)
	}
	if err := writeRenderedIfMissing(res, filepath.Join(appDir, "_base", "kustomization.yaml"), "kustomization-resources.yaml.tmpl", ctx,
		map[string]any{"Resources": names}); err != nil {
		return err
	}
	if err := writeRendered(res, filepath.Join(appDir, ctx.Env, "kustomization.yaml"), "kustomization-base.yaml.tmpl", ctx, nil, false); err != nil {
		return err
	}

	// dependsOn: the selected terminal services apps usually need.
	var deps []string
	for _, d := range []string{"shared", "external-secrets", "istiod"} {
		if ctx.Selected[d] {
			deps = append(deps, d)
		}
	}
	extra["DependsOn"] = deps
	block, err := render("app/register.yaml.tmpl", ctx, extra)
	if err != nil {
		return err
	}
	if err := appendBlock(res, filepath.Join(overlay, "sync.yaml"),
		"# Applications registered for this environment. Managed by `fluxboost app add`.\n",
		"name: "+app.Name+"\n", block); err != nil {
		return err
	}

	// Image automation objects live in flux-system and rewrite the shared
	// _base in git, so exactly one set exists per app: it is registered only
	// in the primary (first) environment's overlay.
	if app.ImageAutomation && ctx.PrimaryEnv {
		auto, err := render("app/imageautomation.yaml.tmpl", ctx, extra)
		if err != nil {
			return err
		}
		if err := appendBlock(res, filepath.Join(overlay, "sync.yaml"),
			"", "# image-automation: "+app.Name, auto); err != nil {
			return err
		}
	}
	return nil
}
