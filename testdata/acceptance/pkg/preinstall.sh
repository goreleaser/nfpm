#!/bin/sh
# Records that the script ran, who ran it and what the installer passed in.
mkdir -p /private/tmp/nfpm-pkg-test
printf 'uid=%s target=%s\n' "$(id -u)" "$3" > /private/tmp/nfpm-pkg-test/preinstall
