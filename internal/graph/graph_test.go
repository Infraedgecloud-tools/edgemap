package graph

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Infraedgecloud-tools/edgemap/internal/parse"
)

func TestFixtureContainment(t *testing.T) {
	for _, tc := range []struct {
		name    string
		parents map[string]string
	}{
		{
			"two-tier",
			map[string]string{
				"aws_vpc.app":              "region:eu-west-2",
				"aws_subnet.public":        "aws_vpc.app.tier.public",
				"aws_instance.web":         "aws_vpc.app.tier.public",
				"aws_subnet.private":       "aws_vpc.app.tier.private",
				"aws_db_subnet_group.data": "aws_vpc.app.tier.private",
				"aws_db_instance.data":     "aws_vpc.app.tier.private",
				"aws_security_group.web":   "aws_vpc.app",
				"aws_iam_role.web":         "global",
				"aws_s3_bucket.logs":       "global",
			},
		},
		{
			"multi-az",
			map[string]string{
				"aws_vpc.app":                 "region:eu-west-2",
				"aws_vpc.dr":                  "region:eu-west-1",
				"aws_subnet.public_a":         "aws_vpc.app.tier.public",
				"aws_subnet.public_b":         "aws_vpc.app.tier.public",
				"aws_lb.web":                  "aws_vpc.app.tier.public",
				"aws_db_instance.data":        "aws_vpc.app.tier.private",
				"aws_autoscaling_group.mixed": "aws_vpc.app",
			},
		},
		{
			"local-modules",
			map[string]string{
				"module.network.aws_vpc.main":      "region:eu-west-2",
				"module.network.aws_subnet.public": "module.network.aws_vpc.main.tier.public",
				"aws_instance.web":                 "module.network.aws_vpc.main.tier.public",
			},
		},
		{
			"vpc-module",
			map[string]string{
				"module.vpc":               "region:eu-west-2",
				"aws_lb.web":               "module.vpc.tier.public",
				"aws_db_subnet_group.data": "module.vpc.tier.private",
				"aws_db_instance.data":     "module.vpc.tier.private",
				"aws_security_group.app":   "module.vpc",
			},
		},
		{
			"for-each",
			map[string]string{
				"aws_vpc.app":           "region:region: var.region",
				"aws_instance.workers":  "aws_vpc.app.tier.private",
				"aws_instance.fixed":    "aws_vpc.app.tier.private",
				"aws_s3_bucket.archive": "global",
			},
		},
		{
			"broken",
			map[string]string{"aws_vpc.valid": "region:eu-west-2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := fixtureGraph(t, tc.name)
			for id, wantParent := range tc.parents {
				node := requireNode(t, g, id)
				if node.Container != wantParent {
					t.Errorf("%s container = %q, want %q", id, node.Container, wantParent)
				}
				if parent := requireNode(t, g, wantParent); !parent.Group {
					t.Errorf("%s is not a container", wantParent)
				}
			}
			if tc.name == "broken" && len(g.Warnings) == 0 {
				t.Error("broken fixture did not produce warnings")
			}
			for _, node := range g.Nodes {
				if node.Container != "" {
					requireNode(t, g, node.Container)
				}
			}
			for _, edge := range g.Edges {
				requireNode(t, g, edge.From)
				requireNode(t, g, edge.To)
				if g.Ancestor(edge.From, edge.To) {
					t.Errorf("nesting already expresses edge %s -> %s", edge.From, edge.To)
				}
			}
		})
	}
}

func TestUnfilteredGraphRetainsNoiseNodes(t *testing.T) {
	g := fixtureGraph(t, "two-tier")
	for _, id := range []string{"aws_iam_role.web", "aws_iam_instance_profile.web", "aws_security_group.web", "data.aws_caller_identity.current"} {
		requireNode(t, g, id)
	}
	if !g.Nodes["data.aws_caller_identity.current"].Data {
		t.Error("data source lost its kind")
	}
	for _, forbidden := range []string{"aws", "provider.aws", "variable.region"} {
		if _, exists := g.Nodes[forbidden]; exists {
			t.Errorf("plumbing node drawn: %s", forbidden)
		}
	}
}

