#!/usr/bin/env sh
set -u

cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
first_run=false
if [ ! -f .config.yaml ]; then
  first_run=true
fi
echo "Starting xiaozhi-server-go..."
./xiaozhi-server
status=$?
if [ "$first_run" = true ]; then
  echo "First start creates .config.yaml. Edit it, then run start.sh again."
fi
exit "$status"
