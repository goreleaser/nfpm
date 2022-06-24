#!/bin/bash

set -o pipefail && TEST_OPTIONS="-json" task $1 | tee output.json | tparse -follow
success=$?

if [ "$2" = "ubuntu-latest" ]; then
	set -e
	NO_COLOR=1 tparse -format markdown -slow 10 -file output.json > $GITHUB_STEP_SUMMARY
fi

exit $success