func TestCollapsedCountsAndUnresolvedRegion(t *testing.T) {
	g := fixtureGraph(t, "for-each")
	if got := requireNode(t, g, "aws_instance.workers"); !slices.Contains(got.Badge, "×?") {
		t.Errorf("unknown fan-out badge = %v", got.Badge)
	}
	if got := requireNode(t, g, "aws_instance.fixed"); !slices.Contains(got.Badge, "×3") {
		t.Errorf("literal count badge = %v", got.Badge)
	}
	if got := requireNode(t, g, "aws_vpc.app"); got.Region != "region: var.region" {
		t.Errorf("unresolved region = %q", got.Region)
	}
	if got := requireNode(t, g, "random_id.suffix"); got.Group {
		t.Error("non-AWS resource should be a plain node")
	}
	for id := range g.Nodes {
		if strings.Contains(id, "[") {
			t.Errorf("fan-out incorrectly expanded: %s", id)
		}
	}
}

func TestCollapsedVPCModuleMetadata(t *testing.T) {
	g := fixtureGraph(t, "vpc-module")
	node := requireNode(t, g, "module.vpc")
	if !node.Group {
		t.Fatal("known VPC module is not a container")
	}
	for _, text := range []string{"terraform-aws-modules/vpc/aws", "5.21.0"} {
		if !strings.Contains(node.Label, text) {
			t.Errorf("module label omits %q: %q", text, node.Label)
		}
	}
}

