#!/bin/sh

remove() {
    printf "\033[32m Pre Remove of a normal remove\033[0m\n"
    echo "Remove" > /tmp/preremove-proof
}

upgrade() {
    printf "\033[32m Pre Remove of an upgrade\033[0m\n"
    echo "Upgrade" > /tmp/preremove-proof
}

action="$1"

case "$action" in
  "0" | "remove")
    remove
    ;;
  "1" | "upgrade")
    upgrade
    ;;
  *)
    printf "\033[32m Alpine\033[0m"
    remove
    ;;
esac