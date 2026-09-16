#!/bin/bash
set -euo pipefail

# mise
eval "$(mise activate bash)"
mise install

# Exercise the go install path: build into an isolated GOBIN, then run the
# resulting binary's --version and --help, which need no network, to
# validate end-to-end install.
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

GOBIN="$TMP" go install ./...
"$TMP/eolwhen" --version
"$TMP/eolwhen" --help | grep -q '^Usage:'
