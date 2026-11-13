#!/bin/sh
# Leaves a marker before failing, so the test can tell "this script stopped
# the installation" from "the installation failed for another reason".
mkdir -p /private/tmp/nfpm-pkg-test
echo PREINSTALL_FAILURE_EXECUTED > /private/tmp/nfpm-pkg-test/preinstall-fail
exit 1
