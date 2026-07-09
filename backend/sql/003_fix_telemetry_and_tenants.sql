-- 003_fix_telemetry_and_tenants.sql
-- Fix two issues from the first RLS pass:
--   1. tenants is the root catalog table and must NOT have RLS.
--   2. telemetry has no tenant_id column; the original policy used it
--      and failed. Replace with a subquery against agents.

ALTER TABLE tenants DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_telemetry ON telemetry;
CREATE POLICY tenant_isolation_telemetry ON telemetry
  USING      (agent_id IN (SELECT id FROM agents))
  WITH CHECK (agent_id IN (SELECT id FROM agents));
