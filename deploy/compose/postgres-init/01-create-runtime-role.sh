#!/bin/sh
# Runs automatically on the FIRST boot of a fresh postgres data volume only
# (docker-entrypoint-initdb.d scripts never run again once the volume has
# data) — creates the non-owning runtime role apps/server/apps/worker
# connect as. Table/sequence/function privileges for this role are granted
# later, by migrations/0029_runtime_role_grants.up.sql, since the tables
# don't exist yet at this point in a fresh install.
#
# See migrations/0001_organisation_hierarchy.up.sql's DEPLOYMENT
# REQUIREMENT comment for why this role must NOT be the one that runs
# migrations / owns the tables.
set -e

: "${BILLING_APP_PASSWORD:?BILLING_APP_PASSWORD must be set — see deploy/compose/.env.example}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
	DO \$\$
	BEGIN
	    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'billing_app') THEN
	        CREATE ROLE billing_app LOGIN PASSWORD '$BILLING_APP_PASSWORD';
	    END IF;
	END
	\$\$;
EOSQL
