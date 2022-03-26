#!/bin/sh

if [ -z "$APP_DB_USERNAME" -o -z "$APP_DB_PASSWORD" ]; then
  echo "You need to specify 'APP_DB_USERNAME' & 'APP_DB_PASSWORD'"
  exit 1
fi
exec "$@"