#!/bin/sh
mkdir -p /private/tmp/nfpm-pkg-test
printf 'uid=%s target=%s\n' "$(id -u)" "$3" > /private/tmp/nfpm-pkg-test/postinstall
