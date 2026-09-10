#!/bin/sh
set -eu

# One transaction and advisory lock prevent partial or competing schema updates.
# Only forward migrations are included; destructive down migrations never run.
script=$(mktemp)
trap 'rm -f "$script"' EXIT HUP INT TERM
cat > "$script" <<'SQL'
BEGIN;
SELECT pg_advisory_xact_lock(735091284);
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    service TEXT NOT NULL,
    version TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (service, version)
);
SQL

# User's historical schema must exist before auth's legacy-account import.
for service in user-service auth-service post-service chat-service notification-service; do
    for migration in /migrations/"$service"/*.up.sql; do
        [ -f "$migration" ] || { echo "Missing migrations for $service" >&2; exit 1; }
        version=$(basename "$migration")
        case "$version" in *[!a-zA-Z0-9_.-]*) echo "Invalid migration filename" >&2; exit 1;; esac
        checksum=$(sha256sum "$migration")
        checksum=${checksum%% *}
        cat >> "$script" <<SQL
SELECT EXISTS (SELECT 1 FROM public.schema_migrations WHERE service = '$service' AND version = '$version') AS already_applied \gset
\if :already_applied
DO \$\$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM public.schema_migrations WHERE service = '$service' AND version = '$version' AND checksum = '$checksum') THEN
  RAISE EXCEPTION 'Checksum mismatch for $service/$version; restore the applied file and add a new migration';
 END IF;
END \$\$;
\echo Verified $service/$version
\else
\echo Applying $service/$version
\i $migration
INSERT INTO public.schema_migrations (service, version, checksum) VALUES ('$service', '$version', '$checksum');
\endif
SQL
    done
done
printf '\nCOMMIT;\n' >> "$script"
psql -X --set=ON_ERROR_STOP=1 --file="$script"
