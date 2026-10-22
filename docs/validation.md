# Validation record

This records what was verified for `v0.1.0-rc.1` (`main` at `4571514`, 2026-10-22).
Release mechanics are in [releasing](releasing.md).

## Hosted

| Check | Result |
| --- | --- |
| `verify` on every pull request (#1, #2, #7–#16) and on `main` | Pass |
| Release run [39705613006](https://github.com/Infraedgecloud-tools/edgemap/actions/runs/39705613006) | Pass |
| Homebrew cask install, `edgemap version`, render (`macos-latest`, `ubuntu-latest`) | Pass; the binary was linked into `/opt/homebrew/bin` and `/home/linuxbrew/.linuxbrew/bin` |
| Action, three calls per runner (`ubuntu-latest`, `macos-latest`, `windows-latest`) | Pass; 9 uniquely named artifacts; one call per runner uses default inputs and live icon fetches |
| Public tap after the candidate | Unchanged (empty); candidates do not publish casks |

The `verify` job runs the following:
- `go mod tidy` diff check;
- `go test -race ./...`;
- golangci-lint 2.15.0 and its formatter check;
- 30 s of `FuzzParse`;
- a `two-tier` render smoke test;
- a six-target GoReleaser 2.18.3 snapshot build.

## Local (linux/arm64, 2 CPUs)

| Check | Result |
| --- | --- |
| `go test ./...`, `go test -race ./...` | Pass |
| `golangci-lint run`, `golangci-lint fmt --diff`, `go mod tidy` diff | Clean |
| `FuzzParse`, 30 s, default worker count (2) | Pass; 61,402 executions |
| `goreleaser check`; `release --snapshot --skip=publish,validate` | Pass. Two clean clones gave identical `checksums.txt` |
| Every commit in each fix PR | Tests, lint and format pass on their own |
| Published candidate assets | All six match `checksums.txt`. Each archive has `LICENSE`, `README.md`, the binary and 71 notice files |
| Extracted `linux_arm64` binary | `version` prints `0.1.0-rc.1`; renders `multi-az` with icons |
| `go install …/cmd/edgemap@v0.1.0-rc.1` | Builds; `version` prints `0.1.0-rc.1` |
| Visual review in Chromium | Every fixture and the plan overlay with ELK and icons, and with Dagre and no icons. No overlaps or clipping; no requests beyond local files |
| SVG determinism | The same render from two cache directories is byte-identical. `scripts/examples.sh` reproduces the committed examples |

## Review

Independent reviewers covered three areas before the fixes: parser and graph, renderer and plan, and release and security. Each finding was reproduced before it was fixed. A separate reviewer then read each fix PR before merge. No High or Medium findings remained after the second read. The fix PRs (#14–#16) list each defect and its regression test.

## Not verified

- Windows runtime outside the Action smoke test; arm64 Windows and Intel macOS binaries are built but not run.
- Homebrew installation from the public tap, until `v0.1.0` publishes a cask.
- `d2` CLI rendering of the written `.d2` files with remote icon URLs.
- Rendering quality on large real-world configurations; the fixtures are small and synthetic.
