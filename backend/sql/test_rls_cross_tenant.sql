-- Test RLS cross-tenant blocking
-- Test 1: SET app.tenant_id to a FAKE tenant. Should see 0 agents.
BEGIN;
SELECT set_config('app.tenant_id', '00000000-0000-0000-0000-000000000000', true);
SELECT COUNT(*) AS fake_tenant_agents FROM agents;
SELECT COUNT(*) AS fake_tenant_software FROM agent_software;
SELECT COUNT(*) AS fake_tenant_alerts FROM alerts;
ROLLBACK;

-- Test 2: SET app.tenant_id to the REAL tenant. Should see 1 agent.
BEGIN;
SELECT set_config('app.tenant_id', 'bf96ef75-40ed-47b8-a8d7-f044fe03c04d', true);
SELECT COUNT(*) AS real_tenant_agents FROM agents;
SELECT COUNT(*) AS real_tenant_software FROM agent_software;
SELECT COUNT(*) AS real_tenant_alerts FROM alerts;
ROLLBACK;
