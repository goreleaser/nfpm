#!/bin/sh
set -e
rm -rf manpages
mkdir manpages
go run ./cmd/nfpm/ man | gzip -c -9 >manpages/nfpm.1.gz
