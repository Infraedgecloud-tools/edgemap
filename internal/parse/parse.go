// Package parse reads Terraform HCL and JSON without running Terraform.
package parse

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Infraedgecloud-tools/edgemap/internal/source"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// Terraform implements the static source adapter.
type Terraform struct{}

var _ source.Source = Terraform{}

var schema = &hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
	{Type: "resource", LabelNames: []string{"type", "name"}},
	{Type: "data", LabelNames: []string{"type", "name"}},
	{Type: "module", LabelNames: []string{"name"}},
	{Type: "variable", LabelNames: []string{"name"}},
	{Type: "output", LabelNames: []string{"name"}},
	{Type: "provider", LabelNames: []string{"name"}},
	{Type: "locals"},
}}

func (Terraform) Load(ctx context.Context, dir string) (source.Result, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return source.Result{}, fmt.Errorf("read configuration directory: %w", err)
	}
	var result source.Result
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") && !strings.HasSuffix(entry.Name(), ".tf.json") {
			continue
		}
		filename := filepath.Join(dir, entry.Name())
		data, err := source.ReadRegularFile(filename)
		if err != nil {
			result.Warnings = append(result.Warnings, entry.Name()+": "+err.Error())
			continue
		}
		parser := hclparse.NewParser()
		var file *hcl.File
		var diagnostics hcl.Diagnostics
		if strings.HasSuffix(entry.Name(), ".json") {
			file, diagnostics = parser.ParseJSON(data, filename)
		} else {
			file, diagnostics = parser.ParseHCL(data, filename)
		}
		addDiagnostics(&result, diagnostics)
		if file == nil {
			continue
		}
		collect(file, &result)
	}
	return result, nil
}

func collect(file *hcl.File, result *source.Result) {
	content, _, diagnostics := file.Body.PartialContent(schema)
	addDiagnostics(result, diagnostics)
	for _, block := range content.Blocks {
		item := source.Block{Kind: block.Type, Labels: block.Labels, Attributes: map[string]source.Attribute{}, File: block.DefRange.Filename}
		if body, ok := block.Body.(*hclsyntax.Body); ok {
			walk(body, "", file.Bytes, item.Attributes)
		} else {
			attrs, diags := block.Body.JustAttributes()
			addDiagnostics(result, diags)
			for key, attr := range attrs {
				a := attribute(attr.Expr, file.Bytes)
				if key == "depends_on" || key == "provider" {
					a = traversalAttribute(a)
				}
				if key == "providers" {
					a.Bindings = providerBindings(attr.Expr)
				}
				item.Attributes[key] = a
			}
		}
		result.Blocks = append(result.Blocks, item)
	}
}

func walk(body *hclsyntax.Body, prefix string, data []byte, attrs map[string]source.Attribute) {
	for key, attr := range body.Attributes {
		a := attribute(attr.Expr, data)
		if key == "providers" {
			a.Bindings = providerBindings(attr.Expr)
		}
		attrs[prefix+key] = a
	}
	for i, block := range body.Blocks {
		walk(block.Body, fmt.Sprintf("%s%s.%d.", prefix, block.Type, i), data, attrs)
	}
}

func attribute(expr hcl.Expression, data []byte) source.Attribute {
	attr := source.Attribute{Raw: string(expr.Range().SliceBytes(data))}
	for _, traversal := range expr.Variables() {
		var parts []string
		for _, step := range traversal {
			switch step := step.(type) {
			case hcl.TraverseRoot:
				parts = append(parts, step.Name)
			case hcl.TraverseAttr:
				parts = append(parts, step.Name)
			}
		}
		if len(parts) > 0 {
			attr.References = append(attr.References, strings.Join(parts, "."))
		}
	}
	sort.Strings(attr.References)
	if len(attr.References) != 0 || !literalExpression(expr) {
		return attr
	}
	value, diags := expr.Value(nil)
	if diags.HasErrors() || !value.IsWhollyKnown() {
		return attr
	}
	bytes, err := ctyjson.Marshal(value, value.Type())
	if err != nil {
		return attr
	}
	if err := json.Unmarshal(bytes, &attr.Value); err == nil {
		attr.Literal = true
		if _, native := expr.(hclsyntax.Expression); !native {
			attr.Literal = jsonLiteral(attr.Value)
		}
	}
	return attr
}

