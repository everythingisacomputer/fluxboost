package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/everythingisacomputer/fluxboost/internal/check"
	"github.com/everythingisacomputer/fluxboost/internal/config"
)

func initRepoWithSops(t *testing.T) (string, *config.Config) {
	t.Helper()
	dir := t.TempDir()
	_, err := Init(Options{
		RepoPath:   dir,
		Provider:   "gcp",
		Envs:       []string{"dev"},
		Services:   mustResolve(t, "standard", "gcp"),
		BaseDomain: "example.com",
		Email:      "ops@example.com",
		Vars:       map[string]string{"gcloudProjectId": "test-project"},
		Sops:       &config.Sops{AgeRecipient: "age1testrecipient"},
		Flux:       testFlux,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, cfg
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

func runCheck(t *testing.T, dir string, cfg *config.Config) {
	t.Helper()
	res, err := check.Run(dir, cfg)
	if err != nil {
		t.Fatalf("check.Run: %v", err)
	}
	for _, p := range res.Problems {
		t.Errorf("check problem: %s", p)
	}
	if res.Built == 0 {
		t.Error("check built no kustomizations")
	}
}

func TestSopsScaffold(t *testing.T) {
	dir, cfg := initRepoWithSops(t)
	if cfg.Sops == nil || cfg.Sops.AgeRecipient != "age1testrecipient" {
		t.Fatalf("sops config not recorded: %+v", cfg.Sops)
	}
	if s := mustRead(t, filepath.Join(dir, ".sops.yaml")); !strings.Contains(s, "age1testrecipient") {
		t.Errorf(".sops.yaml missing recipient: %s", s)
	}
	if s := mustRead(t, filepath.Join(dir, ".gitignore")); !strings.Contains(s, "age.agekey") {
		t.Errorf(".gitignore missing age.agekey: %s", s)
	}
	for _, f := range []string{"clusters/dev/infra/sync.yaml", "infra/overlays/dev/sync.yaml"} {
		if s := mustRead(t, filepath.Join(dir, f)); !strings.Contains(s, "sops-age") {
			t.Errorf("%s missing sops decryption block", f)
		}
	}
	runCheck(t, dir, cfg)
}

func TestAddEnv(t *testing.T) {
	dir, cfg := initRepoWithSops(t)

	// Seed a tenant and an app so AddEnv must re-register them.
	res := &Result{}
	ctx := CtxFromConfig(cfg, "dev", testFlux)
	tenant := config.Tenant{Name: "team-a"}
	app := config.App{Name: "api", Type: "deployment", Image: "nginx:1.27", Port: 8080}
	if err := AddTenant(res, dir, tenant, ctx); err != nil {
		t.Fatal(err)
	}
	if err := AddApp(res, dir, app, ctx); err != nil {
		t.Fatal(err)
	}
	cfg.Tenants = append(cfg.Tenants, tenant)
	cfg.Apps = append(cfg.Apps, app)

	if err := AddEnv(res, cfg, dir, "prod", testFlux); err != nil {
		t.Fatal(err)
	}
	cfg.Environments = append(cfg.Environments, "prod")

	for _, want := range []string{
		"clusters/prod/infra/sync.yaml",
		"clusters/prod/tenants/sync.yaml",
		"clusters/prod/apps/sync.yaml",
		"infra/overlays/prod/sync.yaml",
		"infra/services/cert-manager/prod/kustomization.yaml",
		"tenants/overlays/prod/sync.yaml",
		"tenants/team-a/prod/kustomization.yaml",
		"tenants/team-a/apps/prod/tenant-info.yaml",
		"apps/overlays/prod/sync.yaml",
		"apps/services/api/prod/kustomization.yaml",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("missing %s", want)
		}
	}
	if err := AddEnv(res, cfg, dir, "prod", testFlux); err == nil {
		t.Error("expected error re-adding env prod")
	}
	validateYAMLTree(t, dir)
	runCheck(t, dir, cfg)
}

func TestServiceAddRemove(t *testing.T) {
	dir, cfg := initRepoWithSops(t)

	res := &Result{}
	added, err := AddService(res, cfg, dir, "secret-stores", testFlux)
	if err != nil {
		t.Fatal(err)
	}
	// secret-stores requires vault, which was not in the standard profile.
	if got := strings.Join(added, ","); !strings.Contains(got, "vault") || !strings.Contains(got, "secret-stores") {
		t.Errorf("added = %v, want vault and secret-stores", added)
	}
	sync := mustRead(t, filepath.Join(dir, "infra/overlays/dev/sync.yaml"))
	for _, want := range []string{"name: vault", "name: secret-stores"} {
		if !strings.Contains(sync, want) {
			t.Errorf("overlay sync missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "infra/services/vault/_base/helmrelease.yaml")); err != nil {
		t.Error("vault _base not written")
	}
	runCheck(t, dir, cfg)

	// Removing vault while secret-stores requires it must fail.
	if err := RemoveService(res, cfg, dir, "vault", testFlux); err == nil {
		t.Error("expected error removing vault while secret-stores is enabled")
	}
	if err := RemoveService(res, cfg, dir, "secret-stores", testFlux); err != nil {
		t.Fatal(err)
	}
	if err := RemoveService(res, cfg, dir, "vault", testFlux); err != nil {
		t.Fatal(err)
	}
	sync = mustRead(t, filepath.Join(dir, "infra/overlays/dev/sync.yaml"))
	if strings.Contains(sync, "name: vault") || strings.Contains(sync, "name: secret-stores") {
		t.Errorf("overlay sync still references removed services")
	}
	if _, err := os.Stat(filepath.Join(dir, "infra/services/vault")); !os.IsNotExist(err) {
		t.Error("vault service dir not deleted")
	}
	runCheck(t, dir, cfg)
}

func TestTenantWithRepo(t *testing.T) {
	dir, cfg := initRepoWithSops(t)

	res := &Result{}
	ctx := CtxFromConfig(cfg, "dev", testFlux)
	tenant := config.Tenant{Name: "team-ext", Repo: "https://github.com/example/team-ext", Branch: "release", Path: "./deploy"}
	if err := AddTenant(res, dir, tenant, ctx); err != nil {
		t.Fatal(err)
	}

	gitrepo := mustRead(t, filepath.Join(dir, "tenants/team-ext/_base/gitrepository.yaml"))
	for _, want := range []string{"https://github.com/example/team-ext", "branch: release"} {
		if !strings.Contains(gitrepo, want) {
			t.Errorf("gitrepository.yaml missing %q", want)
		}
	}
	sync := mustRead(t, filepath.Join(dir, "tenants/team-ext/_base/sync.yaml"))
	if !strings.Contains(sync, "path: ./deploy") || !strings.Contains(sync, "name: team-ext") {
		t.Errorf("tenant sync not pointing at tenant repo: %s", sync)
	}
	if strings.Contains(sync, "flux-system") {
		t.Errorf("external tenant should not source from flux-system: %s", sync)
	}
	// External tenants have no in-repo app tree.
	if _, err := os.Stat(filepath.Join(dir, "tenants/team-ext/apps")); !os.IsNotExist(err) {
		t.Error("external tenant should not get an apps tree")
	}
	cfg.Tenants = append(cfg.Tenants, tenant)
	validateYAMLTree(t, dir)
	runCheck(t, dir, cfg)
}

func TestCronjobAndImageAutomation(t *testing.T) {
	dir, cfg := initRepoWithSops(t)

	res := &Result{}
	ctx := CtxFromConfig(cfg, "dev", testFlux)
	cron := config.App{Name: "nightly", Type: "cronjob", Image: "ghcr.io/example/job:1.2.3", Schedule: "0 3 * * *", ImageAutomation: true}
	if err := AddApp(res, dir, cron, ctx); err != nil {
		t.Fatal(err)
	}
	cfg.Apps = append(cfg.Apps, cron)

	cj := mustRead(t, filepath.Join(dir, "apps/services/nightly/_base/cronjob.yaml"))
	if !strings.Contains(cj, `schedule: "0 3 * * *"`) {
		t.Errorf("cronjob missing schedule: %s", cj)
	}
	if !strings.Contains(cj, `{"$imagepolicy": "flux-system:nightly"}`) {
		t.Errorf("cronjob missing image policy marker: %s", cj)
	}
	if _, err := os.Stat(filepath.Join(dir, "apps/services/nightly/_base/svc.yaml")); !os.IsNotExist(err) {
		t.Error("cronjob app should not get a Service")
	}
	sync := mustRead(t, filepath.Join(dir, "apps/overlays/dev/sync.yaml"))
	for _, want := range []string{"kind: ImageRepository", "kind: ImagePolicy", "kind: ImageUpdateAutomation", "image: ghcr.io/example/job"} {
		if !strings.Contains(sync, want) {
			t.Errorf("apps sync missing %q", want)
		}
	}
	if strings.Contains(sync, "image: ghcr.io/example/job:1.2.3\n") {
		t.Error("ImageRepository should reference the image without its tag")
	}

	// The automation objects must register only in the primary env.
	if err := AddEnv(res, cfg, dir, "prod", testFlux); err != nil {
		t.Fatal(err)
	}
	cfg.Environments = append(cfg.Environments, "prod")
	prodSync := mustRead(t, filepath.Join(dir, "apps/overlays/prod/sync.yaml"))
	if strings.Contains(prodSync, "ImageUpdateAutomation") {
		t.Error("image automation must not be duplicated in non-primary envs")
	}
	validateYAMLTree(t, dir)
	runCheck(t, dir, cfg)
}

func TestCheckCatchesUndefinedVar(t *testing.T) {
	dir, cfg := initRepoWithSops(t)
	// Inject a manifest referencing an undefined substitution variable.
	path := filepath.Join(dir, "infra/services/cert-manager/_base/helmrelease.yaml")
	b := mustRead(t, path)
	if err := os.WriteFile(path, []byte(b+"\n# uses ${notDefinedAnywhere}\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: bad\ndata:\n  x: ${notDefinedAnywhere}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := check.Run(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range res.Problems {
		if strings.Contains(p, "notDefinedAnywhere") {
			found = true
		}
	}
	if !found {
		t.Errorf("check did not flag undefined variable; problems: %v", res.Problems)
	}
}
