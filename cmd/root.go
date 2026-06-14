// Package cmd wires the fluxboost CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/everythingisacomputer/fluxboost/internal/fluxver"
)

// version is set via -ldflags at release time.
var version = "0.1.0-dev"

var rootCmd = &cobra.Command{
	Use:   "fluxboost",
	Short: "Scaffold Flux GitOps repositories, platform services, and tenants",
	Long: `fluxboost scaffolds an opinionated Flux GitOps repository layout:
cluster entry points under clusters/<env>, opt-in platform services under
infra/, and namespace-scoped tenants. It supports AWS, GCP, and bare-metal
clusters and renders manifests for the Flux version installed on your machine
(flux 2.7 - 2.9 supported).`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI.
func Execute() error {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
	}
	return err
}

// detectFlux resolves the flux CLI version, warning (not failing) when it is
// missing or outside the supported 2.7 - 2.9 range.
func detectFlux() fluxver.Version {
	v, err := fluxver.Detect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v; assuming flux 2.%d APIs\n", err, fluxver.SupportedMinorMax)
		return v
	}
	if !v.Supported() {
		fmt.Fprintf(os.Stderr, "warning: flux %s detected; fluxboost supports flux 2.%d - 2.%d and will render manifests for that range\n",
			v, fluxver.SupportedMinorMin, fluxver.SupportedMinorMax)
	}
	return v
}
