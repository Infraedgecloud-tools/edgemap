package parse

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Infraedgecloud-tools/edgemap/internal/source"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

func TestLoadFixtures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      string
		labels    []string
		attribute string
		wantRefs  []string
	}{
		{"two-tier", "resource", []string{"aws_instance", "web"}, "subnet_id", []string{"aws_subnet.public.id"}},
		{"multi-az", "resource", []string{"aws_lb", "web"}, "subnets", []string{"aws_subnet.public_a.id", "aws_subnet.public_b.id"}},
		{"local-modules", "resource", []string{"aws_instance", "web"}, "subnet_id", []string{"module.network.public_subnet_id"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (Terraform{}).Load(t.Context(), filepath.Join("..", "..", "testdata", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Warnings) != 0 {
				t.Fatalf("valid fixture produced warnings: %v", got.Warnings)
			}
			block := findBlock(t, got, tc.kind, tc.labels...)
			if refs := block.Attributes[tc.attribute].References; !reflect.DeepEqual(refs, tc.wantRefs) {
				t.Fatalf("references = %v, want %v", refs, tc.wantRefs)
			}
			if tc.name == "local-modules" {
				for _, b := range got.Blocks {
					if b.Kind == "resource" && b.Labels[0] == "aws_vpc" {
						t.Fatal("source adapter unexpectedly traversed a module directory")
					}
				}
			}
		})
	}
}

func TestLoadNestedDynamicReferences(t *testing.T) {
	got := loadText(t, "main.tf", `
variable "rules" {}
resource "aws_security_group" "web" {
  vpc_id = aws_vpc.main.id
  dynamic "ingress" {
    for_each = var.rules
    content {
      from_port = ingress.value.port
      security_groups = [aws_security_group.other.id]
    }
  }
  depends_on = [aws_vpc.main, aws_security_group.other]
}
`)
	b := findBlock(t, got, "resource", "aws_security_group", "web")
	refs := map[string]bool{}
	for key, attr := range b.Attributes {
		for _, ref := range attr.References {
			refs[ref] = true
		}
		if strings.Contains(key, "security_groups") && !strings.Contains(key, "content") {
			t.Fatalf("nested block path lost: %s", key)
		}
	}
	for _, ref := range []string{"aws_vpc.main.id", "var.rules", "ingress.value.port", "aws_security_group.other.id", "aws_vpc.main", "aws_security_group.other"} {
		if !refs[ref] {
			t.Errorf("missing reference %s", ref)
		}
	}
}

func TestLoadJSONAndLiteralBoundaries(t *testing.T) {
	got := loadText(t, "main.tf.json", `{
  "provider": {"aws": {"region": "eu-west-2"}},
  "locals": {"literal": "unchanged"},
  "resource": {"aws_instance": {"web": {
    "count": 3,
    "subnet_id": "${aws_subnet.public.id}",
    "tags": {"Name": "web"},
    "depends_on": ["aws_security_group.web"]
  }}},
  "output": {"id": {"value": "${aws_instance.web.id}"}}
}`)
	if len(got.Warnings) != 0 {
		t.Fatalf("valid JSON produced warnings: %v", got.Warnings)
	}
	b := findBlock(t, got, "resource", "aws_instance", "web")
	if attr := b.Attributes["count"]; !attr.Literal || attr.Value != float64(3) {
		t.Errorf("count literal = %#v", attr)
	}
	if attr := b.Attributes["subnet_id"]; attr.Literal || !reflect.DeepEqual(attr.References, []string{"aws_subnet.public.id"}) {
		t.Errorf("JSON interpolation = %#v", attr)
	}
	if attr := b.Attributes["tags"]; !attr.Literal || !reflect.DeepEqual(attr.Value, map[string]any{"Name": "web"}) {
		t.Errorf("JSON object literal = %#v", attr)
	}
	out := findBlock(t, got, "output", "id")
	if !reflect.DeepEqual(out.Attributes["value"].References, []string{"aws_instance.web.id"}) {
		t.Fatal("JSON output reference lost")
	}
	findBlock(t, got, "locals")
	findBlock(t, got, "provider", "aws")
}

func TestLoadDoesNotEvaluateExpressions(t *testing.T) {
	got := loadText(t, "main.tf", `
locals {
  literal = "kept"
  call = upper("not evaluated")
  choice = true ? "yes" : "no"
  indexed = aws_instance.web[0].id
}
resource "aws_instance" "web" {
  count = 3
  subnet_id = var.enabled ? aws_subnet.a.id : aws_subnet.b.id
}
`)
	locals := findBlock(t, got, "locals")
	if attr := locals.Attributes["literal"]; !attr.Literal || attr.Value != "kept" {
		t.Fatal("literal string not retained")
	}
	for _, name := range []string{"call", "choice", "indexed"} {
		if locals.Attributes[name].Literal {
			t.Errorf("%s incorrectly evaluated", name)
		}
	}
	if refs := locals.Attributes["indexed"].References; !reflect.DeepEqual(refs, []string{"aws_instance.web.id"}) {
		t.Errorf("indexed reference = %v", refs)
	}
	web := findBlock(t, got, "resource", "aws_instance", "web")
	if refs := web.Attributes["subnet_id"].References; !reflect.DeepEqual(refs, []string{"aws_subnet.a.id", "aws_subnet.b.id", "var.enabled"}) {
		t.Errorf("conditional dependencies = %v", refs)
	}
}

