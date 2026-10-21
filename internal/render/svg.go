package render

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/lib/imgbundler"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/simplelog"
	"github.com/d2lang/d2/lib/textmeasure"
)

func SVG(ctx context.Context, input, layout string) ([]byte, error) {
	if layout != "elk" && layout != "dagre" {
		return nil, fmt.Errorf("layout must be elk or dagre")
	}
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, err
	}
	resolver := func(engine string) (d2graph.LayoutGraph, error) {
		switch engine {
		case "elk":
			return d2elklayout.DefaultLayout, nil
		case "dagre":
			return d2dagrelayout.DefaultLayout, nil
		default:
			return nil, fmt.Errorf("unsupported layout %s", engine)
		}
	}
	ctx = log.WithDefault(ctx)
	padding := int64(40)
	opts := &d2svg.RenderOpts{Pad: &padding}
	diagram, _, err := d2lib.Compile(ctx, input, &d2lib.CompileOptions{Layout: &layout, LayoutResolver: resolver, Ruler: ruler}, opts)
	if err != nil {
		return nil, fmt.Errorf("compile D2: %w", err)
	}
	svg, err := d2svg.Render(diagram, opts)
	if err != nil {
		return nil, fmt.Errorf("render SVG: %w", err)
	}
	bundled, err := imgbundler.BundleLocal(ctx, simplelog.Make(nil, nil, nil), "", svg, false)
	if err != nil {
		return nil, fmt.Errorf("bundle local icons: %w", err)
	}
	return bundled, nil
}
