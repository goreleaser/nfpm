#!/bin/bash
set -euo pipefail
curl -sSf -H "Authorization: Bearer $GITHUB_TOKEN" "https://api.github.com/repos/goreleaser/nfpm/releases/latest" |
	jq -r '.tag_name' >./www/static/latest
