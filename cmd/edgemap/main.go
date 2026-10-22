// Command edgemap turns Terraform configuration into architecture diagrams.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// version is set by release builds. Other builds fall back to the module
// version Go records, such as v0.1.0 for go install ...@v0.1.0.
var version = "dev"

func buildVersion(main string, info *debug.BuildInfo, ok bool) string {
	if main != "dev" || !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return main
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

func main() {
	if err := newCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newCommand() *cobra.Command {
	root := &cobra.Command{Use: "edgemap", Short: "Diagram Terraform architecture", SilenceUsage: true}
	root.AddCommand(&cobra.Command{Use: "version", Short: "Print the build version", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		info, ok := debug.ReadBuildInfo()
		_, err := fmt.Fprintln(cmd.OutOrStdout(), buildVersion(version, info, ok))
		return err
	}})
	root.AddCommand(inspectCommand(), renderCommand())
	return root
}
