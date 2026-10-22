#!/usr/bin/env bash
set -euo pipefail
# Run from the repository root. Pin the notice generator independently of go.mod.
notice_dir=THIRD_PARTY_LICENSES
rm -rf "$notice_dir"
go run github.com/google/go-licenses/v2@v2.0.1 save ./cmd/edgemap --save_path="$notice_dir" --ignore=github.com/Infraedgecloud-tools/edgemap,github.com/alecthomas/chroma/v2,github.com/d2lang/d2/d2layouts/d2elklayout

# go-licenses copies the full source of MPL-2.0 modules (d2, hcl, elk-go): over
# 100 MB. MPL-2.0 section 3.2(a) allows telling recipients where the source is
# instead, so keep only licence and notice files and list exact sources below.
find "$notice_dir" -type f ! \( -iname 'LICEN[CS]E*' -o -iname 'COPYING*' -o -iname 'NOTICE*' -o -iname 'PATENTS*' -o -iname 'AUTHORS*' -o -iname 'THIRD_PARTY*' -o -iname '*-OFL*' -o -iname 'COPYRIGHT*' \) -delete
find "$notice_dir" -type d -empty -delete
{
  echo "# Source code of bundled modules"
  echo
  echo "edgemap is built from these exact module versions. Each module's complete"
  echo "source, including MPL-2.0 modules (github.com/d2lang/d2, github.com/d2lang/elk-go,"
  echo "github.com/hashicorp/hcl/v2), is available from its repository and from the Go"
  echo "module proxy as https://proxy.golang.org/<module>/@v/<version>.zip, where each"
  echo "upper-case letter in <module> is written as ! and its lower-case form."
  echo "edgemap's own source is at https://github.com/Infraedgecloud-tools/edgemap."
  echo
  go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}' ./cmd/edgemap |
    sort -u | grep -v '^github.com/Infraedgecloud-tools/edgemap ' | sed 's/^/- /'
} >"$notice_dir/SOURCES.md"

# The Go standard library is compiled into every binary.
mkdir -p "$notice_dir/go"
cp "$(go env GOROOT)/LICENSE" "$notice_dir/go/LICENSE"

# D2 embeds fonts and layout assets that a Go-import scanner cannot discover.
d2_dir=$(go list -m -f '{{.Dir}}' github.com/d2lang/d2)
elk_dir=$(go list -m -f '{{.Dir}}' github.com/d2lang/elk-go)
mkdir -p "$notice_dir/embedded/d2" "$notice_dir/embedded/elk"
for asset in THIRD_PARTY_NOTICES.txt d2renderers/d2fonts/NOTICE.txt d2renderers/d2fonts/NotoColorEmoji-OFL.txt d2layouts/d2elklayout/NOTICE.txt d2layouts/d2dagrelayout/NOTICE.txt d2renderers/d2sketch/NOTICE.txt d2renderers/d2svg/NOTICE.txt lib/textmeasure/NOTICE.txt; do
  target=${asset//\//_}
  cp "$d2_dir/$asset" "$notice_dir/embedded/d2/$target"
done
echo "File names flatten D2's module paths (d2layouts/d2dagrelayout/NOTICE.txt is
d2layouts_d2dagrelayout_NOTICE.txt). References to THIRD_PARTY_NOTICES.txt mean the
copy in this directory." >"$notice_dir/embedded/d2/README.txt"

# D2's font NOTICE links the OFL without its text or the font copyrights. The
# copyright lines below are the fonts' own name-table entries.
{
  echo "Fonts embedded through github.com/d2lang/d2:"
  echo
  echo "Source Sans Pro: © 2010 - 2018 Adobe Systems Incorporated (http://www.adobe.com/), with Reserved Font Name 'Source'."
  echo "Source Code Pro: © 2010 - 2020 Adobe Systems Incorporated (http://www.adobe.com/), with Reserved Font Name 'Source'."
  echo "Fuzzy Bubbles: Copyright 2005 The Fuzzy Bubbles Project Authors (https://github.com/googlefonts/fuzzy-bubbles)"
  echo
  echo "These fonts are licensed under the SIL Open Font License, Version 1.1:"
  echo
  sed -n '/^-*$/,$p' "$d2_dir/d2renderers/d2fonts/NotoColorEmoji-OFL.txt" | sed '1,/SIL OPEN FONT LICENSE/{/SIL OPEN FONT LICENSE/!d}'
} >"$notice_dir/embedded/d2/fonts-OFL.txt"

# D2 0.9 uses the native Go ELK port, not elk.js; retain its EPL and notices.
for asset in LICENSE LICENSE.apache-2.0 LICENSE.v8-bsd NOTICE.md NOTICE.eclipse-elk.md; do
  cp "$elk_dir/$asset" "$notice_dir/embedded/elk/$asset"
done

# Mixed MIT/OFL and EPL notices confuse the classifier; copy these explicitly.
chroma_dir=$(go list -m -f '{{.Dir}}' github.com/alecthomas/chroma/v2)
mkdir -p "$notice_dir/embedded/chroma"
cp "$chroma_dir/COPYING" "$notice_dir/embedded/chroma/COPYING"
