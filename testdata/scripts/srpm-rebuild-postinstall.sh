#!/bin/bash

# The longest-suffix-strip idiom must survive `rpmbuild --rebuild` verbatim.
# Without %-escaping in the generated spec, ${host%%.*} is silently mangled to
# ${host%.*}, which strips the shortest suffix instead of the longest.
host="server.example.com"
echo "${host%%.*}" > /dev/null
