#!/bin/bash
set -euo pipefail
pip install -U mkdocs-material mkdocs-minify-plugin lunr
version="$(curl -sSf -H "Authorization: Bearer $GITHUB_TOKEN" "https://api.github.com/repos/goreleaser/nfpm/releases/latest" | jq -r '.tag_name')"
sed -s'' -i "s/__VERSION__/$version/g" www/docs/install.md
mkdocs build -f www/mkdocs.yml
