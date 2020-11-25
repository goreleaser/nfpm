#!/bin/bash

if [ "$1" = "triggered" ] && [ "$2" = "manual-trigger" ]; then
    echo "Ok" > /tmp/trigger-proof
fi
