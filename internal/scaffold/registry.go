package scaffold

import (
	"fmt"
	"sort"
	"strings"
)

// Providers fluxboost knows how to scaffold for.
var Providers = []string{"aws", "gcp", "baremetal"}

func ValidProvider(p string) bool {
	for _, v := range Providers {
		if v == p {
			return true
		}
	}
	return false
}

// File is a manifest rendered into a service's _base directory.
type File struct {
	Name string          // output file name inside _base/
	Tmpl string          // template path under templates/
	When func(*Ctx) bool // nil = always rendered
}

// Service is a platform-level service that can be opted into. The registry
// mirrors the layout of the reference flux-platform repo: each service lives
// under infra/services/<Dir>/{_base,<env>} and is driven by a Flux
// Kustomization declared in infra/overlays/<env>/sync.yaml.
type Service struct {
	Name            string // Flux Kustomization name
	Desc            string
	Dir             string // path under infra/services/
	TargetNamespace string // ${env} allowed
	CreateNamespace string // namespace added to infra/overlays/<env>/namespaces.yaml ("" = none)
	NamespaceLabels map[string]string
	DependsOn       []string // other service names; filtered to the selected set at render time
	Requires        []string // hard prerequisites, auto-added to the selection
	Providers       []string // nil = available on every provider
	Files           []File
}

func (s Service) availableOn(provider string) bool {
	if len(s.Providers) == 0 {
		return true
	}
	for _, p := range s.Providers {
		if p == provider {
			return true
		}
	}
	return false
}

// Registry order is the order services appear in infra/overlays/<env>/sync.yaml.
var Registry = []Service{
	{
		Name:            "cert-manager",
		Desc:            "TLS certificate management (cert-manager + trust-manager)",
		Dir:             "cert-manager",
		TargetNamespace: "cert-manager",
		CreateNamespace: "cert-manager",
		Files: []File{
			{Name: "helmrelease.yaml", Tmpl: "services/cert-manager/helmrelease.yaml.tmpl"},
		},
	},
	{
		Name:            "cluster-issuers",
		Desc:            "Let's Encrypt ClusterIssuers (DNS01 on cloud, HTTP01 on bare metal)",
		Dir:             "cert-manager/cluster-issuers",
		TargetNamespace: "cert-manager",
		DependsOn:       []string{"cert-manager"},
		Requires:        []string{"cert-manager"},
		Files: []File{
			{Name: "cluster-issuers.yaml", Tmpl: "services/cluster-issuers/cluster-issuers.yaml.tmpl"},
		},
	},
	{
		Name:            "istiod",
		Desc:            "Istio control plane (istio-base + istiod)",
		Dir:             "istio/istiod",
		TargetNamespace: "istio-system",
		CreateNamespace: "istio-system",
		NamespaceLabels: map[string]string{"istio-injection": "enabled"},
		DependsOn:       []string{"cert-manager"},
		Files: []File{
			{Name: "helmrelease.yaml", Tmpl: "services/istiod/helmrelease.yaml.tmpl"},
		},
	},
	{
		Name:            "gateway",
		Desc:            "Istio ingress gateway with a TLS certificate for ${env}.${baseDomain}",
		Dir:             "istio/gateway",
		TargetNamespace: "istio-system",
		DependsOn:       []string{"istiod"},
		Requires:        []string{"istiod", "cluster-issuers"},
		Files: []File{
			{Name: "helmrelease.yaml", Tmpl: "services/gateway/helmrelease.yaml.tmpl"},
			{Name: "certificate.yaml", Tmpl: "services/gateway/certificate.yaml.tmpl"},
		},
	},
	{
		Name:            "shared",
		Desc:            "Shared Istio Gateway resource for the environment namespace",
		Dir:             "_shared",
		TargetNamespace: "${env}",
		DependsOn:       []string{"gateway"},
		Requires:        []string{"gateway"},
		Files: []File{
			{Name: "gateway.yaml", Tmpl: "services/shared/gateway.yaml.tmpl"},
		},
	},
	{
		Name:            "external-dns",
		Desc:            "external-dns wired to the cloud DNS provider (aws/gcp only)",
		Dir:             "external-dns",
		TargetNamespace: "external-dns",
		CreateNamespace: "external-dns",
		DependsOn:       []string{"shared"},
		Providers:       []string{"aws", "gcp"},
		Files: []File{
			{Name: "helmrelease.yaml", Tmpl: "services/external-dns/helmrelease.yaml.tmpl"},
			{Name: "rbac.yaml", Tmpl: "services/external-dns/rbac.yaml.tmpl", When: func(c *Ctx) bool { return c.Provider == "gcp" }},
		},
	},
	{
		Name:            "vault",
		Desc:            "HashiCorp Vault (single replica, file storage)",
		Dir:             "vault",
		TargetNamespace: "vault",
		CreateNamespace: "vault",
		DependsOn:       []string{"shared"},
		Files: []File{
			{Name: "helmrelease.yaml", Tmpl: "services/vault/helmrelease.yaml.tmpl"},
		},
	},
	{
		Name:            "external-secrets",
		Desc:            "External Secrets Operator",
		Dir:             "external-secrets",
		TargetNamespace: "external-secrets",
		CreateNamespace: "external-secrets",
		DependsOn:       []string{"vault", "shared"},
		Files: []File{
			{Name: "helmrelease.yaml", Tmpl: "services/external-secrets/helmrelease.yaml.tmpl"},
		},
	},
	{
		Name:            "secret-stores",
		Desc:            "ClusterSecretStore backed by the in-cluster Vault",
		Dir:             "external-secrets/secret-stores",
		TargetNamespace: "external-secrets",
		DependsOn:       []string{"vault", "external-secrets"},
		Requires:        []string{"vault", "external-secrets"},
		Files: []File{
			{Name: "secret-store.yaml", Tmpl: "services/secret-stores/secret-store.yaml.tmpl"},
		},
	},
	{
		Name:            "weave-gitops",
		Desc:            "Weave GitOps dashboard for Flux",
		Dir:             "weave-gitops",
		TargetNamespace: "flux-system",
		DependsOn:       []string{"shared"},
		Files: []File{
			{Name: "helmrelease.yaml", Tmpl: "services/weave-gitops/helmrelease.yaml.tmpl"},
			{Name: "virtualservice.yaml", Tmpl: "services/weave-gitops/virtualservice.yaml.tmpl", When: func(c *Ctx) bool { return c.Selected["gateway"] }},
		},
	},
	{
		Name:            "kubecost",
		Desc:            "Kubecost cost monitoring",
		Dir:             "kubecost",
		TargetNamespace: "admin-dashboards",
		CreateNamespace: "admin-dashboards",
		DependsOn:       []string{"shared"},
		Files: []File{
			{Name: "helmrelease.yaml", Tmpl: "services/kubecost/helmrelease.yaml.tmpl"},
			{Name: "virtualservice.yaml", Tmpl: "services/kubecost/virtualservice.yaml.tmpl", When: func(c *Ctx) bool { return c.Selected["gateway"] }},
		},
	},
}

