#!/bin/sh
set -eu
if [ "$PGUSER" = "$MIGRATION_USER" ] || [ "$PGUSER" = "$APP_DB_USER" ] || [ "$MIGRATION_USER" = "$APP_DB_USER" ]; then
    echo 'Database roles must be distinct' >&2
    exit 1
fi
if ! psql -X -v ON_ERROR_STOP=1 -f /postgres/setup.sql >/dev/null 2>&1; then
    echo 'PostgreSQL role setup failed; check credentials and role settings' >&2
    exit 1
fi
