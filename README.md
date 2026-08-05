# fluxboost

[![ci](https://github.com/everythingisacomputer/fluxboost/actions/workflows/ci.yml/badge.svg)](https://github.com/everythingisacomputer/fluxboost/actions/workflows/ci.yml)
[![release](https://github.com/everythingisacomputer/fluxboost/actions/workflows/release.yml/badge.svg)](https://github.com/everythingisacomputer/fluxboost/actions/workflows/release.yml)
[![latest release](https://img.shields.io/github/v/release/everythingisacomputer/fluxboost)](https://github.com/everythingisacomputer/fluxboost/releases/latest)
[![buy me a coffee](https://img.shields.io/badge/buy%20me%20a%20coffee-☕-ffdd00?logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/everythingisacomputer)

Scaffold [Flux](https://fluxcd.io) GitOps repositories with opt-in platform
services, SOPS secrets encryption, and namespace-scoped tenants, for **AWS**,
**GCP**, and **bare metal** clusters.

The generated layout follows a clusters → overlays → services pattern:

```
clusters/<env>/          Flux entry points (what `flux bootstrap` applies)
  infra/                 platform services Kustomization + substitution values
  tenants/, apps/        wired lazily by `tenant add` / `app add`
infra/
  overlays/<env>/        namespaces + one Flux Kustomization per service,
                         chained with dependsOn
  services/<svc>/        _base/ shared manifests, <env>/ overlay
tenants/<name>/          namespace, scoped reconciler SA, tenant-owned app tree
apps/services/<name>/    application manifests
fluxboost.yaml           records every choice; the source of truth for all
                         post-init commands
```

Environment-specific values (`${env}`, `${baseDomain}`, provider ids) are set
once per cluster and flow down through Flux `postBuild.substitute` — services
and apps reference them symbolically.

![fluxboost init demo: the wizard collects domain, platform, credentials, profile, and SOPS choices, then scaffolds the repo](demo-init.gif)

## Install

With Homebrew (macOS or Linux):

```sh
brew tap everythingisacomputer/tap
brew trust everythingisacomputer/tap   # newer Homebrew requires trusting third-party taps
brew install fluxboost
```

Or with Go:

```sh
go install github.com/everythingisacomputer/fluxboost@latest
```

## Getting started

Run `fluxboost init` in an **empty directory** (a lone `.git` is fine — a
freshly cloned repo works), or point it at one with `fluxboost init <dir>` —
the directory is created if it does not exist yet. If the directory has any
other content, it exits. Otherwise it walks you through an interactive
wizard — a styled TUI (built with [huh](https://github.com/charmbracelet/huh))
when running in a terminal, ending with a recap you confirm before anything
is written:

```
$ fluxboost init platform
Base domain (e.g. example.com): example.com
Domain manager email (Let's Encrypt registration): you@example.com
Cloud platform (aws/gcp/baremetal): gcp
GCP project id (DNS + Workload Identity): my-project
Profile (standard) [standard]:
Environments (comma-separated) [dev]:
Git remote URL (optional, used for flux bootstrap hints): git@github.com:you/platform
Enable SOPS secrets encryption (age)? (Y/n): y
Set up tenants now? (y/N): y
  Tenant name (empty line to finish): team-a
  Tenant name (empty line to finish):
```

When stdin/stdout is not a terminal, the wizard falls back to plain line
prompts, so answers can be piped for scripting:
`printf 'example.com\nyou@example.com\nbaremetal\nstandard\ndev\n\ny\nn\n' | fluxboost init`.

After the wizard, push the repo and run the `flux bootstrap` command it
prints for each environment. With SOPS enabled, also back up the generated
`age.agekey` (gitignored) and create the decryption secret per cluster:

```sh
kubectl -n flux-system create secret generic sops-age --from-file=age.agekey=age.agekey
```

## Growing the repo

![fluxboost tenant add demo: adding a tenant scaffolds the namespace, scoped RBAC, and tenant-owned reconciler](demo-tenant.gif)

```sh
# New environment: scaffolds clusters/<env> + overlays and re-registers
# every recorded tenant and app
fluxboost env add prod

# Toggle platform services; requirements are pulled in automatically and
# overlays regenerate from fluxboost.yaml
fluxboost service add vault
fluxboost service remove kubecost
fluxboost service list

# Tenants: namespace + ServiceAccount that is admin only within it + a
# tenant-owned Flux Kustomization
fluxboost tenant add team-b
# ...or reconciling from the tenant's own repository:
fluxboost tenant add team-ext --repo git@github.com:you/team-ext --branch main --repo-path ./deploy

# Apps: deployment (+ VirtualService with istio) or cronjob, optionally with
# Flux image automation committing new semver tags back to the repo
fluxboost app add api --image ghcr.io/you/api:1.0.0 --port 8080 --image-automation
fluxboost app add nightly --type cronjob --image ghcr.io/you/job:1.0.0 --schedule "0 3 * * *"

# Validate any time: builds every kustomization with an embedded kustomize
# and flags undefined substitution variables
fluxboost check
```

## Platform services

The wizard currently offers one profile, **standard**. The registry also
defines `minimal` and `full` bundles, and `service add`/`remove` covers
everything in between.

| Service | Providers | Profiles |
|---|---|---|
| cert-manager (+ trust-manager) | all | minimal, standard, full |
| cluster-issuers (Let's Encrypt) | all | minimal, standard, full |
| istiod, gateway, shared | all | standard, full |
| external-secrets | all | standard, full |
| external-dns | aws, gcp | standard, full |
| weave-gitops | all | standard, full |
| vault, secret-stores | all | full |
| kubecost | all | full |

Dependencies are handled for you: hard requirements are auto-added
(`service add gateway` pulls in istiod and cluster-issuers), `dependsOn`
chains are filtered to the services you selected, and provider-specific bits
(DNS01 solvers, Workload Identity / IRSA annotations, external-dns providers)
are rendered per provider.

## Flux version support

fluxboost targets **flux 2.7 – 2.9** and renders the GA Flux APIs
(`kustomize.toolkit.fluxcd.io/v1`, `source.toolkit.fluxcd.io/v1`,
`helm.toolkit.fluxcd.io/v2`, `image.toolkit.fluxcd.io/v1`). It detects the
flux CLI on your PATH and warns when the installed version is outside that
range.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Support

If fluxboost saves you time, you can
[buy me a coffee](https://buymeacoffee.com/everythingisacomputer). ☕
