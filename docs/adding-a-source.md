# Adding a source adapter

Terraform/OpenTofu is the only implemented source. Other IaC formats are extension work, not supported inputs. Non-AWS Terraform resources appear as ungrouped plain shapes; this does not imply cloud-specific containment support.

The internal contract is:

```go
type Source interface {
    Load(context.Context, string) (Result, error)
}
```

`Result` contains `Blocks` and recoverable `Warnings`. A block has a kind, labels, attributes and source filename. Attributes carry literal values, raw expressions, references and optional provider bindings. `Load` reads one directory; module traversal belongs to the graph builder.

The current intermediate representation uses Terraform-style declarations. A new adapter must deliberately normalise to that representation or extend the model and graph together. Implementing `Source` alone does not make arbitrary CloudFormation, Pulumi, CDK or Kubernetes concepts render correctly.

## Minimal adapter shape

This example reads a tiny, already-normalised JSON inventory. It demonstrates the boundary; it is not an adapter for another IaC platform. Place it inside this module because the shared packages are internal.

```go
package inventory

import (
    "context"
    "encoding/json"
    "fmt"
    "path/filepath"

    "github.com/Infraedgecloud-tools/edgemap/internal/source"
)

type Adapter struct{}

var _ source.Source = Adapter{}

func (Adapter) Load(ctx context.Context, dir string) (source.Result, error) {
    if err := ctx.Err(); err != nil {
        return source.Result{}, err
    }
    path := filepath.Join(dir, "inventory.json")
    data, err := source.ReadRegularFile(path)
    if err != nil {
        return source.Result{}, fmt.Errorf("read inventory: %w", err)
    }
    var input struct {
        Resources []struct{ Type, Name string }
    }
    if err := json.Unmarshal(data, &input); err != nil {
        return source.Result{}, fmt.Errorf("invalid inventory JSON")
    }
    result := source.Result{}
    for _, resource := range input.Resources {
        if resource.Type == "" || resource.Name == "" {
            result.Warnings = append(result.Warnings, "resource lacks type or name")
            continue
        }
        result.Blocks = append(result.Blocks, source.Block{
            Kind: "resource", Labels: []string{resource.Type, resource.Name},
            File: path, Attributes: map[string]source.Attribute{},
        })
    }
    return result, nil
}
```

A production adapter must additionally validate identifier uniqueness and the native format's structure, observe cancellation during long loops, and provide references and metadata appropriate to that source. Do not copy executable provider logic or silently treat uncertain values as literal.

## Mapping requirements

| Model field | Required interpretation |
| --- | --- |
| `Kind` and `Labels` | Resource/data blocks use type and name; modules/outputs/providers use their existing graph conventions |
| `Attribute.References` | Addresses of referenced declarations; direction remains consumer → dependency |
| `Attribute.Literal` / `Value` | Only values established statically; retain uncertainty rather than guessing |
| `Attribute.Raw` | Diagnostic/context input, not a general-purpose rendered label |
| `Attribute.Bindings` | Explicit provider mappings where the input format has equivalent semantics |
| `Result.Warnings` | Safe summaries of recoverable incompleteness; no embedded secrets or full source excerpts |

Use `source.ReadRegularFile` for bounded regular-file input. Do not invoke interpreters, cloud APIs, package downloads or credential discovery from `Load`. Resource grouping belongs in a provider catalog and the graph stage, not in parsing or SVG generation. Return the full source graph input: display filtering happens later, so adapters must not discard IAM, data or security-group declarations. Terraform plan annotation is a separate metadata-only path; another source must not feed arbitrary values into it under the guise of equivalent plan support.

Wire source selection into the CLI explicitly. Add fixture tests for nesting, references, unknown expressions, malformed input and module equivalents. Extend D2 golden tests only after graph expectations are independently asserted. Keep unsupported native concepts visible in limitations; do not advertise a source as supported merely because parsing succeeds.