func Lookup(name string) (Service, bool) {
	for _, s := range Registry {
		if s.Name == name {
			return s, true
		}
	}
	return Service{}, false
}

// Profiles are named service bundles. "standard" tracks the reference
// flux-platform repo; services unavailable on the chosen provider are
// dropped automatically (e.g. external-dns on bare metal).
var Profiles = map[string][]string{
	"minimal":  {"cert-manager", "cluster-issuers"},
	"standard": {"cert-manager", "cluster-issuers", "istiod", "gateway", "shared", "external-secrets", "external-dns", "weave-gitops"},
	"full":     {"cert-manager", "cluster-issuers", "istiod", "gateway", "shared", "external-secrets", "external-dns", "weave-gitops", "vault", "secret-stores", "kubecost"},
}

func ProfileNames() []string {
	names := make([]string, 0, len(Profiles))
	for n := range Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ResolveServices computes the final selection from a profile plus
// --with/--without overrides, expands hard requirements, filters by
// provider, and returns the selection in registry order.
func ResolveServices(profile, provider string, with, without []string) ([]string, []string, error) {
	base, ok := Profiles[profile]
	if !ok {
		return nil, nil, fmt.Errorf("unknown profile %q (available: %s)", profile, strings.Join(ProfileNames(), ", "))
	}

	selected := map[string]bool{}
	for _, n := range base {
		selected[n] = true
	}
	for _, n := range with {
		if _, ok := Lookup(n); !ok {
			return nil, nil, fmt.Errorf("unknown service %q (see `fluxboost services list`)", n)
		}
		selected[n] = true
	}
	for _, n := range without {
		delete(selected, n)
	}

	// Expand hard requirements until stable. Explicit --with of a service
	// unavailable on the provider is an error; profile members are dropped
	// silently with a warning entry instead.
	explicit := map[string]bool{}
	for _, n := range with {
		explicit[n] = true
	}
	for changed := true; changed; {
		changed = false
		for n := range selected {
			s, _ := Lookup(n)
			for _, req := range s.Requires {
				if !selected[req] {
					selected[req] = true
					changed = true
				}
			}
		}
	}

	var warnings []string
	for n := range selected {
		s, _ := Lookup(n)
		if !s.availableOn(provider) {
			if explicit[n] {
				return nil, nil, fmt.Errorf("service %q is not available on provider %q", n, provider)
			}
			delete(selected, n)
			warnings = append(warnings, fmt.Sprintf("service %q is not available on provider %q; dropped from profile", n, provider))
		}
	}

	// Verify requirements survived provider filtering.
	for n := range selected {
		s, _ := Lookup(n)
		for _, req := range s.Requires {
			if !selected[req] {
				return nil, nil, fmt.Errorf("service %q requires %q, which is not available on provider %q", n, req, provider)
			}
		}
	}

	var out []string
	for _, s := range Registry {
		if selected[s.Name] {
			out = append(out, s.Name)
		}
	}
	if len(out) == 0 {
		return nil, nil, fmt.Errorf("no services selected")
	}
	return out, warnings, nil
}
