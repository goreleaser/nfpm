#!/bin/sh
set -e
rm -rf manpages
mkdir manpages
go run ./cmd/nfpm/ man | gzip -c >manpages/nfpm.1.gz
