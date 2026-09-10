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

-- Attributes are enforced on EVERY apply, not only at CREATE: on Cloud SQL the
-- users are created by `gcloud sql users create`, which grants CREATEROLE and
-- CREATEDB, so the guarded CREATE above never ran and the live router_service
-- held both (apply-schema job audit, 2026-09-10). Idempotent.
--
-- Only the two attributes the production caller may change. router_migrator is
-- not a superuser, and PostgreSQL 16 lets a non-superuser ALTER ROLE only when it
-- holds CREATEROLE, ADMIN OPTION on the target, and — to change CREATEDB — the
-- CREATEDB attribute itself (src/backend/commands/user.c, AlterRole). SUPERUSER
-- and BYPASSRLS options are superuser-only even when setting them false, so
-- they are asserted by the audits, never altered here. The apply-schema job
-- checks that authority before the bundle runs; the one-time grant it names is
-- `GRANT router_service TO router_migrator WITH ADMIN OPTION, INHERIT FALSE, SET FALSE;`
-- executed as the Cloud SQL postgres user.
ALTER ROLE router_service NOCREATEDB NOCREATEROLE;
