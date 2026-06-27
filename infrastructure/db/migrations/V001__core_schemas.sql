-- V1: Create core schemas and service roles for GarudaX Platform
CREATE SCHEMA IF NOT EXISTS reference;
CREATE SCHEMA IF NOT EXISTS participants;
CREATE SCHEMA IF NOT EXISTS exchange;
CREATE SCHEMA IF NOT EXISTS clearing;
CREATE SCHEMA IF NOT EXISTS compliance;
CREATE SCHEMA IF NOT EXISTS warehouse;
CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS market_data;

-- Service roles (least-privilege per domain)
DO $$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_exchange_svc') THEN CREATE ROLE garudax_exchange_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_clearing_svc') THEN CREATE ROLE garudax_clearing_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_compliance_svc') THEN CREATE ROLE garudax_compliance_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_warehouse_svc') THEN CREATE ROLE garudax_warehouse_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_auth_svc') THEN CREATE ROLE garudax_auth_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'garudax_market_data_svc') THEN CREATE ROLE garudax_market_data_svc LOGIN; END IF;
END $$;
