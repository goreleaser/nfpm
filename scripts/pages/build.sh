#!/bin/bash
set -e

version="$(cat ./www/static/latest)"
sed -s'' -i "s/__VERSION__/$version/g" www/content/docs/install.md
cd www
hugo --minify
