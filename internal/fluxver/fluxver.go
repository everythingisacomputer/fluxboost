// Package fluxver detects the flux CLI version installed on the user's
// machine and maps it to the Flux API versions fluxboost should emit.
package fluxver

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Supported flux minor version range (2.x).
const (
	SupportedMinorMin = 7
	SupportedMinorMax = 9
)

// Version is a parsed flux CLI version.
type Version struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

func (v Version) String() string {
	if v.Raw != "" {
		return v.Raw
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Supported reports whether the version falls in the flux range fluxboost
// is tested against (2.7 - 2.9).
func (v Version) Supported() bool {
	return v.Major == 2 && v.Minor >= SupportedMinorMin && v.Minor <= SupportedMinorMax
}

// APIVersions holds the Flux CRD API versions used when rendering manifests.
type APIVersions struct {
	Kustomize      string // kustomize.toolkit.fluxcd.io Kustomization
	GitRepository  string // source.toolkit.fluxcd.io GitRepository
	HelmRepository string // source.toolkit.fluxcd.io HelmRepository
	HelmRelease    string // helm.toolkit.fluxcd.io HelmRelease
	Image          string // image.toolkit.fluxcd.io ImageRepository/ImagePolicy/ImageUpdateAutomation
}

// APIs returns the API versions for the given flux version. Flux 2.7 - 2.9
// all serve the GA APIs, so the mapping is uniform; the indirection stays so
// future flux minors with new API versions only touch this function.
func (v Version) APIs() APIVersions {
	return APIVersions{
		Kustomize:      "kustomize.toolkit.fluxcd.io/v1",
		GitRepository:  "source.toolkit.fluxcd.io/v1",
		HelmRepository: "source.toolkit.fluxcd.io/v1",
		HelmRelease:    "helm.toolkit.fluxcd.io/v2",
		Image:          "image.toolkit.fluxcd.io/v1",
	}
}

// Default is used when no flux CLI can be found.
var Default = Version{Major: 2, Minor: SupportedMinorMax, Patch: 0, Raw: "unknown (assumed 2.9)"}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// Parse extracts a version from strings like "flux version 2.9.1".
func Parse(s string) (Version, error) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("no semantic version found in %q", strings.TrimSpace(s))
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return Version{Major: major, Minor: minor, Patch: patch, Raw: m[0]}, nil
}

// Detect runs `flux --version` and parses the result.
func Detect() (Version, error) {
	path, err := exec.LookPath("flux")
	if err != nil {
		return Default, fmt.Errorf("flux CLI not found in PATH")
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return Default, fmt.Errorf("running flux --version: %w", err)
	}
	v, err := Parse(string(out))
	if err != nil {
		return Default, err
	}
	return v, nil
}