func TestLoadRecoverableDiagnostics(t *testing.T) {
	dir := t.TempDir()
	writeInput(t, dir, "valid.tf", `resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }`)
	writeInput(t, dir, "broken.tf", "resource \"aws_instance\" \"broken\" {\n subnet_id =\n")
	writeInput(t, dir, "ignored.txt", "not HCL")
	got, err := (Terraform{}).Load(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) == 0 {
		t.Fatal("malformed HCL has no warning")
	}
	findBlock(t, got, "resource", "aws_vpc", "main")
	for _, warning := range got.Warnings {
		if strings.Contains(warning, dir) {
			t.Fatal("diagnostic unnecessarily exposes absolute path")
		}
	}
}

func TestDiagnosticSummariesDoNotIncludeDetail(t *testing.T) {
	var got source.Result
	addDiagnostics(&got, hcl.Diagnostics{&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid expression",
		Detail:   "Sensitive value: FAKE_SECRET_SENTINEL",
		Subject:  &hcl.Range{Filename: "/private/customer/main.tf", Start: hcl.Pos{Line: 7}},
	}})
	if !reflect.DeepEqual(got.Warnings, []string{"main.tf:7: Invalid expression"}) {
		t.Fatalf("unsafe or incorrect summary: %v", got.Warnings)
	}
}

func TestLoadDirectoryAndCancellationErrors(t *testing.T) {
	if _, err := (Terraform{}).Load(t.Context(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory accepted")
	}
	dir := t.TempDir()
	writeInput(t, dir, "main.tf", "")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (Terraform{}).Load(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		`resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }`,
		`resource "aws_instance" "web" { subnet_id = aws_subnet.main[0].id }`,
		`resource "aws_instance" "broken" { subnet_id =`,
		`locals { name = upper("example") }`,
		`{"resource":{"aws_vpc":{"main":{"cidr_block":"10.0.0.0/16"}}}}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		for _, jsonInput := range []bool{false, true} {
			parser := hclparse.NewParser()
			var file *hcl.File
			if jsonInput {
				file, _ = parser.ParseJSON(data, "fuzz.tf.json")
			} else {
				file, _ = parser.ParseHCL(data, "fuzz.tf")
			}
			if file != nil {
				var got source.Result
				collect(file, &got)
			}
		}
	})
}

func loadText(t *testing.T, name, text string) source.Result {
	t.Helper()
	dir := t.TempDir()
	writeInput(t, dir, name, text)
	got, err := (Terraform{}).Load(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func writeInput(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func findBlock(t *testing.T, result source.Result, kind string, labels ...string) source.Block {
	t.Helper()
	for _, b := range result.Blocks {
		if b.Kind == kind && slices.Equal(b.Labels, labels) {
			return b
		}
	}
	t.Fatalf("missing %s block %v", kind, labels)
	return source.Block{}
}

func TestReviewRegressions(t *testing.T) {
	dir := t.TempDir()
	data := `{"provider":{"aws":{"alias":"dr","region":"${upper(\"eu-west-2\")}"}},"resource":{"aws_instance":{"web":{"provider":"aws.dr","depends_on":["aws_security_group.web"],"tags":{"Name":"${true ? \"public\" : \"private\"}"}}}}}`
	if err := os.WriteFile(filepath.Join(dir, "main.tf.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := (Terraform{}).Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range result.Blocks {
		if b.Kind == "provider" && b.Attributes["region"].Literal {
			t.Fatal("JSON interpolation treated as literal")
		}
		if b.Kind == "resource" {
			if got := b.Attributes["depends_on"].References; len(got) != 1 || got[0] != "aws_security_group.web" {
				t.Fatalf("depends_on: %v", got)
			}
			if got := b.Attributes["provider"].References; len(got) != 1 || got[0] != "aws.dr" {
				t.Fatalf("provider: %v", got)
			}
			if b.Attributes["tags"].Literal {
				t.Fatal("nested interpolation treated as literal")
			}
		}
	}
}

func TestRejectSymlinkInput(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("/dev/zero", filepath.Join(dir, "main.tf")); err != nil {
		t.Skip(err)
	}
	result, err := (Terraform{}).Load(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) == 0 || len(result.Blocks) != 0 {
		t.Fatal("symlink was not rejected")
	}
}

func TestComputedObjectKeyNotLiteral(t *testing.T) {
	p := hclparse.NewParser()
	f, d := p.ParseHCL([]byte(`locals { tags = { (true ? "public" : "private") = "value" } }`), "main.tf")
	if d.HasErrors() {
		t.Fatal(d.Error())
	}
	var result source.Result
	collect(f, &result)
	if result.Blocks[0].Attributes["tags"].Literal {
		t.Fatal("computed key evaluated")
	}
}
