#!/bin/sh

cleanInstall() {
    printf "\033[32m Post Install of a clean install\033[0m\n"
    echo "Install" > /tmp/preinstall-proof
}

upgrade() {
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    echo "Upgrade" > /tmp/preinstall-proof
}

action="$1"

case "$action" in
  "1" | "install")
    cleanInstall
    ;;
  "2" | "upgrade")
    upgrade
    ;;
  *)
    printf "\033[32m Alpine\033[0m"
    cleanInstall
    ;;
esac
