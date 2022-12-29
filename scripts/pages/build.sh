#!/bin/bash
set -euo pipefail
pip install -U pip
pip install -U mkdocs-material mkdocs-minify-plugin lunr
version="$(cat ./www/docs/static/latest)"
sed -s'' -i "s/__VERSION__/$version/g" www/docs/install.md
mkdocs build -f www/mkdocs.yml
