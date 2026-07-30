# Contributing

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

The scaffolding engine lives in `internal/scaffold` (service registry in
`registry.go`, embedded manifests under `templates/`). CLI commands live in
`cmd/`. `fluxboost.yaml` written into scaffolded repos is the source of truth
for every post-init command — if you add state, record it there.

Manifest changes should keep `fluxboost check` green on a freshly scaffolded
repo for every provider (the test suite covers this via the embedded
kustomize).

## Releasing

Releases are cut by tagging:

```sh
git tag v0.x.y && git push origin v0.x.y
```

The release workflow runs tests and security checks, then GoReleaser builds
darwin/linux amd64/arm64 archives, publishes the GitHub release, and updates
the Homebrew formula in everythingisacomputer/homebrew-tap (requires the
`TAP_GITHUB_TOKEN` repo secret).
