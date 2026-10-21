#!/usr/bin/env bash
set -euo pipefail
# Run from the repository root. Pin the notice generator independently of go.mod.
notice_dir=THIRD_PARTY_LICENSES
rm -rf "$notice_dir"
go run github.com/google/go-licenses/v2@v2.0.1 save ./cmd/edgemap --save_path="$notice_dir" --ignore=github.com/Infraedgecloud-tools/edgemap,github.com/alecthomas/chroma/v2,github.com/d2lang/d2/d2layouts/d2elklayout
# D2 embeds fonts and layout assets that a Go-import scanner cannot discover.
d2_dir=$(go list -m -f '{{.Dir}}' github.com/d2lang/d2)
elk_dir=$(go list -m -f '{{.Dir}}' github.com/d2lang/elk-go)
mkdir -p "$notice_dir/embedded/d2" "$notice_dir/embedded/elk"
for asset in THIRD_PARTY_NOTICES.txt d2renderers/d2fonts/NOTICE.txt d2renderers/d2fonts/NotoColorEmoji-OFL.txt d2layouts/d2elklayout/NOTICE.txt d2layouts/d2dagrelayout/NOTICE.txt d2renderers/d2sketch/NOTICE.txt d2renderers/d2svg/NOTICE.txt lib/textmeasure/NOTICE.txt; do
  target=${asset//\//_}
  cp "$d2_dir/$asset" "$notice_dir/embedded/d2/$target"
done
# D2 0.9 uses the native Go ELK port, not elk.js; retain its EPL and notices.
for asset in LICENSE LICENSE.apache-2.0 LICENSE.v8-bsd NOTICE.md NOTICE.eclipse-elk.md; do
  cp "$elk_dir/$asset" "$notice_dir/embedded/elk/$asset"
done

# Mixed MIT/OFL and EPL notices confuse the classifier; copy these explicitly.
chroma_dir=$(go list -m -f '{{.Dir}}' github.com/alecthomas/chroma/v2)
mkdir -p "$notice_dir/embedded/chroma"
cp "$chroma_dir/COPYING" "$notice_dir/embedded/chroma/COPYING"
