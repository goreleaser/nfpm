#!/bin/sh
# xbps invokes lifecycle scripts as: ACTION PKGNAME VERSION UPDATE CONF ARCH
# ($4 is "yes" on upgrade, "no" on first install). Record the arguments so the
# acceptance scenarios can assert that nFPM forwards them unchanged.
echo "Install" > /tmp/postinstall-proof
printf '%s\n' "$#" "$1" "$2" "$3" "$4" > /tmp/postinstall-args
