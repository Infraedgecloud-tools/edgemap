# Architecture

Edgemap describes declared infrastructure from local files. It does not run Terraform, initialise modules, call cloud APIs or verify deployed state. A diagram is a static approximation: unresolved expressions remain unresolved, and references are not evidence that network traffic flows between two resources.

## Boundaries

| Package | Responsibility |
| --- | --- |
| `internal/source` | Adapter contract, declaration types and bounded regular-file reads |
| `internal/parse` | Terraform HCL/JSON declarations, literal values, expression references and diagnostics |
| `internal/graph` | Scoped addresses, module inputs/outputs, provider regions, containment and reference edges |
| `internal/catalog` | AWS service metadata and attributes that establish containment |
| `internal/plan` | Metadata-only plan decoding, instance counts and action annotations |
| `internal/filter` | Display-only IAM/data/security-group filtering and badges |
| `internal/render` | Deterministic D2 emission, layouts, SVG and icon handling |

The pipeline builds the full graph, applies optional plan metadata, filters display noise, then renders. Filtering copies the graph; it does not change source discovery or the graph used by `inspect`.

## Parsing and resolution

`Source.Load(ctx, dir)` reads one directory. The Terraform adapter accepts `.tf` and `.tf.json`; the graph builder decides when to load child module directories. HCL nested and dynamic blocks retain their attributes with nested paths. Dynamic blocks are inspected, not expanded.

Attributes keep raw expressions, traversals and, when available, literal values. There is no Terraform expression evaluator: functions, conditions, variable values, tfvars and workspace state do not decide the architecture. Only direct references to literal locals are substituted. A literal `count` produces a count badge; unresolved counts and `for_each` remain a single node with an unknown-multiplicity badge. Index keys are omitted from reference addresses because instances are collapsed.

Local modules expand recursively. Resource addresses include their module scope. Inputs forward resource references into child variables; declared outputs forward references back to callers, including nested modules. Explicit `providers` mappings select the child provider region. Module `depends_on` applies to expanded child resources; a dependency on an expanded module resolves to its resource nodes. Module multiplicity is retained in child badges rather than instantiated.

Registry/git modules expand only when an existing `.terraform/modules/modules.json` supplies their directory. Otherwise they remain collapsed with source/version metadata. Recognised AWS VPC modules receive public/private tier containers, and references to their subnet outputs resolve to those tiers. This special case does not evaluate the module's implementation.

## Placement and edges

Containment uses AWS attribute semantics, not arbitrary dependency edges:

- Provider region → VPC → public/private/unknown subnet tier → resource.
- Public tiers use public-IP auto-assignment, an explicit default route to an internet gateway, or a `Name` tag; private tags identify private tiers. Missing evidence becomes `subnets`.
- Resources spanning subnets in one tier sit in that tier. Resources spanning public and private tiers sit in their common VPC.
- IAM, Route53, CloudFront and S3 use `Global`, as a display convention. Other unknown AWS types use their region's `Other` group. Non-AWS resources remain ungrouped plain nodes, without inferred AWS placement. A collapsed module is placed in an AWS region only when its source is evidently AWS: a registry address whose provider segment is `aws` (`namespace/name/aws`, optionally behind a registry hostname), or a repository named `terraform-aws-*`.
- Non-literal regions retain their expression label. Accounts are never inferred.

Edges point from a referencing resource to the resource it references. Multiple references share one edge with a count. Edges to a containing ancestor are omitted because nesting already expresses that relationship. Referenced containers survive pruning even when they have no contained resources.

## Plans and filtering

`--plan` accepts local `terraform show -json` output. Typed decoding retains only `resource_changes[].address`, `.type` and `.change.actions`; `before`, `after` and other values are skipped, not copied into the graph. The CLI reads the bounded plan bytes in memory. Plans are never uploaded, and diagnostics avoid echoing rejected input values.

Instance keys collapse onto their resource address. Badges count records and actions from the supplied plan: pure deletes or `forget` entries are absent from the after-plan count, while replacements count in both create and destroy. Mixed instances use destroy-red over change-amber over create-green. `forget` is grey and explicitly means leaving infrastructure in place, not destroying it. Collapsed modules show aggregate plan-resource counts. Resources absent from configuration appear in a `Plan-only · placement unknown` container; values are not inspected to invent placement. A partial plan only supplies counts for the records it contains.

By default, IAM, data sources, security groups and security-group rules are hidden. `--show-iam`, `--show-data` and `--show-sg` restore their nodes and reference edges. References through hidden IAM/SG nodes become deduplicated badges on visible consumers. A data-source VPC (`data "aws_vpc"`) stays drawn as a container, labelled as a data source, while it holds visible nodes, and is pruned when empty. Edges with hidden endpoints are removed; arbitrary new dependency edges are not invented. Empty synthetic containers are pruned, while visible real VPCs remain.

## Rendering and I/O

The renderer emits nodes in sorted address order and quotes labels. D2 identifiers are derived from addresses so Terraform names cannot become D2 syntax. SVG uses the embedded D2 ELK or Dagre layout; the D2 text is the stable golden-test boundary. SVG bytes are not promised to be stable across dependency upgrades.

The icon path fetches catalog-selected HTTPS assets, caches them beneath `os.UserCacheDir()/edgemap`, and bundles local images into SVG. Fetch or icon-render failures fall back to plain shapes with warnings. `--no-icons` disables icon requests. Cached paths in an icon-enabled `.d2` file are machine-local; the bundled SVG is the portable output.

Configuration files, the module manifest and plan input are limited to 16 MiB each. Plan action sequences are validated before annotation. Graph construction and plan overlay have a 10,000-node guardrail; module expansion is bounded by nesting depth. Non-regular files and file symlinks are skipped with warnings. Module directories are resolved separately; directory cycles and excessive nesting are diagnosed. These checks are not a filesystem sandbox: local module paths can reference directories outside the root.

Invalid syntax yields warnings and any recoverable declarations, not a claim that the resulting diagram is complete. Diagnostic summaries omit parser detail that could contain values. Raw expressions remain in memory; resource names, module sources and other displayed metadata can still be sensitive.
