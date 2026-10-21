# Validation record

Local validation completed on 2026-10-21. No remote CI or release result is claimed.

| Check | Result |
| --- | --- |
| `go test ./...` | Pass |
| `go test -race ./...` | Pass |
| Parser fuzzing, 30 seconds, 4 workers | Pass; 384,270 executions |
| `golangci-lint run` (2.15.0) | No issues |
| `golangci-lint fmt --diff` | No diff |
| `go mod tidy` followed by module diff check | No diff |
| `goreleaser check` (2.18.3) | Pass |
| Snapshot binaries and archives | Linux/macOS/Windows × amd64/arm64 |
| Archive SHA256 and required notices | Verified for six archives |
| SVG fixture visual review | Six required fixtures plus synthetic plan overlay; no overlaps/clipping |
| Offline/icon failures | Mocked transport tests cover zero requests, cache reuse and fallback |
| Hosted CI, Homebrew and Action smoke jobs | Pending GitHub write access |

Each feature received an independent review, with at most two rounds. Final
whole-repository review found no remaining high-severity defects. Resolved findings
include literal D2 quoting, hidden data-source parents, non-AWS placement, plan
forget actions and sequence validation, repeated Action artifact names, nested
load-balancer subnet mappings, and icon CSS validation.

SVG visual checks used raster previews with the SVG's actual embedded fonts.
An initial fallback-font preview suggested clipping; this was not evidence of
browser SVG clipping. Final previews showed no clipping. No browser or hosted
GitHub rendering test was available.

## Differences from the original brief

- The confirmed organization is `Infraedgecloud-tools`; module and repository
  references use that name.
- Bounded reads feed `ParseHCL`/`ParseJSON` instead of unbounded file convenience
  methods. Files are limited to 16 MiB and nonregular inputs are rejected.
- D2 0.9 uses native Go ELK. Release notices include its EPL and upstream notices,
  plus embedded fonts and other assets.
- The Action adds an artifact download link to the summary. GitHub artifact URLs
  are not inline image URLs.
- Sequential branches and commits are local. `main` was not changed. API writes
  returned HTTP 403, so no PRs, merges, branch protection, tags, tap updates or
  releases were created.

For the remaining publication gates, see [releasing](releasing.md).
