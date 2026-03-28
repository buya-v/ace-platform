#!/bin/bash
set -e

# ── ACE Platform — PostgreSQL Initialization ─────────────────────────
# Runs once on first container start (via docker-entrypoint-initdb.d).
# Creates schemas, enables extensions, and creates service roles.

PGUSER="${POSTGRES_USER:-ace_admin}"
DB="${POSTGRES_DB:-ace_platform}"

echo "=== ACE Platform: Initializing database ==="

psql -v ON_ERROR_STOP=1 --username "$PGUSER" --dbname "$DB" <<-EOSQL

    -- Enable extensions
    CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
    CREATE EXTENSION IF NOT EXISTS "pgcrypto";
    -- TimescaleDB: uncomment when using timescale/timescaledb image
    -- CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

    -- Create schemas
    CREATE SCHEMA IF NOT EXISTS reference;
    CREATE SCHEMA IF NOT EXISTS participants;
    CREATE SCHEMA IF NOT EXISTS exchange;
    CREATE SCHEMA IF NOT EXISTS clearing;
    CREATE SCHEMA IF NOT EXISTS compliance;
    CREATE SCHEMA IF NOT EXISTS warehouse;
    CREATE SCHEMA IF NOT EXISTS auth;
    CREATE SCHEMA IF NOT EXISTS market_data;

    -- Create service roles (one per domain for least-privilege access)
    DO \$\$
    BEGIN
        -- Exchange service role
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_exchange_svc') THEN
            CREATE ROLE ace_exchange_svc WITH LOGIN PASSWORD '${EXCHANGE_SVC_PASSWORD:-exchange_dev_pass}';
        END IF;

        -- Auth service role
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_auth_svc') THEN
            CREATE ROLE ace_auth_svc WITH LOGIN PASSWORD '${AUTH_SVC_PASSWORD:-auth_dev_pass}';
        END IF;

        -- Clearing service role
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_clearing_svc') THEN
            CREATE ROLE ace_clearing_svc WITH LOGIN PASSWORD '${CLEARING_SVC_PASSWORD:-clearing_dev_pass}';
        END IF;

        -- Compliance service role
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_compliance_svc') THEN
            CREATE ROLE ace_compliance_svc WITH LOGIN PASSWORD '${COMPLIANCE_SVC_PASSWORD:-compliance_dev_pass}';
        END IF;

        -- Warehouse service role
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_warehouse_svc') THEN
            CREATE ROLE ace_warehouse_svc WITH LOGIN PASSWORD '${WAREHOUSE_SVC_PASSWORD:-warehouse_dev_pass}';
        END IF;

        -- Analytics read-only role
        IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_analytics_ro') THEN
            CREATE ROLE ace_analytics_ro WITH LOGIN PASSWORD '${ANALYTICS_SVC_PASSWORD:-analytics_dev_pass}';
        END IF;
    END
    \$\$;

    -- Grant schema access to service roles
    GRANT USAGE ON SCHEMA reference TO ace_exchange_svc, ace_clearing_svc, ace_compliance_svc, ace_warehouse_svc, ace_analytics_ro;
    GRANT USAGE ON SCHEMA exchange TO ace_exchange_svc, ace_clearing_svc, ace_analytics_ro;
    GRANT USAGE ON SCHEMA clearing TO ace_clearing_svc, ace_analytics_ro;
    GRANT USAGE ON SCHEMA compliance TO ace_compliance_svc, ace_analytics_ro;
    GRANT USAGE ON SCHEMA warehouse TO ace_warehouse_svc, ace_analytics_ro;
    GRANT USAGE ON SCHEMA auth TO ace_auth_svc;
    GRANT USAGE ON SCHEMA market_data TO ace_exchange_svc, ace_analytics_ro;
    GRANT USAGE ON SCHEMA participants TO ace_exchange_svc, ace_clearing_svc, ace_compliance_svc, ace_warehouse_svc, ace_auth_svc;

    -- Default privileges: grant SELECT on future tables to analytics role
    ALTER DEFAULT PRIVILEGES IN SCHEMA exchange GRANT SELECT ON TABLES TO ace_analytics_ro;
    ALTER DEFAULT PRIVILEGES IN SCHEMA clearing GRANT SELECT ON TABLES TO ace_analytics_ro;
    ALTER DEFAULT PRIVILEGES IN SCHEMA compliance GRANT SELECT ON TABLES TO ace_analytics_ro;
    ALTER DEFAULT PRIVILEGES IN SCHEMA market_data GRANT SELECT ON TABLES TO ace_analytics_ro;

EOSQL

echo "=== ACE Platform: Database initialization complete ==="
echo "  Schemas: reference, participants, exchange, clearing, compliance, warehouse, auth, market_data"
echo "  Roles: ace_exchange_svc, ace_auth_svc, ace_clearing_svc, ace_compliance_svc, ace_warehouse_svc, ace_analytics_ro"
