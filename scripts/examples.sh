#!/usr/bin/env bash
set -euo pipefail
# Regenerate docs/examples from the fixtures. Run from the repository root.
# Icons are fetched once into the user cache; the SVGs embed them as data URIs.
go build -o "${TMPDIR:-/tmp}/edgemap-examples" ./cmd/edgemap
bin="${TMPDIR:-/tmp}/edgemap-examples"
for fixture in two-tier multi-az local-modules vpc-module for-each broken existing-vpc; do
  "$bin" render "testdata/$fixture" --out "docs/examples/$fixture.svg"
done
# A synthetic plan (addresses, types and actions only) drives the overlay example.
"$bin" render testdata/two-tier --plan testdata/plans/two-tier-overlay.json --out docs/examples/plan-overlay.svg
rm -f docs/examples/*.d2 "$bin"
