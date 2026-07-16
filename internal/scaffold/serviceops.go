package scaffold

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/fluxver"
)

// AddService enables a platform service (plus its hard requirements) on an
// existing repo: writes missing service manifests and regenerates the infra
// overlays. cfg.Services is updated in place; the caller saves the config.
func AddService(res *Result, cfg *config.Config, repoPath, name string, flux fluxver.Version) ([]string, error) {
	s, ok := Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown service %q (see `fluxboost service list`)", name)
	}
	if !s.availableOn(cfg.Provider) {
		return nil, fmt.Errorf("service %q is not available on provider %q", name, cfg.Provider)
	}
	if cfg.HasService(name) {
		return nil, fmt.Errorf("service %q is already enabled", name)
	}

	// Expand hard requirements until stable.
	selected := map[string]bool{}
	for _, n := range cfg.Services {
		selected[n] = true
	}
	selected[name] = true
	for changed := true; changed; {
		changed = false
		for n := range selected {
			svc, _ := Lookup(n)
			for _, req := range svc.Requires {
				if !selected[req] {
					r, _ := Lookup(req)
					if !r.availableOn(cfg.Provider) {
						return nil, fmt.Errorf("service %q requires %q, which is not available on provider %q", n, req, cfg.Provider)
					}
					selected[req] = true
					changed = true
				}
			}
		}
	}

	var services, added []string
	for _, svc := range Registry {
		if selected[svc.Name] {
			services = append(services, svc.Name)
			if !cfg.HasService(svc.Name) {
				added = append(added, svc.Name)
			}
		}
	}
	cfg.Services = services

	if err := regenerateAll(res, cfg, repoPath, flux, added); err != nil {
		return nil, err
	}
	return added, nil
}

// RemoveService disables a platform service, regenerates the infra overlays,
// and deletes the service's manifest directory. cfg.Services is updated in
// place; the caller saves the config.
func RemoveService(res *Result, cfg *config.Config, repoPath, name string, flux fluxver.Version) error {
	if !cfg.HasService(name) {
		return fmt.Errorf("service %q is not enabled", name)
	}
	for _, n := range cfg.Services {
		if n == name {
			continue
		}
		s, _ := Lookup(n)
		for _, req := range s.Requires {
			if req == name {
				return fmt.Errorf("service %q requires %q — remove %q first", n, name, n)
			}
		}
	}

	var services []string
	for _, n := range cfg.Services {
		if n != name {
			services = append(services, n)
		}
	}
	cfg.Services = services

	if err := regenerateAll(res, cfg, repoPath, flux, nil); err != nil {
		return err
	}

	s, _ := Lookup(name)
	dir := filepath.Join(repoPath, "infra", "services", filepath.FromSlash(s.Dir))
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	res.Removed = append(res.Removed, dir)
	return nil
}

// regenerateAll rewrites the infra overlays for every environment and, for
// newly added services, writes their _base and per-env overlays.
func regenerateAll(res *Result, cfg *config.Config, repoPath string, flux fluxver.Version, newServices []string) error {
	o := OptionsFromConfig(cfg, repoPath, flux)
	for _, env := range cfg.Environments {
		ctx := ctxFor(env, o)
		if err := RegenerateInfraOverlay(res, repoPath, ctx, cfg.Services); err != nil {
			return err
		}
		for _, name := range newServices {
			if err := writeServiceEnvOverlay(res, repoPath, name, ctx); err != nil {
				return err
			}
		}
	}
	if len(newServices) > 0 {
		ctx := ctxFor(cfg.Environments[0], o)
		for _, name := range newServices {
			if err := WriteServiceBase(res, repoPath, name, ctx); err != nil {
				return err
			}
		}
	}
	return nil
}
