package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/everythingisacomputer/fluxboost/internal/check"
	"github.com/everythingisacomputer/fluxboost/internal/config"
	"github.com/everythingisacomputer/fluxboost/internal/fluxver"
)

var checkFlags struct {
	path string
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate the repo: build every kustomization and verify substitutions",
	Long: `Builds every kustomization in the repo with an embedded kustomize (no
kubectl required), verifies that every Flux substitution variable referenced
by the built manifests is defined, and reports the flux CLI version status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(checkFlags.path)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()

		v, err := fluxver.Detect()
		switch {
		case err != nil:
			fmt.Fprintf(out, "flux CLI: not detected (%v)\n", err)
		case !v.Supported():
			fmt.Fprintf(out, "flux CLI: %s (outside supported range 2.%d - 2.%d)\n", v, fluxver.SupportedMinorMin, fluxver.SupportedMinorMax)
		default:
			fmt.Fprintf(out, "flux CLI: %s (supported)\n", v)
		}

		res, err := check.Run(checkFlags.path, cfg)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "kustomizations built: %d\n", res.Built)
		if len(res.Problems) == 0 {
			fmt.Fprintln(out, successStyle.Render("Repo is healthy."))
			return nil
		}
		for _, p := range res.Problems {
			fmt.Fprintln(out, "PROBLEM:", p)
		}
		return fmt.Errorf("%d problem(s) found", len(res.Problems))
	},
}

func init() {
	checkCmd.Flags().StringVar(&checkFlags.path, "path", ".", "repo root (containing fluxboost.yaml)")
	rootCmd.AddCommand(checkCmd)
}
