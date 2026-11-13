#!/bin/sh
# A non-ASCII literal in the script itself must reach the installed system
# unchanged.
mkdir -p /private/tmp/nfpm-pkg-test
printf '%s\n' 'héllo-世界-Привет' > /private/tmp/nfpm-pkg-test/postinstall-unicode
