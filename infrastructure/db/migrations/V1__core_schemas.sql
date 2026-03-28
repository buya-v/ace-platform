-- V1: Create core schemas and service roles for ACE Platform
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
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_exchange_svc') THEN CREATE ROLE ace_exchange_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_clearing_svc') THEN CREATE ROLE ace_clearing_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_compliance_svc') THEN CREATE ROLE ace_compliance_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_warehouse_svc') THEN CREATE ROLE ace_warehouse_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_auth_svc') THEN CREATE ROLE ace_auth_svc LOGIN; END IF;
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ace_market_data_svc') THEN CREATE ROLE ace_market_data_svc LOGIN; END IF;
END $$;
