#!/bin/sh
# A post-remove hook whose last command fails without calling exit. xbps
# reports a non-zero post-remove status as a failed transaction; the
# acceptance scenario asserts that nFPM's generated directory cleanup, which
# runs after this hook, does not hide that status.
echo "Remove" > /tmp/postremove-proof
false
