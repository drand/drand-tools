#!bin/sh
set -e

if [ -n "$DOCKER_DEBUG" ]; then
   set -x
fi

exec db-migration $@
