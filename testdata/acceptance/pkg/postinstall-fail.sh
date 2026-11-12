#!/bin/sh
# Leaves a marker before failing, so the test can tell "this script failed
# the installation" from "the installation failed for another reason".
mkdir -p /private/tmp/nfpm-pkg-test
echo POSTINSTALL_FAILURE_EXECUTED > /private/tmp/nfpm-pkg-test/postinstall-fail
exit 1
