// Package check validates a fluxboost-managed repo: every kustomization
// must build, and every Flux substitution variable referenced by the built
// manifests must be defined at the cluster level.
package check

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/everythingisacomputer/fluxboost/internal/config"
)

// Result of a repo check.
type Result struct {
	Built    int      // kustomization directories that built successfully
	Problems []string // human-readable failures; empty means the repo is healthy
}

var varRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Run builds every kustomization under repoPath and cross-checks
// substitution variables against the ones fluxboost defines.
func Run(repoPath string, cfg *config.Config) (*Result, error) {
	res := &Result{}

	defined := map[string]bool{"env": true, "baseDomain": true, "domainManagerEmail": true}
	for k := range cfg.Vars {
		defined[k] = true
	}

	var dirs []string
	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "flux-system") {
			// flux-system holds bootstrap output owned by flux itself.
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "kustomization.yaml" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)

	fs := filesys.MakeFsOnDisk()
	k := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	usedVars := map[string][]string{}
	for _, dir := range dirs {
		rm, err := k.Run(fs, dir)
		rel, relErr := filepath.Rel(repoPath, dir)
		if relErr != nil {
			rel = dir
		}
		if err != nil {
			res.Problems = append(res.Problems, fmt.Sprintf("%s: kustomize build failed: %v", rel, err))
			continue
		}
		res.Built++
		out, err := rm.AsYaml()
		if err != nil {
			res.Problems = append(res.Problems, fmt.Sprintf("%s: rendering build output: %v", rel, err))
			continue
		}
		for _, m := range varRe.FindAllStringSubmatch(string(out), -1) {
			name := m[1]
			if !defined[name] && !contains(usedVars[name], rel) {
				usedVars[name] = append(usedVars[name], rel)
			}
		}
	}

	var undefined []string
	for name := range usedVars {
		undefined = append(undefined, name)
	}
	sort.Strings(undefined)
	for _, name := range undefined {
		res.Problems = append(res.Problems, fmt.Sprintf(
			"substitution variable ${%s} is used in %s but not defined in fluxboost.yaml vars",
			name, strings.Join(usedVars[name], ", ")))
	}
	return res, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
