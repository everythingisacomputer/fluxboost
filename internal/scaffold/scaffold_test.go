package scaffold

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/fluxver"
)

var testFlux = fluxver.Version{Major: 2, Minor: 9, Patch: 0}

func mustResolve(t *testing.T, profile, provider string) []string {
	t.Helper()
	services, _, err := ResolveServices(profile, provider, nil, nil)
	if err != nil {
		t.Fatalf("ResolveServices(%s, %s): %v", profile, provider, err)
	}
	return services
}

// validateYAMLTree parses every YAML file under root, including multi-doc
// files, after stripping Flux substitution vars (${...} is not valid YAML in
// scalar keys, but fine in values; we replace to be safe).
func validateYAMLTree(t *testing.T, root string) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := strings.ReplaceAll(string(b), "${", "SUB_")
		dec := yaml.NewDecoder(strings.NewReader(content))
		for {
			var doc any
			if err := dec.Decode(&doc); err != nil {
				if err == io.EOF {
					break
				}
				t.Errorf("%s: invalid YAML: %v", path, err)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func initRepo(t *testing.T, provider, profile string, envs []string, vars map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	_, err := Init(Options{
		RepoPath:   dir,
		Provider:   provider,
		Envs:       envs,
		Services:   mustResolve(t, profile, provider),
		BaseDomain: "example.com",
		Email:      "ops@example.com",
		Vars:       vars,
		Flux:       testFlux,
	})
	if err != nil {
		t.Fatalf("Init(%s/%s): %v", provider, profile, err)
	}
	return dir
}

func TestInitAllProvidersAndProfiles(t *testing.T) {
	cases := []struct {
		provider string
		vars     map[string]string
	}{
		{"gcp", map[string]string{"gcloudProjectId": "test-project"}},
		{"aws", map[string]string{"awsRegion": "us-east-1", "awsAccountId": "123456789012"}},
		{"baremetal", nil},
	}
	for _, c := range cases {
		for _, profile := range ProfileNames() {
			t.Run(c.provider+"/"+profile, func(t *testing.T) {
				dir := initRepo(t, c.provider, profile, []string{"dev"}, c.vars)
				validateYAMLTree(t, dir)
				for _, want := range []string{
					"clusters/dev/infra/sync.yaml",
					"infra/overlays/dev/sync.yaml",
					"infra/overlays/dev/namespaces.yaml",
					"fluxboost.yaml",
				} {
					if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
						t.Errorf("missing %s", want)
					}
				}
			})
		}
	}
}

func TestBaremetalStandardDropsExternalDNS(t *testing.T) {
	services, warnings, err := ResolveServices("standard", "baremetal", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range services {
		if s == "external-dns" {
			t.Error("external-dns should be dropped on baremetal")
		}
	}
	if len(warnings) == 0 {
		t.Error("expected a warning about external-dns")
	}
	if _, _, err := ResolveServices("standard", "baremetal", []string{"external-dns"}, nil); err == nil {
		t.Error("explicit --with external-dns on baremetal should error")
	}
}

func TestRequirementExpansion(t *testing.T) {
	services, _, err := ResolveServices("minimal", "gcp", []string{"gateway"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(services, ",")
	for _, want := range []string{"cert-manager", "cluster-issuers", "istiod", "gateway"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in %s", want, got)
		}
	}
}

func TestTenantAndAppAdd(t *testing.T) {
	dir := initRepo(t, "gcp", "standard", []string{"dev", "prod"}, map[string]string{"gcloudProjectId": "test-project"})
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	res := &Result{}
	for _, env := range cfg.Environments {
		ctx := CtxFromConfig(cfg, env, testFlux)
		if err := AddTenant(res, dir, config.Tenant{Name: "team-a"}, ctx); err != nil {
			t.Fatalf("AddTenant(%s): %v", env, err)
		}
		if err := AddApp(res, dir, config.App{Name: "api", Type: "deployment", Image: "nginx:1.27", Port: 8080}, ctx); err != nil {
			t.Fatalf("AddApp(%s): %v", env, err)
		}
	}

	// Re-adding the same tenant must fail, not duplicate.
	ctx := CtxFromConfig(cfg, "dev", testFlux)
	if err := AddTenant(res, dir, config.Tenant{Name: "team-a"}, ctx); err == nil {
		t.Error("expected error re-adding tenant team-a")
	}

	validateYAMLTree(t, dir)
	for _, want := range []string{
		"clusters/dev/tenants/sync.yaml",
		"clusters/prod/tenants/sync.yaml",
		"tenants/team-a/_base/rbac.yaml",
		"tenants/team-a/apps/dev/tenant-info.yaml",
		"tenants/overlays/dev/sync.yaml",
		"clusters/dev/apps/sync.yaml",
		"apps/services/api/_base/deployment.yaml",
		"apps/services/api/_base/virtualservice.yaml",
		"apps/overlays/prod/sync.yaml",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s", want)
		}
	}

	// The apps overlay kustomization must now include sync.yaml.
	b, _ := os.ReadFile(filepath.Join(dir, "apps", "overlays", "dev", "kustomization.yaml"))
	if !strings.Contains(string(b), "sync.yaml") {
		t.Errorf("apps overlay kustomization not updated: %s", b)
	}
}
