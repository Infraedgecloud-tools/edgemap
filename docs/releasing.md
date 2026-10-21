# Releasing

## Current status

Local snapshot builds and archive packaging passed for Linux, macOS and Windows on amd64/arm64. Archive checksums and embedded licence notices were checked, and `goreleaser check` accepts the current configuration. This is not a published release or proof of installation on those platforms.

Upstream writes returned HTTP 403 during this build session. PR publication/merging, `main` protection, the release-candidate tag, Homebrew smoke tests and the final release remain blocked. No release, tap installation or successful remote CI run should be inferred from the local files.

## Prerequisites

- Restore write access to `Infraedgecloud-tools/edgemap`, publish the sequential branches/PRs, review them and confirm required CI is green before merging. Protect `main` with required checks and no required approvals, as specified by the build brief.
- Create or grant access to `Infraedgecloud-tools/homebrew-tap`.
- Store a fine-grained `TAP_TOKEN` Actions secret with repository **Contents: write** access scoped to the tap. The workflow's `GITHUB_TOKEN` has Contents: write only for the source repository; it cannot publish to the separate tap.
- Use the versions pinned in CI: Go 1.27.3 and GoReleaser 2.18.3. The release workflow runs race-enabled tests before GoReleaser.

## Local checks

Run from the repository root:

```sh
go mod tidy
git diff --exit-code -- go.mod go.sum
go test -race ./...
golangci-lint run
golangci-lint fmt --diff
goreleaser check
goreleaser build --snapshot --clean
```

A snapshot build verifies binaries, not complete archives or tap installation. To exercise local archive/checksum generation without publishing:

```sh
goreleaser release --snapshot --clean --skip=publish,validate
```

The configured `before` hook runs `scripts/licenses.sh`. It generates `THIRD_PARTY_LICENSES` with pinned `go-licenses`, then adds embedded font, layout and other notices the Go import scanner cannot discover. D2 0.9 uses the native Go ELK port; its EPL and additional notices are copied explicitly. Check that every archive includes this directory, `LICENSE`, `README.md` and the appropriate binary. Check archives against `dist/checksums.txt` and smoke-test a host-compatible binary. Cross-compilation alone does not validate runtime behavior on another OS/architecture.

## Release candidate, then final

After reviewed changes reach protected, green `main`, tag that exact commit as `v0.1.0-rc.1` and push the tag. `.github/workflows/release.yml` runs on `v*` tags and invokes `goreleaser release --clean`; prerelease status is inferred from the tag.

The configuration publishes a Homebrew cask into the tap's `Casks` directory. Its post-install quarantine removal hook runs only on macOS for these unsigned binaries. The dependent `brew-smoke` matrix runs on macOS and Linux and executes (with `EXPECTED_VERSION` set to the triggering tag):

```sh
brew install --cask Infraedgecloud-tools/tap/edgemap
test "$(edgemap version)" = "${EXPECTED_VERSION#v}"
edgemap render testdata/multi-az --no-icons
test -s diagram.svg
```

Treat both matrix results as release gates. If casks fail on Linux, evaluate the pinned GoReleaser version's supported formula-generation options, change the tap configuration and smoke command together, and publish another candidate. Do not tag the final release with an unverified Linux installation path.

Test `action.yml` against the candidate using an explicit `version: v0.1.0-rc.1`: it downloads the matching archive, checks its checksum, renders, uploads the SVG and links the artifact in the job summary. The Action's default `v0.1.0` is unavailable until the final release exists. Do not advertise the default invocation before then.

Only after candidate assets, notices, checksums, Homebrew on both runners, Action behavior and rendered examples pass should the same reviewed source receive `v0.1.0`. Record the actual release URL and platform test results. A pushed tag or generated workflow is not completion evidence.
