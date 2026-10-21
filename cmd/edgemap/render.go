package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Infraedgecloud-tools/edgemap/internal/filter"
	"github.com/Infraedgecloud-tools/edgemap/internal/graph"
	"github.com/Infraedgecloud-tools/edgemap/internal/parse"
	"github.com/Infraedgecloud-tools/edgemap/internal/plan"
	"github.com/Infraedgecloud-tools/edgemap/internal/render"
	"github.com/Infraedgecloud-tools/edgemap/internal/source"
	"github.com/spf13/cobra"
)

func renderCommand() *cobra.Command {
	var out, format, layout, title, planPath string
	var noIcons bool
	var filters filter.Options
	cmd := &cobra.Command{Use: "render [dir]", Short: "Render static Terraform architecture", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if format != "svg" && format != "d2" {
			return fmt.Errorf("format must be svg or d2")
		}
		if layout != "elk" && layout != "dagre" {
			return fmt.Errorf("layout must be elk or dagre")
		}
		if out == "" {
			out = "diagram." + format
		}
		if filepath.Ext(out) != "."+format {
			return fmt.Errorf("output extension must match --format %s", format)
		}
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		g, err := graph.Build(cmd.Context(), parse.Terraform{}, dir)
		if err != nil {
			return err
		}
		options := render.Options{Title: title}
		if planPath != "" {
			data, err := source.ReadRegularFile(planPath)
			if err != nil {
				return fmt.Errorf("read plan: %w", err)
			}
			changes, err := plan.Read(bytes.NewReader(data))
			if err != nil {
				return err
			}
			options.Colors, err = plan.Apply(g, changes)
			if err != nil {
				return err
			}
			options.Legend = plan.Legend
		}
		g = filter.Apply(g, filters)
		var warnings []string
		if !noIcons {
			options.Icons, warnings = render.Icons(cmd.Context(), g)
		}
		d2, err := render.Emit(g, options)
		if err != nil {
			return err
		}
		if format == "svg" {
			svg, err := render.SVG(cmd.Context(), d2, layout)
			if err != nil && len(options.Icons) > 0 {
				options.Icons = nil
				warnings = append(warnings, "icon render failed; using plain shapes")
				d2, err = render.Emit(g, options)
				if err != nil {
					return err
				}
				svg, err = render.SVG(cmd.Context(), d2, layout)
			}
			if err != nil {
				return err
			}
			if err := writeOutput(out, svg); err != nil {
				return err
			}
			if err := writeOutput(strings.TrimSuffix(out, ".svg")+".d2", []byte(d2)); err != nil {
				return err
			}
		} else if err := writeOutput(out, []byte(d2)); err != nil {
			return err
		}
		for _, warning := range warnings {
			cmd.PrintErrln("warning:", warning)
		}
		if len(g.Warnings) > 0 {
			cmd.PrintErrf("%d static-analysis warnings; run edgemap inspect for details\n", len(g.Warnings))
		}
		return nil
	}}
	flags := cmd.Flags()
	flags.StringVar(&planPath, "plan", "", "Local terraform show -json plan (never uploaded)")
	flags.StringVar(&out, "out", "", "Output path (default diagram.svg or diagram.d2)")
	flags.StringVar(&format, "format", "svg", "Output format: svg or d2")
	flags.StringVar(&layout, "layout", "elk", "Layout engine: elk or dagre")
	flags.StringVar(&title, "title", "Terraform architecture", "Diagram title")
	flags.BoolVar(&noIcons, "no-icons", false, "Disable all icon network access")
	flags.BoolVar(&filters.ShowIAM, "show-iam", false, "Draw IAM resources")
	flags.BoolVar(&filters.ShowData, "show-data", false, "Draw data sources")
	flags.BoolVar(&filters.ShowSG, "show-sg", false, "Draw security groups and rules")
	return cmd
}

func writeOutput(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".edgemap-*")
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace output: %w", err)
	}
	return nil
}
