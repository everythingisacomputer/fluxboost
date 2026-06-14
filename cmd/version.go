package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/everythingisacomputer/fluxboost/internal/fluxver"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show fluxboost and detected flux versions",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(cmd.OutOrStdout(), "fluxboost %s (supports flux 2.%d - 2.%d)\n", version, fluxver.SupportedMinorMin, fluxver.SupportedMinorMax)
		v, err := fluxver.Detect()
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "flux: not detected (%v)\n", err)
			return nil
		}
		status := "supported"
		if !v.Supported() {
			status = "outside supported range"
		}
		apis := v.APIs()
		fmt.Fprintf(cmd.OutOrStdout(), "flux: %s (%s)\n", v, status)
		fmt.Fprintf(cmd.OutOrStdout(), "rendered APIs: Kustomization %s, GitRepository %s, HelmRepository %s, HelmRelease %s\n",
			apis.Kustomize, apis.GitRepository, apis.HelmRepository, apis.HelmRelease)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
