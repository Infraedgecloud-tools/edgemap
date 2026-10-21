// Command edgemap turns Terraform configuration into architecture diagrams.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := newCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	root := &cobra.Command{Use: "edgemap", Short: "Diagram Terraform architecture", SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print the build version", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), version)
		return err
	}})
	return root
}