func TestReferenceDirectionAndBundling(t *testing.T) {
	dir := t.TempDir()
	writeGraphInput(t, dir, "main.tf", `
resource "example_service" "server" {}
resource "example_service" "client" {
  endpoint = example_service.server.endpoint
  identity = example_service.server.id
  depends_on = [example_service.server]
}
`)
	g, err := Build(context.Background(), parse.Terraform{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("edges = %#v, want one bundled edge", g.Edges)
	}
	edge := g.Edges[0]
	if edge.From != "example_service.client" || edge.To != "example_service.server" || edge.Count != 3 {
		t.Errorf("edge = %#v, want client -> server with count 3", edge)
	}
}

func TestModuleCycleWarnsWithoutRecursingForever(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	writeGraphInput(t, dir, "main.tf", `module "child" { source = "./child" }`)
	writeGraphInput(t, child, "main.tf", `module "back" { source = "../" }`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	g, err := Build(ctx, parse.Terraform{}, dir)
	if err != nil {
		t.Fatalf("cycle should be recoverable: %v", err)
	}
	found := false
	for _, warning := range g.Warnings {
		if strings.Contains(warning, "cycle") {
			found = true
		}
	}
	if !found {
		t.Fatalf("cycle warning missing: %v", g.Warnings)
	}
}

func fixtureGraph(t *testing.T, name string) *Graph {
	t.Helper()
	g, err := Build(context.Background(), parse.Terraform{}, filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func requireNode(t *testing.T, g *Graph, id string) *Node {
	t.Helper()
	node := g.Nodes[id]
	if node == nil {
		t.Fatalf("missing node %q", id)
	}
	return node
}

func writeGraphInput(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestModuleProviderMappingOverridesDefault(t *testing.T) {
	g := graphFromFiles(t, map[string]string{
		"main.tf": `
provider "aws" { region = "us-east-1" }
provider "aws" {
  alias = "west"
  region = "us-west-2"
}
module "child" {
  source = "./child"
  providers = { aws = aws.west }
}
module "remote" {
  source = "example/network/aws"
  providers = { aws = aws.west }
}
resource "aws_vpc" "root" { cidr_block = "10.0.0.0/16" }
`,
		"child/main.tf": `resource "aws_vpc" "child" { cidr_block = "10.1.0.0/16" }`,
	})
	for _, id := range []string{"module.child.aws_vpc.child", "module.remote"} {
		n := requireNode(t, g, id)
		if n.Region != "us-west-2" || n.Container != "region:us-west-2" {
			t.Errorf("mapped provider for %s: region=%q container=%q", id, n.Region, n.Container)
		}
	}
	if n := requireNode(t, g, "aws_vpc.root"); n.Region != "us-east-1" {
		t.Errorf("child mapping changed root region: %q", n.Region)
	}
}

func TestModuleDependenciesConnectExpandedResources(t *testing.T) {
	g := graphFromFiles(t, map[string]string{
		"main.tf": `
resource "example_service" "prerequisite" {}
module "child" {
  source = "./child"
  depends_on = [example_service.prerequisite]
}
resource "example_service" "consumer" { depends_on = [module.child] }
`,
		"child/main.tf": `
resource "example_service" "a" {}
resource "example_service" "b" {}
`,
	})
	for _, child := range []string{"module.child.example_service.a", "module.child.example_service.b"} {
		requireGraphEdge(t, g, child, "example_service.prerequisite", 1)
		requireGraphEdge(t, g, "example_service.consumer", child, 1)
	}
	if len(g.Edges) != 4 {
		t.Errorf("unexpected module dependency edges: %#v", g.Edges)
	}
}

func TestModuleMultiplicitySurvivesExpansion(t *testing.T) {
	g := graphFromFiles(t, map[string]string{
		"main.tf": `
module "child" {
  source = "./child"
  count = 3
}
module "remote" {
  source = "example/network/aws"
  count = 4
}
module "unknown" {
  source = "example/service/aws"
  for_each = var.services
}
`,
		"child/main.tf": `resource "example_service" "worker" { count = 2 }`,
	})
	for _, tc := range []struct {
		id     string
		badges []string
	}{
		{"module.child.example_service.worker", []string{"×2", "module child ×3"}},
		{"module.remote", []string{"×4"}},
		{"module.unknown", []string{"×?"}},
	} {
		n := requireNode(t, g, tc.id)
		for _, want := range tc.badges {
			if !slices.Contains(n.Badge, want) {
				t.Errorf("%s badge %q missing from %v", tc.id, want, n.Badge)
			}
		}
	}
}

func TestReferencedVPCTierIsNotPruned(t *testing.T) {
	g := graphFromFiles(t, map[string]string{"main.tf": `
module "vpc" { source = "terraform-aws-modules/vpc/aws" }
resource "aws_s3_bucket" "metadata" {
  tags = { Subnets = join(",", module.vpc.private_subnets) }
}
`})
	tier := requireNode(t, g, "module.vpc.tier.private")
	if !tier.Group || tier.Container != "module.vpc" {
		t.Errorf("referenced tier has wrong container: %#v", tier)
	}
	if n := requireNode(t, g, "aws_s3_bucket.metadata"); n.Container != "global" {
		t.Errorf("ordinary reference incorrectly changes containment: %q", n.Container)
	}
	requireGraphEdge(t, g, "aws_s3_bucket.metadata", "module.vpc.tier.private", 1)
}

func TestInstalledModuleManifestExpandsWithoutInit(t *testing.T) {
	g := graphFromFiles(t, map[string]string{
		"main.tf": `
provider "aws" { region = "eu-west-2" }
module "network" { source = "example/network/aws" }
resource "aws_instance" "web" { subnet_id = module.network.subnet_id }
`,
		".terraform/modules/modules.json": `{"Modules":[{"Key":"","Dir":"."},{"Key":"network","Dir":".terraform/modules/network"}]}`,
		".terraform/modules/network/main.tf": `
resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }
resource "aws_subnet" "public" {
  vpc_id = aws_vpc.main.id
  map_public_ip_on_launch = true
}
output "subnet_id" { value = aws_subnet.public.id }
`,
	})
	if _, collapsed := g.Nodes["module.network"]; collapsed {
		t.Error("installed module incorrectly remained collapsed")
	}
	requireNode(t, g, "module.network.aws_vpc.main")
	if n := requireNode(t, g, "aws_instance.web"); n.Container != "module.network.aws_vpc.main.tier.public" {
		t.Errorf("installed module output containment = %q", n.Container)
	}
}

func TestNestedInstalledModuleNamesContainingModule(t *testing.T) {
	g := graphFromFiles(t, map[string]string{
		"main.tf": `
provider "aws" { region = "eu-west-2" }
module "submodule" { source = "git::https://example.com/outer.git" }
`,
		".terraform/modules/modules.json": `{"Modules":[
  {"Key":"","Dir":"."},
  {"Key":"submodule","Dir":".terraform/modules/submodule"},
  {"Key":"submodule.network_module","Dir":".terraform/modules/submodule.network_module"}
]}`,
		".terraform/modules/submodule/main.tf":                `module "network_module" { source = "git::https://example.com/inner.git" }`,
		".terraform/modules/submodule.network_module/main.tf": `resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }`,
	})
	requireNode(t, g, "module.submodule.module.network_module.aws_vpc.main")
	if _, collapsed := g.Nodes["module.submodule.module.network_module"]; collapsed {
		t.Error("installed nested module remained collapsed")
	}
}

func TestManifestKey(t *testing.T) {
	for id, want := range map[string]string{
		"module.a":                                 "a",
		"module.submodule.module.inner":            "submodule.inner",
		"module.x_module.module.module_y.module.z": "x_module.module_y.z",
	} {
		if got := manifestKey(id); got != want {
			t.Errorf("manifestKey(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestNestedOutputAndInputForwarding(t *testing.T) {
	g := graphFromFiles(t, map[string]string{
		"main.tf": `
provider "aws" { region = "eu-west-2" }
resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }
resource "aws_subnet" "public" {
  vpc_id = aws_vpc.main.id
  map_public_ip_on_launch = true
}
module "wrapper" {
  source = "./wrapper"
  subnet_id = aws_subnet.public.id
}
resource "example_service" "consumer" { instance_id = module.wrapper.instance_id }
`,
		"wrapper/main.tf": `
variable "subnet_id" {}
module "leaf" {
  source = "./leaf"
  subnet_id = var.subnet_id
}
output "instance_id" { value = module.leaf.instance_id }
`,
		"wrapper/leaf/main.tf": `
variable "subnet_id" {}
resource "aws_instance" "web" { subnet_id = var.subnet_id }
output "instance_id" { value = aws_instance.web.id }
`,
	})
	id := "module.wrapper.module.leaf.aws_instance.web"
	if n := requireNode(t, g, id); n.Container != "aws_vpc.main.tier.public" {
		t.Errorf("nested input containment = %q", n.Container)
	}
	requireGraphEdge(t, g, "example_service.consumer", id, 1)
	requireGraphEdge(t, g, id, "aws_subnet.public", 1)
}

func graphFromFiles(t *testing.T, files map[string]string) *Graph {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		writeGraphInput(t, filepath.Dir(path), filepath.Base(path), text)
	}
	g, err := Build(t.Context(), parse.Terraform{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func requireGraphEdge(t *testing.T, g *Graph, from, to string, count int) {
	t.Helper()
	for _, edge := range g.Edges {
		if edge.From == from && edge.To == to {
			if edge.Count != count {
				t.Errorf("%s -> %s count=%d, want %d", from, to, edge.Count, count)
			}
			return
		}
	}
	t.Errorf("missing %s -> %s edge; got %#v", from, to, g.Edges)
}

func TestDottedChildProviderAlias(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	root := `provider "aws" { region="us-east-1" }
provider "aws" {
 alias="west"
 region="us-west-2"
}
module "child" {
 source="./child"
 providers={aws.replica=aws.west}
}`
	sub := `terraform {
 required_providers {
 aws={source="hashicorp/aws", configuration_aliases=[aws.replica]}
 }
}
resource "aws_vpc" "replica" {
 provider=aws.replica
 cidr_block="10.0.0.0/16"
}`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(root), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "main.tf"), []byte(sub), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := Build(context.Background(), parse.Terraform{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.Nodes["module.child.aws_vpc.replica"] == nil {
		t.Fatalf("missing VPC: %v", g.Warnings)
	}
	if got := g.Nodes["module.child.aws_vpc.replica"].Region; got != "us-west-2" {
		t.Fatalf("region %s", got)
	}
}

func TestJSONNestedContainment(t *testing.T) {
	dir := t.TempDir()
	raw := `{"provider":{"aws":{"region":"eu-west-2"}},"resource":{
 "aws_vpc":{"app":{}},
 "aws_subnet":{"private":{"vpc_id":"${aws_vpc.app.id}","tags":{"Name":"private"}}},
 "aws_lambda_function":{"app":{"vpc_config":[{"subnet_ids":["${aws_subnet.private.id}"]}]}},
"aws_lb":{"app":{"subnet_mapping":[{"subnet_id":"${aws_subnet.private.id}"}]}}
 }}`
	if err := os.WriteFile(filepath.Join(dir, "main.tf.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := Build(context.Background(), parse.Terraform{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := g.Nodes["aws_lambda_function.app"].Container; got != "aws_vpc.app.tier.private" {
		t.Fatalf("Lambda placement %s", got)
	}
	if g.Nodes["aws_lb.app"].Container != "aws_vpc.app.tier.private" {
		t.Fatal("JSON LB mapping lost")
	}
	for _, edge := range g.Edges {
		if edge.From == "aws_lambda_function.app" && edge.Count != 1 {
			t.Fatal("nested JSON references double counted")
		}
	}
}

func TestLBNestedSubnetMapping(t *testing.T) {
	dir := t.TempDir()
	raw := `provider "aws" {region="eu-west-2"}
 resource "aws_vpc" "app" {}
 resource "aws_subnet" "private" {
 vpc_id=aws_vpc.app.id
 tags={Name="private"}
 }
 resource "aws_lb" "app" {
 subnet_mapping {subnet_id=aws_subnet.private.id}
 }`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := Build(context.Background(), parse.Terraform{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.Nodes["aws_lb.app"].Container != "aws_vpc.app.tier.private" {
		t.Fatal("LB mapping lost")
	}
}

func TestModuleFanOutStopsAtLoadBudget(t *testing.T) {
	files := map[string]string{"main.tf": `module "c" { source = "./l0" }`}
	for level := 0; level < 12; level++ {
		next := fmt.Sprintf("../l%d", level+1)
		files[fmt.Sprintf("l%d/main.tf", level)] = fmt.Sprintf(`
module "a" { source = %q }
module "b" { source = %q }
module "c" { source = %q }
`, next, next, next)
	}
	files["l12/main.tf"] = `variable "unused" {}`
	dir := t.TempDir()
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		writeGraphInput(t, filepath.Dir(path), filepath.Base(path), text)
	}
	_, err := Build(t.Context(), parse.Terraform{}, dir)
	if !errors.Is(err, errLimit) {
		t.Fatalf("3^12 module fan-out error = %v, want load limit", err)
	}
}

func TestSeparateInternetRouteMakesSubnetPublic(t *testing.T) {
	g := graphFromFiles(t, map[string]string{"main.tf": `
provider "aws" { region = "eu-west-2" }
resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }
resource "aws_internet_gateway" "igw" { vpc_id = aws_vpc.main.id }
resource "aws_route_table" "edge" { vpc_id = aws_vpc.main.id }
resource "aws_route" "default" {
  route_table_id         = aws_route_table.edge.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.igw.id
}
resource "aws_subnet" "edge" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "private-looking" }
}
resource "aws_route_table_association" "edge" {
  subnet_id      = aws_subnet.edge.id
  route_table_id = aws_route_table.edge.id
}
`})
	if n := requireNode(t, g, "aws_subnet.edge"); n.Container != "aws_vpc.main.tier.public" {
		t.Errorf("subnet behind aws_route to IGW = %q, want public tier", n.Container)
	}
}

func TestVPCModuleCIDROutputsDoNotPlaceResources(t *testing.T) {
	g := graphFromFiles(t, map[string]string{"main.tf": `
provider "aws" { region = "eu-west-2" }
module "vpc" { source = "terraform-aws-modules/vpc/aws" }
resource "aws_lb" "web" { subnets = module.vpc.public_subnets }
resource "aws_s3_bucket" "audit" {
  tags = { private = module.vpc.private_subnets_cidr_blocks[0] }
}
`})
	if n := requireNode(t, g, "aws_lb.web"); n.Container != "module.vpc.tier.public" {
		t.Errorf("public_subnets placement = %q", n.Container)
	}
	for _, edge := range g.Edges {
		if edge.From == "aws_s3_bucket.audit" && edge.To != "module.vpc" {
			t.Errorf("CIDR output resolved to %s, want the module itself", edge.To)
		}
	}
	requireGraphEdge(t, g, "aws_s3_bucket.audit", "module.vpc", 1)
	if _, ok := g.Nodes["module.vpc.tier.private"]; ok {
		t.Error("CIDR reference kept an otherwise empty private tier")
	}
}

func TestSubnetNameTagWords(t *testing.T) {
	for name, want := range map[string]string{
		"app-public-a":              "public",
		"private_data":              "private",
		"Private.Workers":           "private",
		"private-nat-public-facing": "subnets",
		"republican":                "subnets",
		"privateer":                 "subnets",
		"":                          "subnets",
	} {
		g := graphFromFiles(t, map[string]string{"main.tf": fmt.Sprintf(`
provider "aws" { region = "eu-west-2" }
resource "aws_vpc" "main" { cidr_block = "10.0.0.0/16" }
resource "aws_instance" "x" { subnet_id = aws_subnet.s.id }
resource "aws_subnet" "s" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = %q }
}
`, name)})
		if got := requireNode(t, g, "aws_subnet.s").Container; got != "aws_vpc.main.tier."+want {
			t.Errorf("Name %q placed in %q, want tier %s", name, got, want)
		}
	}
}

func TestNonAWSProvidersStayOutOfAWSRegions(t *testing.T) {
	g := graphFromFiles(t, map[string]string{"main.tf": `
provider "aws" { region = "eu-west-1" }
provider "google" { alias = "west" }
resource "google_compute_instance" "a" { provider = google.west }
module "gcp" { source = "terraform-google-modules/network/google" }
module "eks" { source = "terraform-aws-modules/eks/aws" }
module "private" { source = "app.terraform.io/acme/queue/aws//modules/sqs" }
module "git" { source = "git::https://example.com/acme/terraform-aws-cache.git?ref=v1" }
`})
	for id, want := range map[string]string{
		"google_compute_instance.a": "",
		"module.gcp":                "",
		"module.eks":                "region:eu-west-1",
		"module.private":            "region:eu-west-1",
		"module.git":                "region:eu-west-1",
	} {
		if got := requireNode(t, g, id).Container; got != want {
			t.Errorf("%s container = %q, want %q", id, got, want)
		}
	}
	for _, warning := range g.Warnings {
		if strings.Contains(warning, "google_compute_instance.a: unresolved provider region") {
			t.Errorf("non-AWS resource warning: %s", warning)
		}
	}
}

func TestManyCallsToOneModuleStayWithinBudget(t *testing.T) {
	var root strings.Builder
	root.WriteString(`provider "aws" { region = "eu-west-2" }` + "\n")
	for i := 0; i < 1100; i++ {
		fmt.Fprintf(&root, "module \"m%d\" { source = \"./m\" }\n", i)
	}
	g := graphFromFiles(t, map[string]string{"main.tf": root.String(), "m/main.tf": `resource "aws_sqs_queue" "q" {}`})
	requireNode(t, g, "module.m1099.aws_sqs_queue.q")
}

func TestAWSModuleSources(t *testing.T) {
	for src, want := range map[string]bool{
		"terraform-aws-modules/vpc/aws":                                    true,
		"terraform-aws-modules/eks/aws//modules/karpenter":                 true,
		"app.terraform.io/acme/queue/aws":                                  true,
		"git::https://example.com/acme/terraform-aws-cache.git?ref=v1":     true,
		"github.com/acme/terraform-aws-cache":                              true,
		"git@github.com:acme/terraform-aws-cache.git":                      true,
		"git::https://example.com/acme/terraform-aws-cache.git//sub?ref=1": true,
		"terraform-google-modules/network/google":                          false,
		"github.com/acme/aws":                                              false,
		"gitlab.com/acme/aws":                                              false,
		"terraform-google-modules/x/google//modules/terraform-aws-peer":    false,
		"git::https://example.com/acme/platform.git//terraform-aws-peer":   false,
	} {
		if got := awsModule(src); got != want {
			t.Errorf("awsModule(%q) = %v, want %v", src, got, want)
		}
	}
}
