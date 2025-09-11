#!/bin/sh
set -e

SED="sed"
if which gsed >/dev/null 2>&1; then
	SED="gsed"
fi

mkdir -p www/content/docs/cmd
rm -rf www/content/docs/cmd/nfpm*.md
go run ./cmd/nfpm docs

"$SED" -E -i'' \
	-e 's/^## (.*)/---\ntitle: \1\n---/' \
	-e 's/SEE ALSO/See also/g' \
	-e 's/^## /# /g' \
	-e 's/^### /## /g' \
	-e 's/^#### /### /g' \
	-e 's/^##### /#### /g' \
	./www/content/docs/cmd/*.md
