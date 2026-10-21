package main

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
	"github.com/Infraedgecloud-tools/edgemap/internal/parse"
	"github.com/spf13/cobra"
)

func inspectCommand() *cobra.Command {
	return &cobra.Command{Use: "inspect [dir]", Short: "Inspect static nodes, containment and warnings", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		g, err := graph.Build(cmd.Context(), parse.Terraform{}, dir)
		if err != nil {
			return err
		}
		counts := map[string]int{}
		for _, e := range g.Edges {
			counts[e.From] += e.Count
		}
		var text strings.Builder
		text.WriteString("NODE\tTYPE\tCONTAINER\tEDGES\tBADGES\n")
		for _, id := range g.IDs() {
			n := g.Nodes[id]
			fmt.Fprintf(&text, "%s\t%s\t%s\t%d\t%s\n", id, n.Type, n.Container, counts[id], strings.Join(n.Badge, ", "))
		}
		for _, warning := range g.Warnings {
			fmt.Fprintf(&text, "warning: %s\n", warning)
		}
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprint(writer, text.String()); err != nil {
			return err
		}
		return writer.Flush()
	}}
}