func literalExpression(expr hcl.Expression) bool {
	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		return true
	case *hclsyntax.TemplateExpr:
		return e.IsStringLiteral()
	case *hclsyntax.TupleConsExpr:
		for _, v := range e.Exprs {
			if !literalExpression(v) {
				return false
			}
		}
		return true
	case *hclsyntax.ObjectConsExpr:
		for _, v := range e.Items {
			if !literalExpression(v.KeyExpr) || !literalExpression(v.ValueExpr) {
				return false
			}
		}
		return true
	case *hclsyntax.ObjectConsKeyExpr:
		if !e.ForceNonLiteral && hcl.ExprAsKeyword(e.Wrapped) != "" {
			return true
		}
		return literalExpression(e.Wrapped)
	case hclsyntax.Expression:
		return false
	default:
		return true // JSON expressions evaluate only with no traversals or context.
	}
}

func addDiagnostics(result *source.Result, diagnostics hcl.Diagnostics) {
	for _, diag := range diagnostics {
		location := "configuration"
		if diag.Subject != nil {
			location = fmt.Sprintf("%s:%d", filepath.Base(diag.Subject.Filename), diag.Subject.Start.Line)
		}
		// Detail can contain user-provided values. Keep summaries only.
		result.Warnings = append(result.Warnings, location+": "+diag.Summary)
	}
}

func traversalAttribute(attr source.Attribute) source.Attribute {
	var values []string
	if s, ok := attr.Value.(string); ok {
		values = append(values, s)
	}
	if list, ok := attr.Value.([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok {
				values = append(values, s)
			}
		}
	}
	for _, s := range values {
		tr, diags := hclsyntax.ParseTraversalAbs([]byte(s), "reference", hcl.InitialPos)
		if diags.HasErrors() {
			continue
		}
		var names []string
		for _, part := range tr {
			switch p := part.(type) {
			case hcl.TraverseRoot:
				names = append(names, p.Name)
			case hcl.TraverseAttr:
				names = append(names, p.Name)
			}
		}
		attr.References = append(attr.References, strings.Join(names, "."))
	}
	sort.Strings(attr.References)
	return attr
}

func jsonLiteral(value any) bool {
	switch v := value.(type) {
	case string:
		return !strings.Contains(v, "${") && !strings.Contains(v, "%{")
	case []any:
		for _, item := range v {
			if !jsonLiteral(item) {
				return false
			}
		}
	case map[string]any:
		for k, item := range v {
			if !jsonLiteral(k) || !jsonLiteral(item) {
				return false
			}
		}
	}
	return true
}

func providerBindings(expr hcl.Expression) map[string]string {
	result := map[string]string{}
	pairs, diags := hcl.ExprMap(expr)
	if diags.HasErrors() {
		return result
	}
	for _, pair := range pairs {
		keyName := ""
		if tr, d := hcl.AbsTraversalForExpr(pair.Key); !d.HasErrors() {
			var names []string
			for _, part := range tr {
				switch p := part.(type) {
				case hcl.TraverseRoot:
					names = append(names, p.Name)
				case hcl.TraverseAttr:
					names = append(names, p.Name)
				}
			}
			keyName = strings.Join(names, ".")
		} else if key, d := pair.Key.Value(nil); !d.HasErrors() && key.IsKnown() && !key.IsNull() && key.Type() == cty.String {
			keyName = key.AsString()
		}
		if keyName == "" {
			continue
		}
		refs := pair.Value.Variables()
		if len(refs) == 1 {
			var names []string
			for _, part := range refs[0] {
				switch p := part.(type) {
				case hcl.TraverseRoot:
					names = append(names, p.Name)
				case hcl.TraverseAttr:
					names = append(names, p.Name)
				}
			}
			result[keyName] = strings.Join(names, ".")
		} else {
			value, d := pair.Value.Value(nil)
			if !d.HasErrors() && value.IsKnown() && !value.IsNull() && value.Type() == cty.String {
				result[keyName] = value.AsString()
			}
		}
	}
	return result
}
