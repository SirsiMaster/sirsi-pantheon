-- Router ledger — roles (ADR-062 §4 least privilege, rs-05).
--
-- Cluster-level, run once per database server by the owner (or the
-- provisioning step rs-15) BEFORE schema.sql. Passwords are never in this
-- file: they are set out of band (`ALTER ROLE ... PASSWORD` from Secret
-- Manager on Cloud SQL; trust auth on the scratch server).
--
--   router_migrator — owns the schema; the only role that runs DDL
--                     (schema.sql, future migrations, `sirsi router migrate`).
--   router_service  — what `sirsi router serve` connects as: DML on
--                     router.* only, no DDL, no superuser, no other schemas.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'router_migrator') THEN
    CREATE ROLE router_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'router_service') THEN
    CREATE ROLE router_service LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
  END IF;
END $$;
