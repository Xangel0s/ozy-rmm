-- 002_enable_rls.sql
-- Enable PostgreSQL Row-Level Security (RLS) on all 11 tenant-scoped tables.
--
-- Pre-requisites:
--   * `apexrmm_app` role exists (NOSUPERUSER, NOBYPASSRLS) — see 001_create_app_role.sql
--   * Backend `db` pool connects as `apexrmm_app`
--   * Backend `dbAdmin` pool connects as `apexrmm` (SUPERUSER, BYPASSRLS)
--     and is used ONLY for schema setup and handleEnrollAgent.
--
-- What this does:
--   1. ENABLE ROW LEVEL SECURITY on each table — RLS becomes possible.
--   2. FORCE ROW LEVEL SECURITY — applies RLS even to the table owner.
--      Without FORCE, the owner role bypasses RLS by default.
--   3. Create a `tenant_isolation_*` policy per table that filters by
--      `current_setting('app.tenant_id', true)::text`.
--
-- The `true` second argument to current_setting means "return NULL if
-- unset instead of erroring". A NULL setting fails the comparison, so
-- any code path that forgets `SET LOCAL app.tenant_id` will see zero
-- rows — fail-closed.

-- ─── agents ───────────────────────────────────────────────────────────────────
ALTER TABLE agents              ENABLE ROW LEVEL SECURITY;
ALTER TABLE agents              FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_agents ON agents;
CREATE POLICY tenant_isolation_agents ON agents
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── telemetry ────────────────────────────────────────────────────────────────
-- telemetry has no tenant_id column (it joins to agents via agent_id).
-- The policy uses a subquery against agents, which itself is RLS-gated.
-- This is a 1-line "is this agent's tenant mine" check, executed per row.
ALTER TABLE telemetry           ENABLE ROW LEVEL SECURITY;
ALTER TABLE telemetry           FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_telemetry ON telemetry;
CREATE POLICY tenant_isolation_telemetry ON telemetry
  USING      (agent_id IN (SELECT id FROM agents))
  WITH CHECK (agent_id IN (SELECT id FROM agents));

-- ─── alerts ───────────────────────────────────────────────────────────────────
ALTER TABLE alerts              ENABLE ROW LEVEL SECURITY;
ALTER TABLE alerts              FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_alerts ON alerts;
CREATE POLICY tenant_isolation_alerts ON alerts
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── audit_log ────────────────────────────────────────────────────────────────
ALTER TABLE audit_log           ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log           FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_audit_log ON audit_log;
CREATE POLICY tenant_isolation_audit_log ON audit_log
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── backup_jobs ──────────────────────────────────────────────────────────────
ALTER TABLE backup_jobs         ENABLE ROW LEVEL SECURITY;
ALTER TABLE backup_jobs         FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_backup_jobs ON backup_jobs;
CREATE POLICY tenant_isolation_backup_jobs ON backup_jobs
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── agent_software ───────────────────────────────────────────────────────────
ALTER TABLE agent_software      ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_software      FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_agent_software ON agent_software;
CREATE POLICY tenant_isolation_agent_software ON agent_software
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── agent_patches ────────────────────────────────────────────────────────────
ALTER TABLE agent_patches       ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_patches       FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_agent_patches ON agent_patches;
CREATE POLICY tenant_isolation_agent_patches ON agent_patches
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── agent_notes ──────────────────────────────────────────────────────────────
ALTER TABLE agent_notes         ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_notes         FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_agent_notes ON agent_notes;
CREATE POLICY tenant_isolation_agent_notes ON agent_notes
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── agent_logs ───────────────────────────────────────────────────────────────
ALTER TABLE agent_logs          ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_logs          FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_agent_logs ON agent_logs;
CREATE POLICY tenant_isolation_agent_logs ON agent_logs
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── agent_checks ─────────────────────────────────────────────────────────────
ALTER TABLE agent_checks        ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_checks        FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_agent_checks ON agent_checks;
CREATE POLICY tenant_isolation_agent_checks ON agent_checks
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── registration_tokens ──────────────────────────────────────────────────────
-- Note: registration_tokens is read by handleEnrollAgent via dbAdmin
-- (BYPASSRLS), so the policy only matters for handleCreateRegistrationToken
-- and any future read paths. The handler passes tenantID from the JWT, so
-- the WITH CHECK clause ensures a token can only be created for the
-- caller's own tenant.
ALTER TABLE registration_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE registration_tokens FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_registration_tokens ON registration_tokens;
CREATE POLICY tenant_isolation_registration_tokens ON registration_tokens
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── terminal_sessions ────────────────────────────────────────────────────────
ALTER TABLE terminal_sessions    ENABLE ROW LEVEL SECURITY;
ALTER TABLE terminal_sessions    FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_terminal_sessions ON terminal_sessions;
CREATE POLICY tenant_isolation_terminal_sessions ON terminal_sessions
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── scripts ──────────────────────────────────────────────────────────────────
ALTER TABLE scripts             ENABLE ROW LEVEL SECURITY;
ALTER TABLE scripts             FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_scripts ON scripts;
CREATE POLICY tenant_isolation_scripts ON scripts
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── script_executions ───────────────────────────────────────────────────────
ALTER TABLE script_executions   ENABLE ROW LEVEL SECURITY;
ALTER TABLE script_executions   FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_script_executions ON script_executions;
CREATE POLICY tenant_isolation_script_executions ON script_executions
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));

-- ─── users ───────────────────────────────────────────────────────────────────
ALTER TABLE users               ENABLE ROW LEVEL SECURITY;
ALTER TABLE users               FORCE  ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_users ON users;
CREATE POLICY tenant_isolation_users ON users
  USING      (tenant_id::text = current_setting('app.tenant_id', true))
  WITH CHECK (tenant_id::text = current_setting('app.tenant_id', true));
